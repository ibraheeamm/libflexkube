package kubelet_test

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/flexkube/libflexkube/internal/utiltest"
	containertypes "github.com/flexkube/libflexkube/pkg/container/types"
	"github.com/flexkube/libflexkube/pkg/host"
	"github.com/flexkube/libflexkube/pkg/host/transport/direct"
	"github.com/flexkube/libflexkube/pkg/kubelet"
	"github.com/flexkube/libflexkube/pkg/kubernetes/client"
	"github.com/flexkube/libflexkube/pkg/pki"
	"github.com/flexkube/libflexkube/pkg/types"
)

func getClientConfig(t *testing.T) *client.Config {
	t.Helper()

	testPKI := &pki.PKI{
		Kubernetes: &pki.Kubernetes{},
	}

	if err := testPKI.Generate(); err != nil {
		t.Fatalf("Failed generating testing PKI: %v", err)
	}

	return &client.Config{
		Server:        "foo",
		CACertificate: testPKI.Kubernetes.CA.X509Certificate,
		Token:         "foob",
	}
}

func TestToHostConfiguredContainer(t *testing.T) {
	t.Parallel()

	clientConfig := getClientConfig(t)

	testKubelet := &kubelet.Kubelet{
		BootstrapConfig:         clientConfig,
		Name:                    "fooz",
		VolumePluginDir:         "/var/lib/kubelet/volumeplugins",
		KubernetesCACertificate: types.Certificate(utiltest.GenerateX509Certificate(t)),
		Host: host.Host{
			DirectConfig: &direct.Config{},
		},
		Labels: map[string]string{
			"do": "bar",
		},
		Taints: map[string]string{
			"noh": "bar",
		},
		PrivilegedLabels: map[string]string{
			"baz": "bar",
		},

		AdminConfig:   clientConfig,
		ClusterDNSIPs: []string{"10.0.0.1"},
	}

	k, err := testKubelet.New()
	if err != nil {
		t.Fatalf("Creating new kubelet should succeed, got: %v", err)
	}

	hcc, err := k.ToHostConfiguredContainer()
	if err != nil {
		t.Fatalf("Generating HostConfiguredContainer should work, got: %v", err)
	}

	if _, err := hcc.New(); err != nil {
		t.Fatalf("Should produce valid HostConfiguredContainer, got: %v", err)
	}
}

// Validate() tests.
func TestKubeletValidate(t *testing.T) { //nolint:funlen,cyclop // There are just many test cases.
	t.Parallel()

	cases := []struct {
		MutationF func(k *kubelet.Kubelet)
		TestF     func(t *testing.T, er error)
	}{
		{
			MutationF: func(k *kubelet.Kubelet) {},
			TestF: func(t *testing.T, err error) { //nolint:thelper // Actual test code.
				if err != nil {
					t.Fatalf("Validation of kubelet should pass, got: %v", err)
				}
			},
		},
		{
			MutationF: func(k *kubelet.Kubelet) { k.Name = "" },
			TestF: func(t *testing.T, err error) { //nolint:thelper // Actual test code.
				if err == nil {
					t.Fatalf("Validation of kubelet should fail when name is not set")
				}
			},
		},
		{
			MutationF: func(k *kubelet.Kubelet) {},
			TestF:     func(t *testing.T, err error) {}, //nolint:thelper // Actual test code.
		},
		{
			MutationF: func(k *kubelet.Kubelet) { k.KubernetesCACertificate = "" },
			TestF: func(t *testing.T, err error) { //nolint:thelper // Actual test code.
				if err == nil {
					t.Fatalf("Validation of kubelet should fail when kubernetes CA certificate is not set")
				}
			},
		},
		{
			MutationF: func(k *kubelet.Kubelet) { k.BootstrapConfig.Server = "" },
			TestF: func(t *testing.T, err error) { //nolint:thelper // Actual test code.
				if err == nil {
					t.Fatalf("Validation of kubelet should fail when bootstrap config is invalid")
				}
			},
		},
		{
			MutationF: func(k *kubelet.Kubelet) { k.VolumePluginDir = "" },
			TestF: func(t *testing.T, err error) { //nolint:thelper // Actual test code.
				if err == nil {
					t.Fatalf("Validation of kubelet should fail when volume plugin dir is empty")
				}
			},
		},
		{
			MutationF: func(k *kubelet.Kubelet) {
				k.PrivilegedLabels = map[string]string{
					"foo": "bar",
				}
				k.AdminConfig = nil
			},
			TestF: func(t *testing.T, err error) { //nolint:thelper // Actual test code.
				if err == nil {
					t.Fatalf("Validation of kubelet should fail when privileged labels are configured and admin config is not")
				}
			},
		},
		{
			MutationF: func(k *kubelet.Kubelet) {
				k.WaitForNodeReady = true
				k.AdminConfig = nil
			},
			TestF: func(t *testing.T, err error) { //nolint:thelper // Actual test code.
				if err == nil {
					t.Fatalf("Validation of kubelet should fail when waitForNodeReady is true and admin config is not set")
				}
			},
		},
		{
			MutationF: func(k *kubelet.Kubelet) { k.AdminConfig = k.BootstrapConfig },
			TestF: func(t *testing.T, err error) { //nolint:thelper // Actual test code.
				if err == nil {
					t.Fatalf("Validation of kubelet should fail when admin config is defined and there is no privileged labels")
				}
			},
		},
		{
			MutationF: func(k *kubelet.Kubelet) {
				k.PrivilegedLabels = map[string]string{
					"foo": "bar",
				}
				k.AdminConfig = &client.Config{}
			},
			TestF: func(t *testing.T, err error) { //nolint:thelper // Actual test code.
				if err == nil {
					t.Fatalf("Validation of kubelet should fail when admin config is wrong")
				}
			},
		},
		{
			MutationF: func(k *kubelet.Kubelet) { k.Host.DirectConfig = nil },
			TestF: func(t *testing.T, err error) { //nolint:thelper // Actual test code.
				if err == nil {
					t.Fatalf("Validation of kubelet should fail when host is invalid")
				}
			},
		},
	}

	for i, testCase := range cases {
		testCase := testCase

		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			clientConfig := getClientConfig(t)

			testKubelet := &kubelet.Kubelet{
				BootstrapConfig:         clientConfig,
				Name:                    "foo",
				VolumePluginDir:         "/foo",
				KubernetesCACertificate: types.Certificate(utiltest.GenerateX509Certificate(t)),
				Host: host.Host{
					DirectConfig: &direct.Config{},
				},
			}

			testCase.MutationF(testKubelet)

			testCase.TestF(t, testKubelet.Validate())
		})
	}
}

func TestKubeletIncludeExtraMounts(t *testing.T) {
	t.Parallel()

	expectedExtraMount := containertypes.Mount{
		Source: "/tmp/",
		Target: "/foo",
	}

	clientConfig := getClientConfig(t)

	testKubeletConfig := &kubelet.Kubelet{
		BootstrapConfig:         clientConfig,
		Name:                    "foo",
		VolumePluginDir:         "/var/lib/kubelet/volumeplugins",
		KubernetesCACertificate: types.Certificate(utiltest.GenerateX509Certificate(t)),
		Host: host.Host{
			DirectConfig: &direct.Config{},
		},
		Labels: map[string]string{
			"foo": "bar",
		},
		Taints: map[string]string{
			"foo": "bar",
		},
		PrivilegedLabels: map[string]string{
			"baz": "bar",
		},
		ExtraMounts:   []containertypes.Mount{expectedExtraMount},
		AdminConfig:   clientConfig,
		ClusterDNSIPs: []string{"10.0.0.1"},
	}

	testKubelet, err := testKubeletConfig.New()
	if err != nil {
		t.Fatalf("Creating new kubelet should succeed, got: %v", err)
	}

	found := false

	hcc, err := testKubelet.ToHostConfiguredContainer()
	if err != nil {
		t.Fatalf("Converting kubelet to HostConfiguredContainer: %v", err)
	}

	for _, v := range hcc.Container.Config.Mounts {
		if reflect.DeepEqual(v, expectedExtraMount) {
			found = true
		}
	}

	if !found {
		t.Fatalf("Extra mount should be included in generated mounts")
	}
}

func Test_Kubelet_container_definition_does_include_defined_extra_flags(t *testing.T) {
	t.Parallel()

	extraArg := "--foo"

	clientConfig := getClientConfig(t)

	testKubeletConfig := &kubelet.Kubelet{
		BootstrapConfig:         clientConfig,
		Name:                    "foo",
		VolumePluginDir:         "/var/lib/kubelet/volumeplugins",
		KubernetesCACertificate: types.Certificate(utiltest.GenerateX509Certificate(t)),
		Host: host.Host{
			DirectConfig: &direct.Config{},
		},
		Labels: map[string]string{
			"foo": "bar",
		},
		Taints: map[string]string{
			"foo": "bar",
		},
		PrivilegedLabels: map[string]string{
			"baz": "bar",
		},
		AdminConfig:   clientConfig,
		ClusterDNSIPs: []string{"10.0.0.1"},
		ExtraArgs:     []string{extraArg},
	}

	testKubelet, err := testKubeletConfig.New()
	if err != nil {
		t.Fatalf("Creating new kubelet should succeed, got: %v", err)
	}

	found := false

	hcc, err := testKubelet.ToHostConfiguredContainer()
	if err != nil {
		t.Fatalf("Converting kubelet to HostConfiguredContainer: %v", err)
	}

	for _, arg := range hcc.Container.Config.Args {
		if arg == extraArg {
			found = true
		}
	}

	if !found {
		t.Fatalf("Extra arguments should be included in generated arguments")
	}
}
