package resource

import (
	"fmt"
	"strings"
)

// DisplayClusterStats shows aggregated cluster statistics
func DisplayClusterStats(stats *ResourceStats) {
	fmt.Println("\n📊 Cluster Resource Summary")
	fmt.Println(strings.Repeat("=", 100))

	// Nodes
	fmt.Printf("Nodes:          %d total (%d online, %d offline)\n",
		stats.TotalNodes, stats.OnlineNodes, stats.TotalNodes-stats.OnlineNodes)

	// VMs
	fmt.Printf("VMs:            %d total (%d running, %d stopped)\n",
		stats.TotalVMs, stats.RunningVMs, stats.StoppedVMs)

	// Storage
	fmt.Printf("Storage:        %d total (%d available)\n",
		stats.TotalStorage, stats.AvailableStorage)

	fmt.Println(strings.Repeat("-", 100))

	// CPU
	fmt.Printf("CPU:            %.1f / %d cores (%.1f%%)\n",
		stats.UsedCPU, stats.TotalCPU, stats.CPUPercent)
	displayBar("CPU", stats.CPUPercent)

	// Memory
	fmt.Printf("Memory:         %s / %s (%.1f%%)\n",
		formatBytes(stats.UsedMemory),
		formatBytes(stats.TotalMemory),
		stats.MemoryPercent)
	displayBar("Memory", stats.MemoryPercent)

	// Disk
	fmt.Printf("Disk:           %s / %s (%.1f%%)\n",
		formatBytes(stats.UsedDisk),
		formatBytes(stats.TotalDisk),
		stats.DiskPercent)
	displayBar("Disk", stats.DiskPercent)

	// Storage
	if stats.TotalStorageSize > 0 {
		fmt.Printf("Storage Total:  %s / %s (%.1f%%)\n",
			formatBytes(stats.UsedStorage),
			formatBytes(stats.TotalStorageSize),
			stats.StoragePercent)
		displayBar("Storage", stats.StoragePercent)
	}

	fmt.Println(strings.Repeat("=", 100))
}

// DisplayNodeResources shows resource usage for nodes
func DisplayNodeResources(nodes []*NodeResource) {
	if len(nodes) == 0 {
		fmt.Println("❌ No nodes found")
		return
	}

	fmt.Println("\n🖥️  Node Resources")
	fmt.Println(strings.Repeat("=", 140))
	fmt.Printf("%-15s %-10s %-15s %-15s %-15s %-20s %-10s\n",
		"Node", "Status", "CPU", "Memory", "Disk", "Network (In/Out)", "Uptime")
	fmt.Println(strings.Repeat("-", 140))

	for _, node := range nodes {
		status := "🔴 offline"
		if node.Online {
			status = "🟢 online"
		}

		cpu := fmt.Sprintf("%.1f%% (%d)", node.CPUPercent, node.MaxCPU)
		memory := fmt.Sprintf("%.1f%% %s", node.MemPercent, formatBytes(node.Memory))
		disk := fmt.Sprintf("%.1f%% %s", node.DiskPercent, formatBytes(node.Disk))
		network := fmt.Sprintf("%s/%s", formatBytes(node.NetIn), formatBytes(node.NetOut))
		uptime := formatUptime(node.Uptime)

		fmt.Printf("%-15s %-10s %-15s %-15s %-15s %-20s %-10s\n",
			truncate(node.Node, 15),
			status,
			cpu,
			memory,
			disk,
			network,
			uptime)
	}

	fmt.Println(strings.Repeat("-", 140))
	fmt.Printf("Total nodes: %d\n", len(nodes))
}

// DisplayVMResources shows resource usage for VMs
func DisplayVMResources(vms []*VMResource) {
	if len(vms) == 0 {
		fmt.Println("❌ No VMs found")
		return
	}

	fmt.Println("\n💻 VM Resources")
	fmt.Println(strings.Repeat("=", 140))
	fmt.Printf("%-6s %-20s %-15s %-8s %-10s %-15s %-15s %-15s %-10s\n",
		"VMID", "Name", "Node", "Type", "Status", "CPU", "Memory", "Disk", "Uptime")
	fmt.Println(strings.Repeat("-", 140))

	for _, vm := range vms {
		status := "🔴 stopped"
		if vm.Status == "running" {
			status = "🟢 running"
		}

		cpu := fmt.Sprintf("%.1f%% (%d)", vm.CPUPercent, vm.MaxCPU)
		memory := fmt.Sprintf("%.1f%% %s", vm.MemPercent, formatBytes(vm.Memory))
		disk := formatBytes(vm.Disk)
		uptime := "-"
		if vm.Status == "running" {
			uptime = formatUptime(vm.Uptime)
		}

		fmt.Printf("%-6d %-20s %-15s %-8s %-10s %-15s %-15s %-15s %-10s\n",
			vm.VMID,
			truncate(vm.Name, 20),
			truncate(vm.Node, 15),
			vm.Type,
			status,
			cpu,
			memory,
			disk,
			uptime)
	}

	fmt.Println(strings.Repeat("-", 140))
	fmt.Printf("Total VMs: %d\n", len(vms))
}

// DisplayStorageResources shows resource usage for storage
func DisplayStorageResources(storages []*StorageResource) {
	if len(storages) == 0 {
		fmt.Println("❌ No storage found")
		return
	}

	fmt.Println("\n💾 Storage Resources")
	fmt.Println(strings.Repeat("=", 120))
	fmt.Printf("%-20s %-15s %-10s %-20s %-10s %-15s %-15s\n",
		"Storage", "Node", "Type", "Content", "Status", "Used", "Total")
	fmt.Println(strings.Repeat("-", 120))

	for _, storage := range storages {
		status := "🔴 inactive"
		if storage.Active {
			status = "🟢 active"
		}

		used := fmt.Sprintf("%.1f%% %s", storage.UsePercent, formatBytes(storage.Used))
		total := formatBytes(storage.Total)

		fmt.Printf("%-20s %-15s %-10s %-20s %-10s %-15s %-15s\n",
			truncate(storage.Storage, 20),
			truncate(storage.Node, 15),
			truncate(storage.Type, 10),
			truncate(storage.Content, 20),
			status,
			used,
			total)
	}

	fmt.Println(strings.Repeat("-", 120))
	fmt.Printf("Total storage: %d\n", len(storages))
}

// DisplayNodeDetails shows detailed resource information for a single node
func DisplayNodeDetails(node *NodeResource) {
	fmt.Printf("\n🖥️  Node Details: %s\n", node.Node)
	fmt.Println(strings.Repeat("=", 80))

	fmt.Printf("Status:      %s\n", node.Status)
	fmt.Printf("Type:        %s\n", node.Type)
	fmt.Println()

	// CPU
	fmt.Printf("CPU Usage:   %.2f / %d cores (%.1f%%)\n",
		node.CPU, node.MaxCPU, node.CPUPercent)
	displayBar("  ", node.CPUPercent)

	// Memory
	fmt.Printf("Memory:      %s / %s (%.1f%%)\n",
		formatBytes(node.Memory),
		formatBytes(node.MaxMemory),
		node.MemPercent)
	displayBar("  ", node.MemPercent)

	// Disk
	fmt.Printf("Disk:        %s / %s (%.1f%%)\n",
		formatBytes(node.Disk),
		formatBytes(node.MaxDisk),
		node.DiskPercent)
	displayBar("  ", node.DiskPercent)

	fmt.Println()

	// Network
	fmt.Printf("Network In:  %s\n", formatBytes(node.NetIn))
	fmt.Printf("Network Out: %s\n", formatBytes(node.NetOut))

	// Uptime
	fmt.Printf("Uptime:      %s\n", formatUptime(node.Uptime))

	// Versions
	if node.PVEVersion != "" {
		fmt.Printf("PVE Version: %s\n", node.PVEVersion)
	}
	if node.KernelVersion != "" {
		fmt.Printf("Kernel:      %s\n", node.KernelVersion)
	}

	fmt.Println(strings.Repeat("=", 80))
}

// DisplayVMDetails shows detailed resource information for a single VM
func DisplayVMDetails(vm *VMResource) {
	fmt.Printf("\n💻 VM Details: %d (%s)\n", vm.VMID, vm.Name)
	fmt.Println(strings.Repeat("=", 80))

	fmt.Printf("Node:        %s\n", vm.Node)
	fmt.Printf("Type:        %s\n", vm.Type)
	fmt.Printf("Status:      %s\n", vm.Status)
	if vm.PID > 0 {
		fmt.Printf("PID:         %d\n", vm.PID)
	}
	fmt.Println()

	// CPU
	fmt.Printf("CPU Usage:   %.2f / %d vCPUs (%.1f%%)\n",
		vm.CPU, vm.MaxCPU, vm.CPUPercent)
	displayBar("  ", vm.CPUPercent)

	// Memory
	fmt.Printf("Memory:      %s / %s (%.1f%%)\n",
		formatBytes(vm.Memory),
		formatBytes(vm.MaxMemory),
		vm.MemPercent)
	displayBar("  ", vm.MemPercent)

	// Disk
	if vm.MaxDisk > 0 {
		fmt.Printf("Disk:        %s / %s (%.1f%%)\n",
			formatBytes(vm.Disk),
			formatBytes(vm.MaxDisk),
			vm.DiskPercent)
		displayBar("  ", vm.DiskPercent)
	} else {
		fmt.Printf("Disk:        %s\n", formatBytes(vm.Disk))
	}

	fmt.Println()

	// Network
	fmt.Printf("Network In:  %s\n", formatBytes(vm.NetIn))
	fmt.Printf("Network Out: %s\n", formatBytes(vm.NetOut))

	// Uptime
	if vm.Status == "running" {
		fmt.Printf("Uptime:      %s\n", formatUptime(vm.Uptime))
	}

	fmt.Println(strings.Repeat("=", 80))
}

// DisplayRRDData shows historical RRD data
func DisplayRRDData(rrdData *RRDData, resourceName string) {
	if len(rrdData.DataPoints) == 0 {
		fmt.Printf("❌ No RRD data found for %s\n", resourceName)
		return
	}

	fmt.Printf("\n📈 Resource History: %s (Timeframe: %s)\n", resourceName, rrdData.TimeFrame)
	fmt.Println(strings.Repeat("=", 120))
	fmt.Printf("%-20s %-10s %-10s %-10s %-15s %-15s %-10s\n",
		"Time", "CPU", "Memory", "Disk", "Net In", "Net Out", "Load")
	fmt.Println(strings.Repeat("-", 120))

	// Show last 10 data points
	start := 0
	if len(rrdData.DataPoints) > 10 {
		start = len(rrdData.DataPoints) - 10
	}

	for i := start; i < len(rrdData.DataPoints); i++ {
		point := rrdData.DataPoints[i]
		fmt.Printf("%-20s %-10.1f %-10.1f %-10.1f %-15s %-15s %-10.2f\n",
			point.Time.Format("2006-01-02 15:04"),
			point.CPU*100,
			point.Memory,
			point.Disk,
			formatBytes(int64(point.NetIn)),
			formatBytes(int64(point.NetOut)),
			point.LoadAvg)
	}

	fmt.Println(strings.Repeat("-", 120))
	fmt.Printf("Total data points: %d (showing last %d)\n",
		len(rrdData.DataPoints),
		len(rrdData.DataPoints)-start)
}

// Helper functions

func displayBar(label string, percent float64) {
	barWidth := 50
	filled := int((percent / 100.0) * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}
	if filled < 0 {
		filled = 0
	}

	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

	// Color based on usage
	color := ""
	if percent >= 90 {
		color = "🔴"
	} else if percent >= 70 {
		color = "🟡"
	} else {
		color = "🟢"
	}

	fmt.Printf("%s %s [%s] %.1f%%\n", label, color, bar, percent)
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	units := []string{"KB", "MB", "GB", "TB", "PB"}
	return fmt.Sprintf("%.1f %s", float64(bytes)/float64(div), units[exp])
}

func formatUptime(seconds int64) string {
	if seconds == 0 {
		return "-"
	}

	days := seconds / 86400
	hours := (seconds % 86400) / 3600
	minutes := (seconds % 3600) / 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh", days, hours)
	} else if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max-3] + "..."
	}
	return s
}

func ocIcon(ratio float64) string {
	switch {
	case ratio > 1.5:
		return "🔴"
	case ratio > 1.0:
		return "🟡"
	default:
		return "🟢"
	}
}

// DisplayCapacityAnalysis renders the capacity/overcommit report and N-1 verdict.
func DisplayCapacityAnalysis(a *CapacityAnalysis) {
	fmt.Println("\n📐 Cluster Capacity Analysis")
	fmt.Println(strings.Repeat("=", 100))
	fmt.Println("Note: RAM figures are ALLOCATED (VM maxmem), not actual in-guest used memory.")
	fmt.Println(strings.Repeat("-", 100))

	fmt.Printf("%-12s %-7s %-10s %-9s %-14s %-14s %-8s\n",
		"Node", "Online", "vCPU/Core", "CPU O/C", "AllocRAM", "PhysRAM", "RAM O/C")
	fmt.Println(strings.Repeat("-", 100))

	for _, n := range a.Nodes {
		online := "🟢 yes"
		if !n.Online {
			online = "🔴 no"
		}
		fmt.Printf("%-12s %-7s %-10s %s %-7.2f %-14s %-14s %s %-6.2f\n",
			n.Node, online,
			fmt.Sprintf("%d/%d", n.AllocVCPU, n.PhysCPU),
			ocIcon(n.CPUOvercommit), n.CPUOvercommit,
			formatBytes(n.AllocMemory), formatBytes(n.PhysMemory),
			ocIcon(n.MemOvercommit), n.MemOvercommit)
	}

	fmt.Println(strings.Repeat("-", 100))
	fmt.Printf("Cluster CPU:    %d vCPU / %d cores  (overcommit %.2fx)\n",
		a.TotalAllocVCPU, a.TotalPhysCPU, a.ClusterCPUOC)
	fmt.Printf("Cluster Memory: %s allocated / %s physical  (overcommit %.2fx)\n",
		formatBytes(a.TotalAllocMemory), formatBytes(a.TotalPhysMemory), a.ClusterMemOC)

	fmt.Println(strings.Repeat("-", 100))
	if a.N1Degraded {
		fmt.Printf("N-1 Failover (lose the largest ONLINE node) — ⚠️  %d node(s) already offline, verdict is best-effort:\n", a.OfflineNodes)
	} else {
		fmt.Println("N-1 Failover (lose the largest-RAM node):")
	}
	fmt.Printf("  Largest node:        %s\n", a.LargestNode)
	fmt.Printf("  Surviving phys RAM:  %s\n", formatBytes(a.SurvivingPhysMem))
	fmt.Printf("  Alloc RAM to place:  %s\n", formatBytes(a.AllocMemToPlace))

	memVerdict := "🔴 DOES NOT FIT"
	if a.N1MemFits {
		memVerdict = "🟢 fits"
	}
	fmt.Printf("  RAM capacity:        %s\n", memVerdict)

	quorumVerdict := "🔴 quorum LOST"
	if a.QuorumAfterFailure {
		quorumVerdict = "🟢 quorum retained"
	}
	fmt.Printf("  Quorum after loss:   %s (%d of %d nodes remain)\n",
		quorumVerdict, a.TotalNodes-1, a.TotalNodes)
	fmt.Println(strings.Repeat("=", 100))
}

// DisplayClusterHealth renders the REST-only cluster health snapshot.
func DisplayClusterHealth(h *ClusterHealth) {
	fmt.Println("\n🩺 Cluster Health (REST-only)")
	fmt.Println(strings.Repeat("=", 80))

	quorum := "🔴 NOT quorate"
	if h.Quorate {
		quorum = "🟢 quorate"
	}
	fmt.Printf("Quorum:   %s\n", quorum)
	fmt.Printf("Nodes:    %d total, %d online\n", h.TotalNodes, h.OnlineNodes)
	fmt.Printf("Storage:  %d total, %d available\n", h.StorageTotal, h.StorageAvailable)

	fmt.Println(strings.Repeat("-", 80))
	fmt.Println("Node membership:")
	for _, ns := range h.NodeStates {
		icon := "🔴"
		if ns.Online {
			icon = "🟢"
		}
		local := ""
		if ns.Local {
			local = " (local)"
		}
		fmt.Printf("  %s %s%s\n", icon, ns.Name, local)
	}

	fmt.Println(strings.Repeat("-", 80))
	switch h.HAState {
	case HAConfigured:
		fmt.Printf("HA resources: %d\n", len(h.HAResources))
		for _, r := range h.HAResources {
			icon := "🟢"
			if r.State != "started" {
				icon = "🟡"
			}
			fmt.Printf("  %s %-14s node=%-10s state=%s\n", icon, r.ID, r.Node, r.State)
		}
	case HANotConfigured:
		fmt.Println("HA:       not configured")
	case HAForbidden:
		fmt.Println("HA:       ⚠️  could not read (token lacks HA.Audit) — state unknown")
	case HAUnavailable:
		fmt.Println("HA:       ⚠️  could not read (transient API error) — state unknown")
	default:
		fmt.Println("HA:       (not probed)")
	}

	fmt.Println(strings.Repeat("-", 80))
	fmt.Println("⚠️  This is a REST-only snapshot. It does NOT cover on-node signals")
	fmt.Println("   (systemd services, journalctl/dmesg errors, zpool status, pvesr")
	fmt.Println("   replication) — those require SSH access to each node.")
	fmt.Println(strings.Repeat("=", 80))
}
