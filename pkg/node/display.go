package node

import (
	"fmt"
	"strings"
	"time"
)

// DisplayNodes shows nodes in formatted table
func DisplayNodes(nodes []*Node) {
	if len(nodes) == 0 {
		fmt.Println("❌ No nodes found")
		return
	}

	fmt.Println("\nProxmox Cluster Nodes:")
	fmt.Println(strings.Repeat("=", 120))
	fmt.Printf("%-15s %-10s %-15s %-10s %-10s %-15s %-15s %-15s\n",
		"Node", "Status", "IP", "Level", "CPU %", "Memory %", "Disk %", "Uptime")
	fmt.Println(strings.Repeat("-", 120))

	for _, node := range nodes {
		status := "🔴 offline"
		if node.Online {
			status = "🟢 online"
		}

		cpuPercent := 0.0
		if node.MaxCPU > 0 {
			cpuPercent = node.CPUUsage * 100
		}

		memPercent := 0.0
		if node.Memory > 0 {
			memPercent = (float64(node.MemoryUsed) / float64(node.Memory)) * 100
		}

		diskPercent := 0.0
		if node.Disk > 0 {
			diskPercent = (float64(node.DiskUsed) / float64(node.Disk)) * 100
		}

		uptime := formatUptime(node.Uptime)

		fmt.Printf("%-15s %-10s %-15s %-10s %-10.1f %-15.1f %-15.1f %-15s\n",
			node.Name, status, node.IP, node.Level,
			cpuPercent, memPercent, diskPercent, uptime)
	}

	fmt.Println(strings.Repeat("-", 120))
	fmt.Printf("Total nodes: %d\n", len(nodes))
}

// DisplayNodeStatus shows detailed status for a single node
func DisplayNodeStatus(node *Node) {
	fmt.Printf("\n🖥️  Node: %s\n", node.Name)
	fmt.Println(strings.Repeat("=", 80))

	// Status section
	status := "🔴 Offline"
	if node.Online {
		status = "🟢 Online"
	}
	fmt.Printf("Status:          %s\n", status)
	fmt.Printf("Uptime:          %s\n", formatUptime(node.Uptime))

	// Version section
	if node.PVEVersion != "" {
		fmt.Printf("PVE Version:     %s\n", node.PVEVersion)
	}
	if node.KernelVersion != "" {
		fmt.Printf("Kernel Version:  %s\n", node.KernelVersion)
	}

	fmt.Println()

	// CPU section
	cpuPercent := 0.0
	if node.MaxCPU > 0 {
		cpuPercent = node.CPUUsage * 100
	}
	fmt.Printf("CPU:             %.1f%% of %d cores (%.2f used)\n",
		cpuPercent, node.MaxCPU, node.CPUUsage*float64(node.MaxCPU))

	// Memory section
	if node.Memory > 0 {
		memPercent := (float64(node.MemoryUsed) / float64(node.Memory)) * 100
		fmt.Printf("Memory:          %.1f%% of %.2f GB (%.2f GB used, %.2f GB free)\n",
			memPercent,
			float64(node.Memory)/(1024*1024*1024),
			float64(node.MemoryUsed)/(1024*1024*1024),
			float64(node.MemoryFree)/(1024*1024*1024))
	}

	// Disk section
	if node.Disk > 0 {
		diskPercent := (float64(node.DiskUsed) / float64(node.Disk)) * 100
		fmt.Printf("Disk:            %.1f%% of %.2f GB (%.2f GB used, %.2f GB free)\n",
			diskPercent,
			float64(node.Disk)/(1024*1024*1024),
			float64(node.DiskUsed)/(1024*1024*1024),
			float64(node.DiskFree)/(1024*1024*1024))
	}

	fmt.Println(strings.Repeat("=", 80))
}

// DisplayServices shows services in formatted table
func DisplayServices(nodeName string, services []*Service) {
	if len(services) == 0 {
		fmt.Printf("❌ No services found on node %s\n", nodeName)
		return
	}

	fmt.Printf("\n⚙️  Services on Node: %s\n", nodeName)
	fmt.Println(strings.Repeat("=", 100))
	fmt.Printf("%-3s %-25s %-12s %-12s %-40s\n",
		"#", "Service", "State", "Unit State", "Description")
	fmt.Println(strings.Repeat("-", 100))

	for i, svc := range services {
		state := "🔴 stopped"
		if svc.Running {
			state = "🟢 running"
		}

		unitState := svc.UnitState
		if unitState == "" {
			if svc.Active {
				unitState = "active"
			} else {
				unitState = "inactive"
			}
		}

		desc := svc.Description
		if len(desc) > 40 {
			desc = desc[:37] + "..."
		}

		fmt.Printf("%-3d %-25s %-12s %-12s %-40s\n",
			i+1, truncate(svc.Name, 25), state, unitState, desc)
	}

	fmt.Println(strings.Repeat("-", 100))
	fmt.Printf("Total services: %d\n", len(services))
}

// DisplayServiceStatus shows detailed status for a single service
func DisplayServiceStatus(nodeName string, service *Service) {
	fmt.Printf("\n⚙️  Service: %s on Node: %s\n", service.Name, nodeName)
	fmt.Println(strings.Repeat("=", 80))

	state := "🔴 Stopped"
	if service.Running {
		state = "🟢 Running"
	}

	fmt.Printf("Name:        %s\n", service.Name)
	fmt.Printf("State:       %s\n", state)
	fmt.Printf("Unit State:  %s\n", service.UnitState)

	active := "inactive"
	if service.Active {
		active = "active"
	}
	fmt.Printf("Active:      %s\n", active)

	if service.Description != "" {
		fmt.Printf("Description: %s\n", service.Description)
	}

	fmt.Println(strings.Repeat("=", 80))
}

// DisplayVersionInfo shows node version information
func DisplayVersionInfo(nodeName string, version *VersionInfo) {
	fmt.Printf("\n📦 Version Information - Node: %s\n", nodeName)
	fmt.Println(strings.Repeat("=", 80))

	fmt.Printf("Version:     %s\n", version.Version)
	if version.Release != "" {
		fmt.Printf("Release:     %s\n", version.Release)
	}
	if version.RepoID != "" {
		fmt.Printf("Repo ID:     %s\n", version.RepoID)
	}
	if version.Kernel != "" {
		fmt.Printf("Kernel:      %s\n", version.Kernel)
	}

	fmt.Println(strings.Repeat("=", 80))
}

// Helper functions

func formatUptime(seconds int64) string {
	if seconds == 0 {
		return "-"
	}

	d := time.Duration(seconds) * time.Second
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
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
