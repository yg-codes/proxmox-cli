package network

import (
	"fmt"
	"strings"
)

// DisplayNetworkInterfaces displays a list of network interfaces
func DisplayNetworkInterfaces(interfaces []*NetworkInterface) {
	if len(interfaces) == 0 {
		fmt.Println("❌ No network interfaces found")
		return
	}

	fmt.Println("\n🌐 Network Interfaces")
	fmt.Println(strings.Repeat("=", 140))
	fmt.Printf("%-15s %-12s %-10s %-10s %-18s %-18s %-15s %-30s\n",
		"Interface", "Type", "Active", "Autostart", "Address", "Gateway", "Method", "Bridge/Bond Info")
	fmt.Println(strings.Repeat("-", 140))

	for _, iface := range interfaces {
		// Status indicators
		active := "🔴 No"
		if iface.Active {
			active = "🟢 Yes"
		}
		autostart := "No"
		if iface.Autostart {
			autostart = "Yes"
		}

		// Additional info based on type
		extraInfo := ""
		switch iface.Type {
		case "bridge":
			extraInfo = fmt.Sprintf("Ports: %s", iface.BridgePorts)
			if iface.BridgeVLANAware {
				extraInfo += " (VLAN-aware)"
			}
		case "bond":
			extraInfo = fmt.Sprintf("Mode: %s, Slaves: %s", iface.BondMode, iface.Slaves)
		case "vlan":
			extraInfo = fmt.Sprintf("Device: %s, VLAN ID: %d", iface.VLANRawDevice, iface.VLANID)
		}

		fmt.Printf("%-15s %-12s %-10s %-10s %-18s %-18s %-15s %-30s\n",
			iface.Iface,
			iface.Type,
			active,
			autostart,
			iface.Address,
			iface.Gateway,
			iface.Method,
			extraInfo,
		)
	}

	fmt.Println(strings.Repeat("=", 140))
	fmt.Printf("Total: %d interfaces\n\n", len(interfaces))
}

// DisplayInterfaceDetails displays detailed information about a specific network interface
func DisplayInterfaceDetails(iface *NetworkInterface) {
	fmt.Println("\n🌐 Network Interface Details")
	fmt.Println(strings.Repeat("=", 80))

	// Basic info
	fmt.Printf("Interface:    %s\n", iface.Iface)
	fmt.Printf("Node:         %s\n", iface.Node)
	fmt.Printf("Type:         %s\n", iface.Type)

	active := "🔴 Inactive"
	if iface.Active {
		active = "🟢 Active"
	}
	fmt.Printf("Status:       %s\n", active)

	autostart := "No"
	if iface.Autostart {
		autostart = "Yes"
	}
	fmt.Printf("Autostart:    %s\n", autostart)

	// IPv4 configuration
	if iface.Address != "" || iface.Method != "" {
		fmt.Println("\nIPv4 Configuration:")
		fmt.Printf("  Method:     %s\n", iface.Method)
		if iface.Address != "" {
			fmt.Printf("  Address:    %s\n", iface.Address)
		}
		if iface.Netmask != "" {
			fmt.Printf("  Netmask:    %s\n", iface.Netmask)
		}
		if iface.Gateway != "" {
			fmt.Printf("  Gateway:    %s\n", iface.Gateway)
		}
	}

	// IPv6 configuration
	if iface.Address6 != "" || iface.Method6 != "" {
		fmt.Println("\nIPv6 Configuration:")
		fmt.Printf("  Method:     %s\n", iface.Method6)
		if iface.Address6 != "" {
			fmt.Printf("  Address:    %s/%d\n", iface.Address6, iface.Netmask6)
		}
		if iface.Gateway6 != "" {
			fmt.Printf("  Gateway:    %s\n", iface.Gateway6)
		}
	}

	// Type-specific configuration
	switch iface.Type {
	case "bridge":
		fmt.Println("\nBridge Configuration:")
		fmt.Printf("  Ports:      %s\n", iface.BridgePorts)
		if iface.BridgeSTP != "" {
			fmt.Printf("  STP:        %s\n", iface.BridgeSTP)
		}
		if iface.BridgeFD != "" {
			fmt.Printf("  Forward Delay: %s\n", iface.BridgeFD)
		}
		vlanAware := "No"
		if iface.BridgeVLANAware {
			vlanAware = "Yes"
		}
		fmt.Printf("  VLAN Aware: %s\n", vlanAware)

	case "bond":
		fmt.Println("\nBond Configuration:")
		fmt.Printf("  Mode:       %s\n", iface.BondMode)
		fmt.Printf("  Slaves:     %s\n", iface.Slaves)
		if iface.BondPrimary != "" {
			fmt.Printf("  Primary:    %s\n", iface.BondPrimary)
		}
		if iface.BondXmitHashPolicy != "" {
			fmt.Printf("  Hash Policy: %s\n", iface.BondXmitHashPolicy)
		}

	case "vlan":
		fmt.Println("\nVLAN Configuration:")
		fmt.Printf("  Raw Device: %s\n", iface.VLANRawDevice)
		fmt.Printf("  VLAN ID:    %d\n", iface.VLANID)
	}

	if iface.Comments != "" {
		fmt.Printf("\nComments:     %s\n", iface.Comments)
	}

	fmt.Println(strings.Repeat("=", 80))
}

// DisplayNetworkSummary displays a summary of network interfaces by type
func DisplayNetworkSummary(interfaces []*NetworkInterface, node string) {
	if len(interfaces) == 0 {
		fmt.Println("❌ No network interfaces found")
		return
	}

	fmt.Printf("\n🌐 Network Summary for Node: %s\n", node)
	fmt.Println(strings.Repeat("=", 80))

	// Count by type
	counts := make(map[string]int)
	activeCount := 0
	autostartCount := 0

	for _, iface := range interfaces {
		counts[iface.Type]++
		if iface.Active {
			activeCount++
		}
		if iface.Autostart {
			autostartCount++
		}
	}

	fmt.Printf("Total Interfaces:    %d\n", len(interfaces))
	fmt.Printf("Active Interfaces:   %d (🟢 %d active, 🔴 %d inactive)\n",
		len(interfaces), activeCount, len(interfaces)-activeCount)
	fmt.Printf("Autostart Enabled:   %d\n", autostartCount)

	fmt.Println("\nBy Type:")
	for ifType, count := range counts {
		icon := getTypeIcon(ifType)
		fmt.Printf("  %s %-10s: %d\n", icon, ifType, count)
	}

	fmt.Println(strings.Repeat("=", 80))
}

// DisplaySDNZones displays a list of SDN zones
func DisplaySDNZones(zones []*SDNZone) {
	if len(zones) == 0 {
		fmt.Println("❌ No SDN zones found")
		return
	}

	fmt.Println("\n🔷 SDN Zones")
	fmt.Println(strings.Repeat("=", 100))
	fmt.Printf("%-20s %-15s %-20s %-15s %-20s\n",
		"Zone", "Type", "Nodes", "State", "Configuration")
	fmt.Println(strings.Repeat("-", 100))

	for _, zone := range zones {
		state := zone.State
		if zone.Pending {
			state += " (pending)"
		}

		config := ""
		switch zone.Type {
		case "vlan":
			config = fmt.Sprintf("Bridge: %s, Tag: %d", zone.Bridge, zone.Tag)
		case "vxlan":
			config = fmt.Sprintf("Peers: %s, MTU: %d", zone.Peers, zone.MTU)
		}

		fmt.Printf("%-20s %-15s %-20s %-15s %-20s\n",
			zone.Zone,
			zone.Type,
			zone.Nodes,
			state,
			config,
		)
	}

	fmt.Println(strings.Repeat("=", 100))
	fmt.Printf("Total: %d SDN zones\n\n", len(zones))
}

// DisplaySDNVNets displays a list of SDN virtual networks
func DisplaySDNVNets(vnets []*SDNVNet) {
	if len(vnets) == 0 {
		fmt.Println("❌ No SDN virtual networks found")
		return
	}

	fmt.Println("\n🔷 SDN Virtual Networks")
	fmt.Println(strings.Repeat("=", 100))
	fmt.Printf("%-20s %-20s %-20s %-10s %-15s\n",
		"VNet", "Zone", "Alias", "Tag", "State")
	fmt.Println(strings.Repeat("-", 100))

	for _, vnet := range vnets {
		state := vnet.State
		if vnet.Pending {
			state += " (pending)"
		}

		vlanAware := ""
		if vnet.VLANAware {
			vlanAware = " [VLAN-aware]"
		}

		fmt.Printf("%-20s %-20s %-20s %-10d %-15s%s\n",
			vnet.VNet,
			vnet.Zone,
			vnet.Alias,
			vnet.Tag,
			state,
			vlanAware,
		)
	}

	fmt.Println(strings.Repeat("=", 100))
	fmt.Printf("Total: %d SDN virtual networks\n\n", len(vnets))
}

// DisplayFirewallRules displays a list of firewall rules
func DisplayFirewallRules(rules []*FirewallRule, node string) {
	if len(rules) == 0 {
		fmt.Printf("❌ No firewall rules found for node %s\n", node)
		return
	}

	fmt.Printf("\n🔥 Firewall Rules for Node: %s\n", node)
	fmt.Println(strings.Repeat("=", 120))
	fmt.Printf("%-5s %-8s %-6s %-10s %-15s %-15s %-10s %-15s %-30s\n",
		"Pos", "Enabled", "Type", "Action", "Source", "Dest", "Proto", "Ports", "Comment")
	fmt.Println(strings.Repeat("-", 120))

	for _, rule := range rules {
		enabled := "🔴 No"
		if rule.Enable {
			enabled = "🟢 Yes"
		}

		ports := ""
		if rule.Dport != "" {
			ports = fmt.Sprintf("D:%s", rule.Dport)
		}
		if rule.Sport != "" {
			if ports != "" {
				ports += " "
			}
			ports += fmt.Sprintf("S:%s", rule.Sport)
		}

		fmt.Printf("%-5d %-8s %-6s %-10s %-15s %-15s %-10s %-15s %-30s\n",
			rule.Pos,
			enabled,
			rule.Type,
			rule.Action,
			rule.Source,
			rule.Dest,
			rule.Proto,
			ports,
			rule.Comment,
		)
	}

	fmt.Println(strings.Repeat("=", 120))
	fmt.Printf("Total: %d firewall rules\n\n", len(rules))
}

// Helper functions

func getTypeIcon(ifType string) string {
	icons := map[string]string{
		"bridge":  "🌉",
		"bond":    "🔗",
		"eth":     "🔌",
		"vlan":    "🏷️",
		"vmbr":    "🌉",
		"unknown": "❓",
	}

	if icon, ok := icons[ifType]; ok {
		return icon
	}
	return icons["unknown"]
}
