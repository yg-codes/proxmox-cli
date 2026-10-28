package container

import "time"

// Container represents an LXC container
type Container struct {
	VMID   int
	Name   string
	Node   string
	Status string
	Type   string // Always "lxc"

	// Resource allocation
	CPUs   int
	Memory int64 // in bytes
	Swap   int64 // in bytes
	Disk   int64 // in bytes

	// Runtime info
	Uptime int64
	PID    int

	// Network
	NetIn  int64
	NetOut int64

	// Configuration
	OSType      string
	Arch        string
	Hostname    string
	Description string
	Protected   bool
	Template    bool
	Lock        string

	// Storage
	RootFS string
}

// ContainerConfig represents container configuration
type ContainerConfig struct {
	VMID        int
	Name        string
	Node        string
	OSTemplate  string
	RootFS      string
	Hostname    string
	Description string
	Protected   bool

	// Resources
	CPUs   int
	Memory int64
	Swap   int64
	Disk   int64

	// Network
	Network []NetworkConfig

	// Storage
	Storage string

	// Startup
	Startup     string
	OnBoot      bool
	StartOnBoot bool

	// Features
	Nesting   bool
	KeyCTL    bool
	Fuse      bool
	MountCIFS bool
	MountNFS  bool

	// Password/SSH
	Password string
	SSHKeys  string

	// Unprivileged
	Unprivileged bool
}

// NetworkConfig represents container network configuration
type NetworkConfig struct {
	Name     string
	Bridge   string
	IP       string
	IP6      string
	Gateway  string
	Gateway6 string
	VLAN     int
	Firewall bool
	Rate     float64 // MB/s
}

// ContainerSnapshot represents a container snapshot
type ContainerSnapshot struct {
	Name        string
	Description string
	SnapTime    time.Time
	Parent      string
	VMState     bool
}

// ContainerStatus represents detailed container status
type ContainerStatus struct {
	VMID   int
	Name   string
	Node   string
	Status string
	Uptime int64
	PID    int

	// Resource usage
	CPU        float64
	CPUPercent float64
	MaxCPU     int
	Memory     int64
	MaxMemory  int64
	MemPercent float64
	Swap       int64
	MaxSwap    int64
	Disk       int64
	MaxDisk    int64

	// Network
	NetIn  int64
	NetOut int64

	// HA status
	HAState string
}

// ContainerBackup represents a container backup
type ContainerBackup struct {
	VolID        string
	Format       string
	Size         int64
	CreatedTime  time.Time
	VMID         int
	Note         string
	Protected    bool
	Verification string
}

// MountPoint represents a container mount point
type MountPoint struct {
	ID     string
	Volume string
	Size   string
	MP     string
	ACL    bool
	Backup bool
	Quota  bool
	RO     bool
	Shared bool
}

// ContainerFeatures represents enabled container features
type ContainerFeatures struct {
	Nesting   bool
	KeyCTL    bool
	Fuse      bool
	MountCIFS bool
	MountNFS  bool
}

// ContainerTemplate represents an available OS template
type ContainerTemplate struct {
	VolID       string
	Format      string
	Size        int64
	Description string
}

// ContainerFilter for filtering containers
type ContainerFilter struct {
	Node     string
	Status   string // running, stopped
	Template bool
	Name     string
}

// ContainerCloneOptions for cloning containers
type ContainerCloneOptions struct {
	NewID       int
	Hostname    string
	Description string
	Pool        string
	SnapName    string
	Storage     string
	Target      string
	Full        bool
}

// ContainerRestoreOptions for restoring containers
type ContainerRestoreOptions struct {
	VMID         int
	Node         string
	BackupFile   string
	Storage      string
	Pool         string
	Force        bool
	Unprivileged bool
}
