package resource

import "time"

// ClusterResources represents cluster-wide resource information
type ClusterResources struct {
	Nodes   []*NodeResource
	VMs     []*VMResource
	Storage []*StorageResource
}

// NodeResource represents node resource usage
type NodeResource struct {
	Node   string
	Type   string
	Status string
	Online bool

	// CPU metrics
	CPU        float64 // Current CPU usage (0-1)
	MaxCPU     int     // Number of CPU cores
	CPUPercent float64 // CPU usage percentage

	// Memory metrics
	Memory     int64   // Used memory in bytes
	MaxMemory  int64   // Total memory in bytes
	MemPercent float64 // Memory usage percentage

	// Disk metrics
	Disk        int64   // Used disk in bytes
	MaxDisk     int64   // Total disk in bytes
	DiskPercent float64 // Disk usage percentage

	// Network metrics
	NetIn  int64 // Network input in bytes
	NetOut int64 // Network output in bytes

	// System info
	Uptime        int64
	LoadAverage   []float64 // 1, 5, 15 minute load averages
	KernelVersion string
	PVEVersion    string
}

// VMResource represents VM resource usage
type VMResource struct {
	VMID   int
	Name   string
	Node   string
	Type   string // qemu or lxc
	Status string

	// CPU metrics
	CPU        float64 // Current CPU usage (0-1)
	MaxCPU     int     // Number of vCPUs
	CPUPercent float64

	// Memory metrics
	Memory     int64 // Used memory in bytes
	MaxMemory  int64 // Allocated memory in bytes
	MemPercent float64

	// Disk metrics
	Disk        int64 // Used disk in bytes
	MaxDisk     int64 // Allocated disk in bytes
	DiskPercent float64

	// Network metrics
	NetIn  int64 // Network input in bytes
	NetOut int64 // Network output in bytes

	// Runtime info
	Uptime int64
	PID    int
}

// StorageResource represents storage resource usage
type StorageResource struct {
	Storage string
	Node    string
	Type    string
	Content string
	Status  string
	Active  bool

	// Disk metrics
	Used       int64   // Used space in bytes
	Total      int64   // Total space in bytes
	Available  int64   // Available space in bytes
	UsePercent float64 // Usage percentage
}

// ResourceHistory represents historical resource data
type ResourceHistory struct {
	Time    time.Time
	CPU     float64
	Memory  float64
	Disk    float64
	NetIn   float64
	NetOut  float64
	IODelay float64
	LoadAvg float64
}

// ResourceStats represents aggregated statistics
type ResourceStats struct {
	// Cluster totals
	TotalNodes  int
	OnlineNodes int
	TotalVMs    int
	RunningVMs  int
	StoppedVMs  int

	// Cluster resources
	TotalCPU   int
	UsedCPU    float64
	CPUPercent float64

	TotalMemory   int64
	UsedMemory    int64
	MemoryPercent float64

	TotalDisk   int64
	UsedDisk    int64
	DiskPercent float64

	// Storage stats
	TotalStorage     int
	AvailableStorage int
	UsedStorage      int64
	TotalStorageSize int64
	StoragePercent   float64
}

// NodeCapacity holds per-node capacity/overcommit figures derived from
// live cluster resources. RAM figures are ALLOCATED (VM maxmem), not actual
// in-guest used memory — that requires an external metrics server.
type NodeCapacity struct {
	Node          string
	Online        bool
	PhysCPU       int   // Physical CPU cores (node maxcpu)
	AllocVCPU     int   // Sum of vCPUs across VMs on this node
	PhysMemory    int64 // Node physical RAM (bytes)
	AllocMemory   int64 // Sum of VM allocated RAM on this node (bytes)
	RunningVMs    int
	TotalVMs      int
	CPUOvercommit float64 // AllocVCPU / PhysCPU
	MemOvercommit float64 // AllocMemory / PhysMemory
}

// CapacityAnalysis is the cluster-wide capacity/overcommit report plus an
// N-1 failover verdict (can the cluster hold all running-VM allocated RAM if
// the single largest node is lost, while keeping quorum?).
type CapacityAnalysis struct {
	Nodes []*NodeCapacity

	TotalPhysCPU     int
	TotalAllocVCPU   int
	TotalPhysMemory  int64
	TotalAllocMemory int64
	ClusterCPUOC     float64
	ClusterMemOC     float64

	// N-1 failover. Computed over ONLINE nodes only: an already-offline node
	// can neither host failover VMs nor be "lost" as a fresh failure.
	LargestNode        string // Largest ONLINE node (worst-case to lose)
	SurvivingPhysMem   int64  // Online nodes' RAM minus the largest online node
	AllocMemToPlace    int64  // Running-VM allocated RAM that needs a home
	N1MemFits          bool
	QuorumAfterFailure bool // (TotalNodes-1) still a quorum majority
	TotalNodes         int
	OfflineNodes       int  // Nodes already offline at query time
	N1Degraded         bool // True if any node is offline (verdict is best-effort)
}

// HAState describes the result of probing the HA status endpoint, so callers
// can distinguish "no HA configured" from "couldn't read it".
type HAState int

const (
	HAUnknown       HAState = iota // Not yet probed
	HAConfigured                   // Endpoint reachable; 0+ resources listed
	HANotConfigured                // Endpoint reachable, empty result
	HAForbidden                    // 403 — token lacks HA.Audit
	HAUnavailable                  // Transient/non-403 error
)

// ClusterHealth is a REST-only health snapshot: quorum, node online/offline,
// HA resource state, and storage availability. It is NOT a substitute for
// on-node checks (journalctl, pvecm, ha-manager, zpool status, replication),
// which require SSH the CLI does not have.
type ClusterHealth struct {
	Quorate     bool
	TotalNodes  int
	OnlineNodes int
	NodeStates  []ClusterNodeState
	HAResources []HAResourceState
	HAState     HAState

	StorageTotal     int
	StorageAvailable int
}

// ClusterNodeState is a node's membership/online state from /cluster/status.
type ClusterNodeState struct {
	Name   string
	Online bool
	Local  bool
}

// HAResourceState is one HA-managed resource from /cluster/ha/status/current.
type HAResourceState struct {
	ID    string // e.g. "vm:101"
	Node  string
	State string // started, stopped, error, etc.
}

// RRDData represents RRD (Round Robin Database) data for graphs
type RRDData struct {
	TimeFrame  string // hour, day, week, month, year
	DataPoints []*ResourceHistory
}

// ResourceFilter for filtering resources
type ResourceFilter struct {
	Type   string // node, vm, storage, qemu, lxc
	Node   string // Filter by specific node
	Status string // running, stopped, online, offline
}
