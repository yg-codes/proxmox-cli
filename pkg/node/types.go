package node

import "time"

// Node represents a Proxmox cluster node with full details
type Node struct {
	Name   string
	Status string
	Online bool
	IP     string
	Level  string
	ID     string
	NodeID int

	// Resource metrics
	CPU        float64
	CPUUsage   float64
	MaxCPU     int
	Memory     int64
	MemoryUsed int64
	MemoryFree int64
	Disk       int64
	DiskUsed   int64
	DiskFree   int64
	Uptime     int64

	// Version info
	PVEVersion    string
	KernelVersion string
}

// Service represents a Proxmox node service
type Service struct {
	Name        string
	State       string
	Active      bool
	Running     bool
	Description string
	Enabled     bool
	UnitState   string
}

// NodeStats represents detailed node statistics
type NodeStats struct {
	Node          string
	CPUPercent    float64
	MemoryPercent float64
	DiskPercent   float64
	Uptime        time.Duration
	LoadAverage   []float64
}

// VersionInfo represents node version information
type VersionInfo struct {
	Version string
	Release string
	RepoID  string
	Kernel  string
}
