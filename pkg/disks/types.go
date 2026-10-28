package disks

// ZFSPool represents a ZFS pool as reported by /nodes/{node}/disks/zfs.
// Fields mirror the output of `zpool list -HpPLo` that Proxmox parses.
type ZFSPool struct {
	Node   string
	Name   string
	Size   int64   // Total pool size in bytes
	Alloc  int64   // Allocated bytes
	Free   int64   // Free bytes
	Frag   float64 // Fragmentation percentage
	Dedup  float64 // Dedup ratio
	Health string  // ONLINE, DEGRADED, FAULTED, etc.

	// UsePercent is derived (Alloc / Size * 100)
	UsePercent float64
}

// ZFSVdev represents a node in the ZFS pool device tree from
// /nodes/{node}/disks/zfs/{name}.
type ZFSVdev struct {
	Name     string
	Type     string // pool, mirror, raidz, disk, etc.
	State    string // ONLINE, DEGRADED, etc.
	Read     int64  // Read error count
	Write    int64  // Write error count
	Cksum    int64  // Checksum error count
	Msg      string // Optional status message
	Level    int    // Indentation level in the tree
	Children []*ZFSVdev
}

// ZFSPoolDetail is the full detail view for a single pool.
type ZFSPoolDetail struct {
	Node   string
	Name   string
	State  string
	Scan   string // Last scrub/resilver status text
	Errors string
	Root   *ZFSVdev // Device tree root
}

// Disk represents a physical disk from /nodes/{node}/disks/list.
type Disk struct {
	Node    string
	DevPath string // /dev/sdX
	Model   string
	Serial  string
	Size    int64  // Bytes
	Type    string // hdd, ssd, nvme, unknown
	Vendor  string
	Health  string // PASSED, FAILED, UNKNOWN (SMART overall)
	Wearout int    // SSD wearout percentage remaining (-1 if N/A)
	Used    string // Usage: LVM, ZFS, partitions, mounted, etc. ("" if unused)
	RPM     int64
	GPT     bool
}

// SMARTAttribute is a single S.M.A.R.T. attribute row.
type SMARTAttribute struct {
	ID        int
	Name      string
	Value     int
	Worst     int
	Threshold int
	Flags     string
	Failing   string
	Raw       string
}

// SMARTStatus is the SMART health view for one disk from
// /nodes/{node}/disks/smart.
type SMARTStatus struct {
	Node       string
	DevPath    string
	Health     string // PASSED, FAILED, UNKNOWN
	Type       string // ata, nvme, etc.
	Text       string // Raw smartctl text when attribute table is unavailable
	Attributes []*SMARTAttribute
}
