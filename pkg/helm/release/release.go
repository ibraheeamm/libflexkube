// Package release allows to manage Helm 3 releases.
package release

import (
	"context"
	"fmt"
	"strings"

	"github.com/flexkube/helm/v3/pkg/action"
	"github.com/flexkube/helm/v3/pkg/chart"
	"github.com/flexkube/helm/v3/pkg/chart/loader"
	"github.com/flexkube/helm/v3/pkg/cli"
	"github.com/flexkube/helm/v3/pkg/storage"
	"github.com/flexkube/helm/v3/pkg/storage/driver"
	"sigs.k8s.io/yaml"

	"github.com/flexkube/libflexkube/internal/util"
	"github.com/flexkube/libflexkube/pkg/kubernetes/client"
)

// Release is an interface representing helm release.
type Release interface {
	// ValidateChart validates configured chart.
	ValidateChart() error

	// Install installs configured release. If release already exists, error will be returned.
	Install(context.Context) error

	// Upgrade upgrades configured release. If release does not exist, error will be returned.
	Upgrade(context.Context) error

	// InstallOrUpgrade either installs or upgrades the release, depends whether it exists or not.
	InstallOrUpgrade(context.Context) error

	// Exists checks, if release exists. If cluster is not reachable, error is returned.
	Exists() (bool, error)

	// Uninstall removes the release.
	Uninstall() error
}

// Config represents user-configured Helm release.
type Config struct {
	// Kubeconfig is content of kubeconfig file in YAML format, which will be used to authenticate
	// to the cluster and create a release.
	Kubeconfig string `json:"kubeconfig,omitempty"`

	// Namespace is a namespace, where helm release will be created and all it's resources.
	Namespace string `json:"namespace,omitempty"`

	// Name is a name of the release used to identify it.
	Name string `json:"name,omitempty"`

	// Chart is a location of the chart. It may be local path or remote chart in user repository.
	Chart string `json:"chart,omitempty"`

	// Values is a chart values in YAML format.
	Values string `json:"values,omitempty"`

	// Version is a requested version of the chart.
	Version string `json:"version,omitempty"`

	// CreateNamespace controls, if the namespace for the release should be created before installing
	// the release.
	CreateNamespace bool `json:"createNamespace,omitempty"`

	// Wait controls if client should wait until managed chart converges.
	Wait bool `json:"wait,omitempty"`
}

// release is a validated and installable/update'able version of Config.
type release struct {
	actionConfig    *action.Configuration
	settings        *cli.EnvSettings
	values          map[string]interface{}
	name            string
	namespace       string
	version         string
	chart           string
	client          client.Client
	createNamespace bool
	wait            bool
}

// New validates release configuration and builds installable version of it.
func (r *Config) New() (Release, error) {
	if err := r.Validate(); err != nil {
		return nil, fmt.Errorf("validating Helm release: %w", err)
	}

	// Initialize kubernetes and helm CLI clients.
	actionConfig := &action.Configuration{}
	settings := cli.New()

	getter, kc, clientSet, _ := newClients(r.Kubeconfig) //nolint:errcheck // We check it in Validate().

	kc.Namespace = r.Namespace

	actionConfig.RESTClientGetter = getter
	actionConfig.KubeClient = kc
	actionConfig.Releases = storage.Init(driver.NewSecrets(clientSet.CoreV1().Secrets(r.Namespace)))
	actionConfig.Log = func(_ string, _ ...interface{}) {}

	values, _ := r.parseValues() //nolint:errcheck // We check it in Validate().

	client, _ := client.NewClient([]byte(r.Kubeconfig)) //nolint:errcheck // We check it in Validate().

	release := &release{
		actionConfig:    actionConfig,
		settings:        settings,
		values:          values,
		name:            r.Name,
		namespace:       r.Namespace,
		version:         r.Version,
		chart:           r.Chart,
		client:          client,
		createNamespace: r.CreateNamespace,
		wait:            r.Wait,
	}

	return release, nil
}

// Validate validates Release configuration.
func (r *Config) Validate() error {
	var errors util.ValidateErrors

	// Check if all required values are filled in.
	if r.Kubeconfig == "" {
		errors = append(errors, fmt.Errorf("kubeconfig is empty"))
	}

	if r.Namespace == "" {
		errors = append(errors, fmt.Errorf("namespace is empty"))
	}

	if r.Name == "" {
		errors = append(errors, fmt.Errorf("name is empty"))
	}

	if r.Chart == "" {
		errors = append(errors, fmt.Errorf("chart is empty"))
	}

	// Try to create a clients.
	if _, _, _, err := newClients(r.Kubeconfig); err != nil {
		errors = append(errors, fmt.Errorf("creating Kubernetes clients: %w", err))
	}

	// Parse given values.
	if _, err := r.parseValues(); err != nil {
		errors = append(errors, fmt.Errorf("parsing values: %w", err))
	}

	return errors.Return()
}

// ValidateChart locates and parses the chart.
//
// This method is not part of Validate(), since Validate() should be fully offline and no-op.
// However, if user wants know that the chart is already available and wants to avoid runtime
// errors, this function can be called in addition to Validate().
func (r *release) ValidateChart() error {
	if _, err := r.loadChart(); err != nil {
		return fmt.Errorf("validating chart: %w", err)
	}

	return nil
}

// Install installs configured chart as release. Equivalent of 'helm install'.
func (r *release) Install(ctx context.Context) error {
	if err := r.client.PingWait(client.PollInterval, client.RetryTimeout); err != nil {
		return fmt.Errorf("timed out waiting for kube-apiserver to be reachable")
	}

	client := r.installClient()

	chart, err := r.loadChart()
	if err != nil {
		return fmt.Errorf("loading chart: %w", err)
	}

	client.CreateNamespace = r.createNamespace

	// Install a release.
	if err := retryOnEtcdError(func() error {
		_, err = client.RunWithContext(ctx, chart, r.values)

		return err
	}); err != nil {
		return fmt.Errorf("installing a release: %w", err)
	}

	return nil
}

// Upgrade upgrades already existing release. Equivalent of 'helm upgrade'.
func (r *release) Upgrade(ctx context.Context) error {
	if err := r.client.PingWait(client.PollInterval, client.RetryTimeout); err != nil {
		return fmt.Errorf("timed out waiting for kube-apiserver to be reachable")
	}

	client := r.upgradeClient()

	chart, err := r.loadChart()
	if err != nil {
		return fmt.Errorf("loading chart: %w", err)
	}

	if err := retryOnEtcdError(func() error {
		_, err := client.RunWithContext(ctx, r.name, chart, r.values)

		return err
	}); err != nil {
		return fmt.Errorf("upgrading a release: %w", err)
	}

	return nil
}

// InstallOrUpgrade checks if release already exists, and if it does it tries to upgrade it
// If the release does not exist, it will be created.
func (r *release) InstallOrUpgrade(ctx context.Context) error {
	e, err := r.Exists()
	if err != nil {
		return fmt.Errorf("checking release existence: %w", err)
	}

	if e {
		return r.Upgrade(ctx)
	}

	return r.Install(ctx)
}

// Exists checks if configured release exists.
func (r *release) Exists() (bool, error) {
	if err := r.client.PingWait(client.PollInterval, client.RetryTimeout); err != nil {
		return false, fmt.Errorf("timed out waiting for kube-apiserver to be reachable")
	}

	histClient := action.NewHistory(r.actionConfig)
	histClient.Max = 1

	err := retryOnEtcdError(func() error {
		_, err := histClient.Run(r.name)

		return err
	})

	if err == driver.ErrReleaseNotFound { //nolint:errorlint // errors.Is does not work for Helm errors.
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("checking if release exists: %w", err)
	}

	return true, nil
}

func retryOnEtcdError(f func() error) error {
	var err error

	for i := 0; i < 3; i++ {
		err = f()
		if err == nil {
			return nil
		}

		if strings.Contains(err.Error(), "etcdserver:") {
			continue
		}

		return err
	}

	return err
}

// Uninstall removes the release from the cluster. This function is idempotent.
func (r *release) Uninstall() error {
	// Check if release exists.
	releaseExists, err := r.Exists()
	if err != nil {
		return fmt.Errorf("checking if release exists: %w", err)
	}

	// If it does not exist anymore, simply return.
	if !releaseExists {
		return nil
	}

	client := r.uninstallClient()

	if err := retryOnEtcdError(func() error {
		_, err := client.Run(r.name)

		return err
	}); err != nil {
		return fmt.Errorf("uninstalling a release: %w", err)
	}

	return nil
}

// loadChart locates and loads the chart.
func (r *release) loadChart() (*chart.Chart, error) {
	client := r.installClient()

	// Locate chart to install.
	cp, err := client.ChartPathOptions.LocateChart(r.chart, r.settings)
	if err != nil {
		return nil, fmt.Errorf("locating chart: %w", err)
	}

	return loader.Load(cp)
}

// installClient returns action install client for helm.
func (r *release) installClient() *action.Install {
	// Initialize install action client.
	//
	// TODO: Maybe there is more generic action we could use?
	client := action.NewInstall(r.actionConfig)

	client.Version = r.version
	client.ReleaseName = r.name
	client.Namespace = r.namespace
	client.Wait = r.wait

	return client
}

// upgradeClient returns action install client for helm.
func (r *release) upgradeClient() *action.Upgrade {
	// Initialize install action client.
	// TODO: Maybe there is more generic action we could use?
	client := action.NewUpgrade(r.actionConfig)

	client.Version = r.version
	client.Namespace = r.namespace
	client.Wait = r.wait

	return client
}

// uninstallClient returns action uninstall client for helm.
func (r *release) uninstallClient() *action.Uninstall {
	// Initialize install action client.
	//
	// TODO: Maybe there is more generic action we could use?
	client := action.NewUninstall(r.actionConfig)

	return client
}

// parseValues parses release values and returns it ready to use when installing chart.
func (r *Config) parseValues() (map[string]interface{}, error) {
	values := map[string]interface{}{}
	if err := yaml.Unmarshal([]byte(r.Values), &values); err != nil {
		return nil, fmt.Errorf("parsing values: %w", err)
	}

	return values, nil
}

// FromYaml allows to quickly create new release object from YAML format.
func FromYaml(data []byte) (Release, error) {
	newConfig := &Config{}

	if err := yaml.Unmarshal(data, newConfig); err != nil {
		return nil, fmt.Errorf("unmarshaling release: %w", err)
	}

	release, err := newConfig.New()
	if err != nil {
		return nil, fmt.Errorf("creating release: %w", err)
	}

	return release, nil
}
