package etcd

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.etcd.io/etcd/clientv3"
	"sigs.k8s.io/yaml"

	"github.com/flexkube/libflexkube/internal/util"
	"github.com/flexkube/libflexkube/pkg/container"
	"github.com/flexkube/libflexkube/pkg/host"
	"github.com/flexkube/libflexkube/pkg/host/transport/ssh"
	"github.com/flexkube/libflexkube/pkg/types"
)

// defaultDialTimeout is default timeout value for etcd client.
const defaultDialTimeout = 5 * time.Second

// Cluster represents etcd cluster configuration and state from the user.
type Cluster struct {
	// User-configurable fields.
	Image         string            `json:"image,omitempty"`
	SSH           *ssh.Config       `json:"ssh,omitempty"`
	CACertificate types.Certificate `json:"caCertificate,omitempty"`
	Members       map[string]Member `json:"members,omitempty"`

	// Serializable fields.
	State container.ContainersState `json:"state"`
}

// cluster is executable version of Cluster, with validated fields and calculated containers.
type cluster struct {
	image         string
	ssh           *ssh.Config
	caCertificate string
	containers    container.Containers
	members       map[string]*member
}

// propagateMember fills given Member's empty fields with fields from Cluster.
func (c *Cluster) propagateMember(i string, m *Member) {
	initialClusterArr := []string{}
	peerCertAllowedCNArr := []string{}

	for n, m := range c.Members {
		initialClusterArr = append(initialClusterArr, fmt.Sprintf("%s=https://%s:2380", fmt.Sprintf("etcd-%s", n), m.PeerAddress))
		peerCertAllowedCNArr = append(peerCertAllowedCNArr, fmt.Sprintf("etcd-%s", n))
	}

	sort.Strings(initialClusterArr)
	sort.Strings(peerCertAllowedCNArr)

	m.Name = util.PickString(m.Name, fmt.Sprintf("etcd-%s", i))
	m.Image = util.PickString(m.Image, c.Image)
	m.InitialCluster = util.PickString(m.InitialCluster, strings.Join(initialClusterArr, ","))
	m.PeerCertAllowedCN = util.PickString(m.PeerCertAllowedCN, strings.Join(peerCertAllowedCNArr, ","))
	m.CACertificate = types.Certificate(util.PickString(string(m.CACertificate), string(c.CACertificate)))

	m.Host = host.BuildConfig(m.Host, host.Host{
		SSHConfig: c.SSH,
	})

	if len(c.State) == 0 {
		m.NewCluster = true
	}
}

// New validates etcd cluster configuration and fills members with default and computed values.
func (c *Cluster) New() (types.Resource, error) {
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("failed to validate cluster configuration: %w", err)
	}

	cluster := &cluster{
		image:         c.Image,
		ssh:           c.SSH,
		caCertificate: string(c.CACertificate),
		containers: container.Containers{
			PreviousState: c.State,
			DesiredState:  make(container.ContainersState),
		},
		members: map[string]*member{},
	}

	for n, m := range c.Members {
		m := m
		c.propagateMember(n, &m)

		mem, _ := m.New()
		hcc, _ := mem.ToHostConfiguredContainer()

		cluster.containers.DesiredState[n] = hcc

		cluster.members[n] = mem.(*member)
	}

	return cluster, nil
}

// Validate validates Cluster configuration.
func (c *Cluster) Validate() error {
	if len(c.Members) == 0 && c.State == nil {
		return fmt.Errorf("either members or previous state needs to be defined")
	}

	for n, m := range c.Members {
		m := m
		c.propagateMember(n, &m)

		member, err := m.New()
		if err != nil {
			return fmt.Errorf("failed to validate member '%s': %w", n, err)
		}

		if _, err := member.ToHostConfiguredContainer(); err != nil {
			return fmt.Errorf("failed to generate container configuration for member '%s': %w", n, err)
		}
	}

	return nil
}

// FromYaml allows to restore cluster state from YAML.
func FromYaml(c []byte) (types.Resource, error) {
	return types.ResourceFromYaml(c, &Cluster{})
}

// StateToYaml allows to dump cluster state to YAML, so it can be restored later.
func (c *cluster) StateToYaml() ([]byte, error) {
	return yaml.Marshal(Cluster{State: c.containers.PreviousState})
}

// CheckCurrentState refreshes current state of the cluster.
func (c *cluster) CheckCurrentState() error {
	if err := c.containers.CheckCurrentState(); err != nil {
		return fmt.Errorf("failed checking current state of etcd cluster: %w", err)
	}

	return nil
}

// getExistingEndpoints returns list of already deployed etcd endpoints.
func (c *cluster) getExistingEndpoints() []string {
	endpoints := []string{}

	for i, m := range c.members {
		if _, ok := c.containers.PreviousState[i]; !ok {
			continue
		}

		endpoints = append(endpoints, fmt.Sprintf("%s:2379", m.peerAddress))
	}

	return endpoints
}

func (c *cluster) firstMember() (*member, error) {
	var m *member

	for i := range c.members {
		m = c.members[i]
		break
	}

	if m == nil {
		return nil, fmt.Errorf("no members defined")
	}

	return m, nil
}

func (c *cluster) getClient() (etcdClient, error) {
	m, err := c.firstMember()
	if err != nil {
		return nil, fmt.Errorf("failed getting member object: %w", err)
	}

	endpoints, err := m.forwardEndpoints(c.getExistingEndpoints())
	if err != nil {
		return nil, fmt.Errorf("failed forwarding endpoints: %w", err)
	}

	return m.getEtcdClient(endpoints)
}

type etcdClient interface {
	MemberList(context context.Context) (*clientv3.MemberListResponse, error)
	MemberAdd(context context.Context, peerURLs []string) (*clientv3.MemberAddResponse, error)
	MemberRemove(context context.Context, id uint64) (*clientv3.MemberRemoveResponse, error)
	Close() error
}

func (c *cluster) membersToRemove() []string {
	m := []string{}

	for i := range c.containers.PreviousState {
		if _, ok := c.containers.DesiredState[i]; !ok {
			m = append(m, i)
		}
	}

	return m
}

func (c *cluster) membersToAdd() []string {
	m := []string{}

	for i := range c.containers.DesiredState {
		if _, ok := c.containers.PreviousState[i]; !ok {
			m = append(m, i)
		}
	}

	return m
}

// updateMembers adds and remove members from the cluster according to the configuration.
func (c *cluster) updateMembers(cli etcdClient) error {
	for _, name := range c.membersToRemove() {
		m := &member{
			name: name,
		}

		if err := m.remove(cli); err != nil {
			return fmt.Errorf("failed removing member: %w", err)
		}
	}

	for _, m := range c.membersToAdd() {
		if err := c.members[m].add(cli); err != nil {
			return fmt.Errorf("failed adding member: %w", err)
		}
	}

	return nil
}

// Deploy refreshes current state of the cluster and deploys detected changes.
func (c *cluster) Deploy() error {
	// If we create new cluster or destroy entire cluster, just start deploying.
	if len(c.containers.PreviousState) != 0 && len(c.containers.DesiredState) != 0 {
		// Build client, so we can pass it around.
		cli, err := c.getClient()
		if err != nil {
			return fmt.Errorf("failed getting etcd client: %w", err)
		}

		defer cli.Close()

		if err := c.updateMembers(cli); err != nil {
			return fmt.Errorf("failed to update members before deploying: %w", err)
		}
	}

	return c.containers.Deploy()
}
