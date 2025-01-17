package etcd_test

import (
	"testing"

	"github.com/flexkube/libflexkube/internal/utiltest"
	"github.com/flexkube/libflexkube/pkg/defaults"
	"github.com/flexkube/libflexkube/pkg/etcd"
	"github.com/flexkube/libflexkube/pkg/host"
	"github.com/flexkube/libflexkube/pkg/host/transport/direct"
)

const (
	nonEmptyString = "foo"
)

func TestMemberToHostConfiguredContainer(t *testing.T) {
	t.Parallel()

	cert := utiltest.GenerateX509Certificate(t)
	privateKey := utiltest.GenerateRSAPrivateKey(t)

	kas := &etcd.MemberConfig{
		Name:              nonEmptyString,
		PeerAddress:       nonEmptyString,
		CACertificate:     cert,
		PeerCertificate:   cert,
		PeerKey:           privateKey,
		ServerCertificate: cert,
		ServerKey:         privateKey,
		Image:             defaults.EtcdImage,
		PeerCertAllowedCN: nonEmptyString,
		Host: host.Host{
			DirectConfig: &direct.Config{},
		},
	}

	o, err := kas.New()
	if err != nil {
		t.Fatalf("New should not return error, got: %v", err)
	}

	hcc, err := o.ToHostConfiguredContainer()
	if err != nil {
		t.Fatalf("Generating HostConfiguredContainer should work, got: %v", err)
	}

	if _, err := hcc.New(); err != nil {
		t.Fatalf("ToHostConfiguredContainer() should generate valid HostConfiguredContainer, got: %v", err)
	}
}

func validMember(t *testing.T) *etcd.MemberConfig {
	t.Helper()

	cert := utiltest.GenerateX509Certificate(t)
	privateKey := utiltest.GenerateRSAPrivateKey(t)

	return &etcd.MemberConfig{
		Name:              nonEmptyString,
		PeerAddress:       nonEmptyString,
		CACertificate:     cert,
		PeerCertificate:   cert,
		PeerKey:           privateKey,
		ServerCertificate: cert,
		ServerKey:         privateKey,
		Image:             defaults.EtcdImage,
		PeerCertAllowedCN: nonEmptyString,
		Host: host.Host{
			DirectConfig: &direct.Config{},
		},
	}
}

// Validate() tests.
//
//nolint:funlen // Just many test cases.
func TestValidate(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		mutator     func(m *etcd.MemberConfig) *etcd.MemberConfig
		expectError bool
	}{
		"valid": {
			func(m *etcd.MemberConfig) *etcd.MemberConfig { return m },
			false,
		},
		"peer address": {
			func(m *etcd.MemberConfig) *etcd.MemberConfig {
				m.PeerAddress = ""

				return m
			},
			true,
		},
		"member name": {
			func(m *etcd.MemberConfig) *etcd.MemberConfig {
				m.Name = ""

				return m
			},
			true,
		},
		"CA certificate": {
			func(m *etcd.MemberConfig) *etcd.MemberConfig {
				m.CACertificate = nonEmptyString

				return m
			},
			true,
		},
		"peer certificate": {
			func(m *etcd.MemberConfig) *etcd.MemberConfig {
				m.PeerCertificate = nonEmptyString

				return m
			},
			true,
		},
		"server certificate": {
			func(m *etcd.MemberConfig) *etcd.MemberConfig {
				m.ServerCertificate = nonEmptyString

				return m
			},
			true,
		},
		"peer key": {
			func(m *etcd.MemberConfig) *etcd.MemberConfig {
				m.PeerKey = nonEmptyString

				return m
			},
			true,
		},
		"server key": {
			func(m *etcd.MemberConfig) *etcd.MemberConfig {
				m.ServerKey = nonEmptyString

				return m
			},
			true,
		},
		"bad host": {
			func(m *etcd.MemberConfig) *etcd.MemberConfig {
				m.Host.DirectConfig = nil

				return m
			},
			true,
		},
	}

	for c, testCase := range cases {
		testCase := testCase

		t.Run(c, func(t *testing.T) {
			t.Parallel()

			m := testCase.mutator(validMember(t))
			err := m.Validate()

			if testCase.expectError && err == nil {
				t.Fatalf("Expected error")
			}

			if !testCase.expectError && err != nil {
				t.Fatalf("Didn't expect error, got: %v", err)
			}
		})
	}
}
