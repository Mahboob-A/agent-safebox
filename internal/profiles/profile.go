package profiles

// Profile represents the declarative configuration for an agent.
type Profile struct {
	Binary          BinarySpec
	Paths           PathSpec
	PersistentState PersistentStateSpec
	Network         NetworkSpec
}

// BinarySpec defines how the binary is matched.
type BinarySpec struct {
	Name string // Substring matched against filepath.Base(argv0)
}

// PathSpec defines file and directory access rules.
type PathSpec struct {
	AllowRO      []string // Directory paths with read-only access
	AllowRW      []string // Directory paths with read-write access
	AllowRWFiles []string // Single file paths with read-write access
}

// PersistentStateSpec defines state directory bind-mount mapping.
type PersistentStateSpec struct {
	HostDir string // Host directory path under $XDG_STATE_HOME/safebox/agents/<tool>
	MountAt string // Mount destination inside sandbox mount namespace
}

// NetworkSpec defines selective outbound network access policies.
//
// In v1, only the binary AllowNet field is honored at runtime. AllowDomains
// is preserved as a dormant seam for v0.5+ domain-restricted egress via
// TLS-SNI inspection (deferred). Do not delete without coordinating with
// the v0.5 plan.
type NetworkSpec struct {
	AllowNet     bool     // v1 binary toggle: true = full internet egress via userspace NAT
	AllowDomains []string // Allowed domain names for userspace NAT egress (dormant in v1; v0.5 SNI)
}
