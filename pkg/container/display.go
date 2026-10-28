package container

import (
	"fmt"
	"strings"
)

// DisplayContainers shows a list of containers in table format
func DisplayContainers(containers []*Container) {
	if len(containers) == 0 {
		fmt.Println("❌ No containers found")
		return
	}

	fmt.Println("\n📦 LXC Containers")
	fmt.Println(strings.Repeat("=", 140))
	fmt.Printf("%-6s %-20s %-15s %-10s %-10s %-15s %-15s %-15s %-10s\n",
		"VMID", "Name", "Node", "Status", "Type", "CPU", "Memory", "Disk", "Uptime")
	fmt.Println(strings.Repeat("-", 140))

	for _, ct := range containers {
		status := "🔴 stopped"
		if ct.Status == "running" {
			status = "🟢 running"
		}

		cpu := "-"
		if ct.CPUs > 0 {
			cpu = fmt.Sprintf("%d cores", ct.CPUs)
		}

		memory := formatBytes(ct.Memory)
		disk := formatBytes(ct.Disk)

		uptime := "-"
		if ct.Status == "running" && ct.Uptime > 0 {
			uptime = formatUptime(ct.Uptime)
		}

		template := ""
		if ct.Template {
			template = " 📋"
		}

		fmt.Printf("%-6d %-20s %-15s %-10s %-10s %-15s %-15s %-15s %-10s\n",
			ct.VMID,
			truncate(ct.Name, 20)+template,
			truncate(ct.Node, 15),
			status,
			ct.OSType,
			cpu,
			memory,
			disk,
			uptime)
	}

	fmt.Println(strings.Repeat("-", 140))
	fmt.Printf("Total containers: %d\n", len(containers))
}

// DisplayContainerDetails shows detailed information about a single container
func DisplayContainerDetails(ct *Container) {
	fmt.Printf("\n📦 Container Details: %d (%s)\n", ct.VMID, ct.Name)
	fmt.Println(strings.Repeat("=", 80))

	// Basic information
	fmt.Printf("Node:        %s\n", ct.Node)
	fmt.Printf("Status:      %s\n", ct.Status)
	fmt.Printf("Type:        %s\n", ct.Type)
	if ct.OSType != "" {
		fmt.Printf("OS Type:     %s\n", ct.OSType)
	}
	if ct.Arch != "" {
		fmt.Printf("Arch:        %s\n", ct.Arch)
	}
	if ct.Hostname != "" {
		fmt.Printf("Hostname:    %s\n", ct.Hostname)
	}
	if ct.Description != "" {
		fmt.Printf("Description: %s\n", ct.Description)
	}
	fmt.Println()

	// Resource allocation
	fmt.Println("Resources:")
	if ct.CPUs > 0 {
		fmt.Printf("  CPUs:      %d cores\n", ct.CPUs)
	}
	if ct.Memory > 0 {
		fmt.Printf("  Memory:    %s\n", formatBytes(ct.Memory))
	}
	if ct.Swap > 0 {
		fmt.Printf("  Swap:      %s\n", formatBytes(ct.Swap))
	}
	if ct.Disk > 0 {
		fmt.Printf("  Disk:      %s\n", formatBytes(ct.Disk))
	}
	fmt.Println()

	// Runtime info
	if ct.Status == "running" {
		fmt.Println("Runtime:")
		if ct.Uptime > 0 {
			fmt.Printf("  Uptime:    %s\n", formatUptime(ct.Uptime))
		}
		if ct.PID > 0 {
			fmt.Printf("  PID:       %d\n", ct.PID)
		}
		if ct.NetIn > 0 || ct.NetOut > 0 {
			fmt.Printf("  Network:   In: %s, Out: %s\n", formatBytes(ct.NetIn), formatBytes(ct.NetOut))
		}
		fmt.Println()
	}

	// Flags
	flags := []string{}
	if ct.Protected {
		flags = append(flags, "🔒 Protected")
	}
	if ct.Template {
		flags = append(flags, "📋 Template")
	}
	if ct.Lock != "" {
		flags = append(flags, fmt.Sprintf("🔐 Locked (%s)", ct.Lock))
	}
	if len(flags) > 0 {
		fmt.Printf("Flags:       %s\n", strings.Join(flags, ", "))
		fmt.Println()
	}

	// Storage
	if ct.RootFS != "" {
		fmt.Printf("Root FS:     %s\n", ct.RootFS)
	}

	fmt.Println(strings.Repeat("=", 80))
}

// DisplayContainerStatus shows detailed status information
func DisplayContainerStatus(status *ContainerStatus) {
	fmt.Printf("\n📊 Container Status: %d (%s)\n", status.VMID, status.Name)
	fmt.Println(strings.Repeat("=", 80))

	fmt.Printf("Node:        %s\n", status.Node)
	fmt.Printf("Status:      %s\n", status.Status)
	fmt.Println()

	if status.Status == "running" {
		// CPU
		fmt.Printf("CPU Usage:   %.2f / %d cores (%.1f%%)\n",
			status.CPU, status.MaxCPU, status.CPUPercent)
		displayBar("  ", status.CPUPercent)

		// Memory
		fmt.Printf("Memory:      %s / %s (%.1f%%)\n",
			formatBytes(status.Memory),
			formatBytes(status.MaxMemory),
			status.MemPercent)
		displayBar("  ", status.MemPercent)

		// Swap
		if status.MaxSwap > 0 {
			swapPercent := 0.0
			if status.MaxSwap > 0 {
				swapPercent = (float64(status.Swap) / float64(status.MaxSwap)) * 100
			}
			fmt.Printf("Swap:        %s / %s (%.1f%%)\n",
				formatBytes(status.Swap),
				formatBytes(status.MaxSwap),
				swapPercent)
			displayBar("  ", swapPercent)
		}

		// Disk
		if status.MaxDisk > 0 {
			diskPercent := (float64(status.Disk) / float64(status.MaxDisk)) * 100
			fmt.Printf("Disk:        %s / %s (%.1f%%)\n",
				formatBytes(status.Disk),
				formatBytes(status.MaxDisk),
				diskPercent)
			displayBar("  ", diskPercent)
		} else {
			fmt.Printf("Disk:        %s\n", formatBytes(status.Disk))
		}

		fmt.Println()

		// Network
		fmt.Printf("Network In:  %s\n", formatBytes(status.NetIn))
		fmt.Printf("Network Out: %s\n", formatBytes(status.NetOut))

		// Uptime
		fmt.Printf("Uptime:      %s\n", formatUptime(status.Uptime))
		if status.PID > 0 {
			fmt.Printf("PID:         %d\n", status.PID)
		}
	}

	// HA status
	if status.HAState != "" {
		fmt.Printf("\nHA State:    %s\n", status.HAState)
	}

	fmt.Println(strings.Repeat("=", 80))
}

// DisplaySnapshots shows container snapshots
func DisplaySnapshots(vmid int, snapshots []*ContainerSnapshot) {
	if len(snapshots) == 0 {
		fmt.Printf("❌ No snapshots found for container %d\n", vmid)
		return
	}

	fmt.Printf("\n📸 Snapshots for Container %d\n", vmid)
	fmt.Println(strings.Repeat("=", 100))
	fmt.Printf("%-25s %-20s %-40s %-15s\n",
		"Name", "Created", "Description", "Parent")
	fmt.Println(strings.Repeat("-", 100))

	for _, snap := range snapshots {
		parent := "-"
		if snap.Parent != "" {
			parent = snap.Parent
		}

		desc := "-"
		if snap.Description != "" {
			desc = truncate(snap.Description, 40)
		}

		created := "-"
		if !snap.SnapTime.IsZero() {
			created = snap.SnapTime.Format("2006-01-02 15:04:05")
		}

		fmt.Printf("%-25s %-20s %-40s %-15s\n",
			truncate(snap.Name, 25),
			created,
			desc,
			parent)
	}

	fmt.Println(strings.Repeat("-", 100))
	fmt.Printf("Total snapshots: %d\n", len(snapshots))
}

// DisplayContainerSummary shows a summary of containers by status
func DisplayContainerSummary(containers []*Container) {
	if len(containers) == 0 {
		fmt.Println("❌ No containers found")
		return
	}

	running := 0
	stopped := 0
	templates := 0
	totalCPU := 0
	var totalMemory int64
	var totalDisk int64

	for _, ct := range containers {
		if ct.Status == "running" {
			running++
		} else {
			stopped++
		}
		if ct.Template {
			templates++
		}
		totalCPU += ct.CPUs
		totalMemory += ct.Memory
		totalDisk += ct.Disk
	}

	fmt.Println("\n📊 Container Summary")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("Total Containers:  %d\n", len(containers))
	fmt.Printf("  Running:         %d 🟢\n", running)
	fmt.Printf("  Stopped:         %d 🔴\n", stopped)
	fmt.Printf("  Templates:       %d 📋\n", templates)
	fmt.Println()
	fmt.Printf("Total Resources:\n")
	fmt.Printf("  CPUs:            %d cores\n", totalCPU)
	fmt.Printf("  Memory:          %s\n", formatBytes(totalMemory))
	fmt.Printf("  Disk:            %s\n", formatBytes(totalDisk))
	fmt.Println(strings.Repeat("=", 80))
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
