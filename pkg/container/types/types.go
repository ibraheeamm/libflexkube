package types

// To avoid cyclic dependencies between container, runtime and runtime implementation packages,
// we put container-related types in separated package.

// ContainerConfig stores runtime-agnostic information how to run the container
type ContainerConfig struct {
	Name       string    `json:"name" yaml:"name"`
	Image      string    `json:"image" yaml:"image"`
	Args       []string  `json:"args,omitempty" yaml:"args,omitempty"`
	Entrypoint []string  `json:"entrypoint,omitempty" yaml:"entrypoint,omitempty"`
	Ports      []PortMap `json:"ports,omitempty" yaml:"ports,omitempty"`
	Mounts     []Mount   `json:"mounts,omitempty" yaml:"mounts,omitempty"`
}

// ContainerStatus stores status information received from the runtime
// TODO this should cover all fields which are defined in ContainerConfig,
// so we can read and compare if actual configuration matches our expectations.
type ContainerStatus struct {
	Image  string `json:"image" yaml:"image"`
	ID     string `json:"id" yaml:"id"`
	Name   string `json:"name", yaml:"name"`
	Status string `json:"status", yaml:"status"`
}

// PortMap is basically a github.com/docker/go-connections/nat.PortMap
// TODO Once we introduce Kubelet runtime, we need to figure out how to structure it
type PortMap struct {
	IP       string `json:"ip" yaml:"ip"`
	Port     int    `json:"port" yaml:"port"`
	Protocol string `json:"protocol" yaml:"protocol"`
}

// Mount describe host bind mount
// TODO Same as PortMap
type Mount struct {
	Source string `json:"source" yaml:"source"`
	Target string `json:"target" yaml:"target"`
}
