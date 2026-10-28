package network

// NetworkInterface represents a network interface
type NetworkInterface struct {
	Node      string
	Iface     string
	Type      string // bridge, bond, eth, vlan, etc.
	Active    bool
	Autostart bool
	Method    string // static, manual, dhcp
	Method6   string // static, manual, dhcp

	// IP configuration
	Address  string
	Netmask  string
	Gateway  string
	Address6 string
	Netmask6 int
	Gateway6 string

	// VLAN configuration
	VLANRawDevice string
	VLANID        int

	// Bridge configuration
	BridgePorts     string
	BridgeSTP       string
	BridgeFD        string
	BridgeVLANAware bool

	// Bond configuration
	BondMode           string
	BondPrimary        string
	Slaves             string
	BondXmitHashPolicy string

	// Additional info
	Comments string
	Priority int
	Families []string
	Exists   bool
}

// NetworkConfig represents network configuration for a node
type NetworkConfig struct {
	Node       string
	Interfaces []*NetworkInterface
	DNS        DNSConfig
}

// DNSConfig represents DNS configuration
type DNSConfig struct {
	DNS1   string
	DNS2   string
	DNS3   string
	Domain string
	Search string
}

// SDNZone represents a Software Defined Network zone
type SDNZone struct {
	Zone    string
	Type    string // vlan, vxlan, qinq, simple
	Nodes   string
	Pending bool
	State   string

	// VLAN zone fields
	Bridge string
	Tag    int

	// VXLAN zone fields
	Peers string
	MTU   int

	// Common fields
	IPAM       string
	DNS        string
	ReverseDNS string
	DNSZone    string
}

// SDNVNet represents a virtual network in an SDN zone
type SDNVNet struct {
	VNet      string
	Zone      string
	Alias     string
	Tag       int
	VLANAware bool

	// State
	Pending bool
	State   string
}

// SDNSubnet represents a subnet in an SDN virtual network
type SDNSubnet struct {
	Subnet  string
	VNet    string
	Type    string // subnet
	Gateway string
	Snat    bool
	Dhcp    string // range or none
}

// Firewall represents node or cluster firewall configuration
type Firewall struct {
	Enable                               bool
	LogLevelIn                           string
	LogLevelOut                          string
	Nf_conntrack_max                     int
	Nf_conntrack_tcp_timeout_established int
	Nf_conntrack_tcp_timeout_syn_recv    int
}

// FirewallRule represents a firewall rule
type FirewallRule struct {
	Pos     int
	Enable  bool
	Type    string // in, out, group
	Action  string // ACCEPT, DROP, REJECT
	Macro   string
	Iface   string
	Source  string
	Dest    string
	Proto   string
	Dport   string
	Sport   string
	Log     string
	Comment string
}

// IPSet represents an IP set for firewall rules
type IPSet struct {
	Name    string
	Comment string
	Digest  string
	CIDRs   []string
}

// SecurityGroup represents a firewall security group
type SecurityGroup struct {
	Group   string
	Comment string
	Digest  string
	Rules   []*FirewallRule
}

// NetworkFilter for filtering network interfaces
type NetworkFilter struct {
	Node   string
	Type   string // bridge, bond, eth, vlan
	Active *bool
}

// SDNFilter for filtering SDN resources
type SDNFilter struct {
	Zone  string
	Type  string
	State string
}

// BridgeOptions for creating/updating bridges
type BridgeOptions struct {
	Node        string
	Iface       string
	Type        string
	Autostart   bool
	BridgePorts string
	Address     string
	Netmask     string
	Gateway     string
	Comments    string
	VLANAware   bool
}

// BondOptions for creating/updating bonds
type BondOptions struct {
	Node      string
	Iface     string
	Autostart bool
	Slaves    string
	BondMode  string
	Address   string
	Netmask   string
	Gateway   string
	Comments  string
}

// VLANOptions for creating/updating VLANs
type VLANOptions struct {
	Node          string
	Iface         string
	VLANRawDevice string
	VLANID        int
	Autostart     bool
	Address       string
	Netmask       string
	Comments      string
}
