package network

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/yg-codes/proxmox-cli/pkg/api"
)

// Operations handles network-related API operations
type Operations struct {
	client *api.Client
	logger *logrus.Logger
}

// NewOperations creates a new network operations instance
func NewOperations(client *api.Client, logger *logrus.Logger) *Operations {
	return &Operations{
		client: client,
		logger: logger,
	}
}

// GetNetworkInterfaces lists all network interfaces on a node
// API: GET /nodes/{node}/network
func (ops *Operations) GetNetworkInterfaces(node string, filter *NetworkFilter) ([]*NetworkInterface, error) {
	ops.logger.Debugf("Fetching network interfaces for node %s", node)

	path := fmt.Sprintf("/nodes/%s/network", node)
	resp, err := ops.client.Get(path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get network interfaces: %w", err)
	}

	var interfaces []*NetworkInterface

	// Network API returns data in "items" key, not "data" key
	var items []interface{}
	if itemsData, ok := resp["items"].([]interface{}); ok {
		items = itemsData
	} else if itemsData, ok := resp["data"].([]interface{}); ok {
		// Fallback to "data" key for consistency with other endpoints
		items = itemsData
	}

	for _, item := range items {
		if ifMap, ok := item.(map[string]interface{}); ok {
			iface := parseNetworkInterface(ifMap, node)
			if matchesNetworkFilter(iface, filter) {
				interfaces = append(interfaces, iface)
			}
		}
	}

	ops.logger.Infof("Found %d network interfaces on node %s", len(interfaces), node)
	return interfaces, nil
}

// GetNetworkInterface gets details of a specific network interface
// API: GET /nodes/{node}/network/{iface}
func (ops *Operations) GetNetworkInterface(node, iface string) (*NetworkInterface, error) {
	ops.logger.Debugf("Fetching network interface %s on node %s", iface, node)

	path := fmt.Sprintf("/nodes/%s/network/%s", node, iface)
	resp, err := ops.client.Get(path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get network interface %s: %w", iface, err)
	}

	// Try "items" key first (network API format), then fallback to "data"
	if data, ok := resp["items"].(map[string]interface{}); ok {
		return parseNetworkInterface(data, node), nil
	} else if data, ok := resp["data"].(map[string]interface{}); ok {
		return parseNetworkInterface(data, node), nil
	} else {
		// Fallback: Get all interfaces and filter for the requested one
		ops.logger.Debug("Single interface endpoint returned unexpected format, falling back to list")
		interfaces, err := ops.GetNetworkInterfaces(node, &NetworkFilter{})
		if err != nil {
			return nil, err
		}

		for _, ifc := range interfaces {
			if ifc.Iface == iface {
				return ifc, nil
			}
		}

		return nil, fmt.Errorf("interface %s not found", iface)
	}
}

// CreateBridge creates a new bridge interface
// API: POST /nodes/{node}/network
func (ops *Operations) CreateBridge(opts *BridgeOptions) error {
	ops.logger.Infof("Creating bridge %s on node %s", opts.Iface, opts.Node)

	path := fmt.Sprintf("/nodes/%s/network", opts.Node)
	params := map[string]interface{}{
		"iface":        opts.Iface,
		"type":         "bridge",
		"autostart":    boolToInt(opts.Autostart),
		"bridge_ports": opts.BridgePorts,
	}

	if opts.Address != "" {
		params["address"] = opts.Address
	}
	if opts.Netmask != "" {
		params["netmask"] = opts.Netmask
	}
	if opts.Gateway != "" {
		params["gateway"] = opts.Gateway
	}
	if opts.Comments != "" {
		params["comments"] = opts.Comments
	}
	if opts.VLANAware {
		params["bridge_vlan_aware"] = 1
	}

	_, err := ops.client.Post(path, params)
	if err != nil {
		return fmt.Errorf("failed to create bridge: %w", err)
	}

	ops.logger.Infof("Bridge %s created successfully on node %s", opts.Iface, opts.Node)
	return nil
}

// UpdateNetworkInterface updates an existing network interface
// API: PUT /nodes/{node}/network/{iface}
func (ops *Operations) UpdateNetworkInterface(node, iface string, params map[string]interface{}) error {
	ops.logger.Infof("Updating network interface %s on node %s", iface, node)

	path := fmt.Sprintf("/nodes/%s/network/%s", node, iface)
	_, err := ops.client.Put(path, params)
	if err != nil {
		return fmt.Errorf("failed to update network interface %s: %w", iface, err)
	}

	ops.logger.Infof("Network interface %s updated successfully", iface)
	return nil
}

// DeleteNetworkInterface deletes a network interface
// API: DELETE /nodes/{node}/network/{iface}
func (ops *Operations) DeleteNetworkInterface(node, iface string) error {
	ops.logger.Infof("Deleting network interface %s on node %s", iface, node)

	path := fmt.Sprintf("/nodes/%s/network/%s", node, iface)
	_, err := ops.client.Delete(path)
	if err != nil {
		return fmt.Errorf("failed to delete network interface %s: %w", iface, err)
	}

	ops.logger.Infof("Network interface %s deleted successfully", iface)
	return nil
}

// ApplyNetworkConfig applies pending network configuration changes
// API: PUT /nodes/{node}/network
func (ops *Operations) ApplyNetworkConfig(node string) error {
	ops.logger.Infof("Applying network configuration on node %s", node)

	path := fmt.Sprintf("/nodes/%s/network", node)
	_, err := ops.client.Put(path, nil)
	if err != nil {
		return fmt.Errorf("failed to apply network configuration: %w", err)
	}

	ops.logger.Infof("Network configuration applied successfully on node %s", node)
	return nil
}

// RevertNetworkConfig reverts pending network configuration changes
// API: DELETE /nodes/{node}/network
func (ops *Operations) RevertNetworkConfig(node string) error {
	ops.logger.Infof("Reverting network configuration on node %s", node)

	path := fmt.Sprintf("/nodes/%s/network", node)
	_, err := ops.client.Delete(path)
	if err != nil {
		return fmt.Errorf("failed to revert network configuration: %w", err)
	}

	ops.logger.Infof("Network configuration reverted successfully on node %s", node)
	return nil
}

// GetSDNZones lists all SDN zones
// API: GET /cluster/sdn/zones
func (ops *Operations) GetSDNZones(filter *SDNFilter) ([]*SDNZone, error) {
	ops.logger.Debug("Fetching SDN zones")

	resp, err := ops.client.Get("/cluster/sdn/zones", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get SDN zones: %w", err)
	}

	var zones []*SDNZone

	if items, ok := resp["data"].([]interface{}); ok {
		for _, item := range items {
			if zoneMap, ok := item.(map[string]interface{}); ok {
				zone := parseSDNZone(zoneMap)
				if matchesSDNFilter(zone, filter) {
					zones = append(zones, zone)
				}
			}
		}
	}

	ops.logger.Infof("Found %d SDN zones", len(zones))
	return zones, nil
}

// GetSDNVNets lists all SDN virtual networks
// API: GET /cluster/sdn/vnets
func (ops *Operations) GetSDNVNets(zone string) ([]*SDNVNet, error) {
	ops.logger.Debug("Fetching SDN virtual networks")

	resp, err := ops.client.Get("/cluster/sdn/vnets", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get SDN vnets: %w", err)
	}

	var vnets []*SDNVNet

	if items, ok := resp["data"].([]interface{}); ok {
		for _, item := range items {
			if vnetMap, ok := item.(map[string]interface{}); ok {
				vnet := parseSDNVNet(vnetMap)
				if zone == "" || vnet.Zone == zone {
					vnets = append(vnets, vnet)
				}
			}
		}
	}

	ops.logger.Infof("Found %d SDN virtual networks", len(vnets))
	return vnets, nil
}

// GetFirewallRules gets firewall rules for a node
// API: GET /nodes/{node}/firewall/rules
func (ops *Operations) GetFirewallRules(node string) ([]*FirewallRule, error) {
	ops.logger.Debugf("Fetching firewall rules for node %s", node)

	path := fmt.Sprintf("/nodes/%s/firewall/rules", node)
	resp, err := ops.client.Get(path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get firewall rules: %w", err)
	}

	var rules []*FirewallRule

	if items, ok := resp["data"].([]interface{}); ok {
		for _, item := range items {
			if ruleMap, ok := item.(map[string]interface{}); ok {
				rules = append(rules, parseFirewallRule(ruleMap))
			}
		}
	}

	ops.logger.Infof("Found %d firewall rules for node %s", len(rules), node)
	return rules, nil
}

// Helper functions

func parseNetworkInterface(data map[string]interface{}, node string) *NetworkInterface {
	iface := &NetworkInterface{
		Node: node,
	}

	if v, ok := data["iface"].(string); ok {
		iface.Iface = v
	}
	if v, ok := data["type"].(string); ok {
		iface.Type = v
	}
	if v, ok := data["active"].(float64); ok {
		iface.Active = v == 1
	}
	if v, ok := data["autostart"].(float64); ok {
		iface.Autostart = v == 1
	}
	if v, ok := data["method"].(string); ok {
		iface.Method = v
	}
	if v, ok := data["method6"].(string); ok {
		iface.Method6 = v
	}
	if v, ok := data["address"].(string); ok {
		iface.Address = v
	}
	if v, ok := data["netmask"].(string); ok {
		iface.Netmask = v
	}
	if v, ok := data["gateway"].(string); ok {
		iface.Gateway = v
	}
	if v, ok := data["address6"].(string); ok {
		iface.Address6 = v
	}
	if v, ok := data["netmask6"].(float64); ok {
		iface.Netmask6 = int(v)
	}
	if v, ok := data["gateway6"].(string); ok {
		iface.Gateway6 = v
	}
	if v, ok := data["vlan-raw-device"].(string); ok {
		iface.VLANRawDevice = v
	}
	if v, ok := data["vlan-id"].(float64); ok {
		iface.VLANID = int(v)
	}
	if v, ok := data["bridge_ports"].(string); ok {
		iface.BridgePorts = v
	}
	if v, ok := data["bridge_stp"].(string); ok {
		iface.BridgeSTP = v
	}
	if v, ok := data["bridge_fd"].(string); ok {
		iface.BridgeFD = v
	}
	if v, ok := data["bridge_vlan_aware"].(float64); ok {
		iface.BridgeVLANAware = v == 1
	}
	if v, ok := data["bond_mode"].(string); ok {
		iface.BondMode = v
	}
	if v, ok := data["bond-primary"].(string); ok {
		iface.BondPrimary = v
	}
	if v, ok := data["slaves"].(string); ok {
		iface.Slaves = v
	}
	if v, ok := data["bond_xmit_hash_policy"].(string); ok {
		iface.BondXmitHashPolicy = v
	}
	if v, ok := data["comments"].(string); ok {
		iface.Comments = v
	}
	if v, ok := data["priority"].(float64); ok {
		iface.Priority = int(v)
	}
	if v, ok := data["exists"].(float64); ok {
		iface.Exists = v == 1
	}

	return iface
}

func parseSDNZone(data map[string]interface{}) *SDNZone {
	zone := &SDNZone{}

	if v, ok := data["zone"].(string); ok {
		zone.Zone = v
	}
	if v, ok := data["type"].(string); ok {
		zone.Type = v
	}
	if v, ok := data["nodes"].(string); ok {
		zone.Nodes = v
	}
	if v, ok := data["pending"].(float64); ok {
		zone.Pending = v == 1
	}
	if v, ok := data["state"].(string); ok {
		zone.State = v
	}
	if v, ok := data["bridge"].(string); ok {
		zone.Bridge = v
	}
	if v, ok := data["tag"].(float64); ok {
		zone.Tag = int(v)
	}
	if v, ok := data["peers"].(string); ok {
		zone.Peers = v
	}
	if v, ok := data["mtu"].(float64); ok {
		zone.MTU = int(v)
	}
	if v, ok := data["ipam"].(string); ok {
		zone.IPAM = v
	}
	if v, ok := data["dns"].(string); ok {
		zone.DNS = v
	}
	if v, ok := data["reversedns"].(string); ok {
		zone.ReverseDNS = v
	}
	if v, ok := data["dnszone"].(string); ok {
		zone.DNSZone = v
	}

	return zone
}

func parseSDNVNet(data map[string]interface{}) *SDNVNet {
	vnet := &SDNVNet{}

	if v, ok := data["vnet"].(string); ok {
		vnet.VNet = v
	}
	if v, ok := data["zone"].(string); ok {
		vnet.Zone = v
	}
	if v, ok := data["alias"].(string); ok {
		vnet.Alias = v
	}
	if v, ok := data["tag"].(float64); ok {
		vnet.Tag = int(v)
	}
	if v, ok := data["vlanaware"].(float64); ok {
		vnet.VLANAware = v == 1
	}
	if v, ok := data["pending"].(float64); ok {
		vnet.Pending = v == 1
	}
	if v, ok := data["state"].(string); ok {
		vnet.State = v
	}

	return vnet
}

func parseFirewallRule(data map[string]interface{}) *FirewallRule {
	rule := &FirewallRule{}

	if v, ok := data["pos"].(float64); ok {
		rule.Pos = int(v)
	}
	if v, ok := data["enable"].(float64); ok {
		rule.Enable = v == 1
	}
	if v, ok := data["type"].(string); ok {
		rule.Type = v
	}
	if v, ok := data["action"].(string); ok {
		rule.Action = v
	}
	if v, ok := data["macro"].(string); ok {
		rule.Macro = v
	}
	if v, ok := data["iface"].(string); ok {
		rule.Iface = v
	}
	if v, ok := data["source"].(string); ok {
		rule.Source = v
	}
	if v, ok := data["dest"].(string); ok {
		rule.Dest = v
	}
	if v, ok := data["proto"].(string); ok {
		rule.Proto = v
	}
	if v, ok := data["dport"].(string); ok {
		rule.Dport = v
	}
	if v, ok := data["sport"].(string); ok {
		rule.Sport = v
	}
	if v, ok := data["log"].(string); ok {
		rule.Log = v
	}
	if v, ok := data["comment"].(string); ok {
		rule.Comment = v
	}

	return rule
}

func matchesNetworkFilter(iface *NetworkInterface, filter *NetworkFilter) bool {
	if filter == nil {
		return true
	}

	if filter.Type != "" && iface.Type != filter.Type {
		return false
	}

	if filter.Active != nil && iface.Active != *filter.Active {
		return false
	}

	return true
}

func matchesSDNFilter(zone *SDNZone, filter *SDNFilter) bool {
	if filter == nil {
		return true
	}

	if filter.Type != "" && zone.Type != filter.Type {
		return false
	}

	if filter.State != "" && zone.State != filter.State {
		return false
	}

	return true
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
