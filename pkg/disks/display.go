package disks

import (
	"fmt"
	"strings"
)

func bytesToGB(b int64) float64 {
	return float64(b) / (1024 * 1024 * 1024)
}

func healthIcon(health string) string {
	switch strings.ToUpper(health) {
	case "ONLINE", "PASSED", "OK":
		return "🟢"
	case "DEGRADED", "UNKNOWN", "":
		return "🟡"
	default: // FAULTED, FAILED, OFFLINE, UNAVAIL, etc.
		return "🔴"
	}
}

// DisplayZFSPools renders ZFS pools in a formatted table.
func DisplayZFSPools(pools []*ZFSPool) {
	if len(pools) == 0 {
		fmt.Println("❌ No ZFS pools found")
		return
	}

	fmt.Println("\nZFS Pools:")
	fmt.Println(strings.Repeat("=", 92))
	fmt.Printf("%-12s %-14s %-10s %-10s %-10s %-7s %-10s\n",
		"Node", "Pool", "Size(GB)", "Alloc(GB)", "Free(GB)", "Frag%", "Health")
	fmt.Println(strings.Repeat("-", 92))

	for _, p := range pools {
		fmt.Printf("%-12s %-14s %-10.1f %-10.1f %-10.1f %-7.0f %s %s\n",
			p.Node, p.Name,
			bytesToGB(p.Size), bytesToGB(p.Alloc), bytesToGB(p.Free),
			p.Frag, healthIcon(p.Health), p.Health)
	}

	fmt.Println(strings.Repeat("-", 92))
	fmt.Printf("Total pools: %d\n", len(pools))
}

// DisplayZFSPoolDetail renders the device tree of a single pool.
func DisplayZFSPoolDetail(detail *ZFSPoolDetail) {
	fmt.Printf("\nZFS Pool: %s (node %s)\n", detail.Name, detail.Node)
	fmt.Println(strings.Repeat("=", 70))
	fmt.Printf("State:  %s %s\n", healthIcon(detail.State), detail.State)
	if detail.Scan != "" {
		fmt.Printf("Scan:   %s\n", detail.Scan)
	}
	if detail.Errors != "" {
		fmt.Printf("Errors: %s\n", detail.Errors)
	}
	fmt.Println(strings.Repeat("-", 70))
	fmt.Printf("%-40s %-10s %6s %6s %6s\n", "NAME", "STATE", "READ", "WRITE", "CKSUM")
	if detail.Root != nil {
		printVdev(detail.Root, 0)
	}
	fmt.Println(strings.Repeat("-", 70))
}

func printVdev(v *ZFSVdev, indent int) {
	name := strings.Repeat("  ", indent) + v.Name
	fmt.Printf("%-40s %-10s %6d %6d %6d\n", name, v.State, v.Read, v.Write, v.Cksum)
	for _, c := range v.Children {
		printVdev(c, indent+1)
	}
}

// DisplayDisks renders physical disks in a formatted table.
func DisplayDisks(nodeName string, disks []*Disk) {
	if len(disks) == 0 {
		fmt.Printf("❌ No disks found on node %s\n", nodeName)
		return
	}

	fmt.Printf("\nPhysical Disks on %s:\n", nodeName)
	fmt.Println(strings.Repeat("=", 100))
	fmt.Printf("%-12s %-6s %-9s %-20s %-8s %-8s %-12s\n",
		"Device", "Type", "Size(GB)", "Model", "Wearout", "Health", "Usage")
	fmt.Println(strings.Repeat("-", 100))

	for _, d := range disks {
		wear := "N/A"
		if d.Wearout >= 0 {
			wear = fmt.Sprintf("%d%%", d.Wearout)
		}
		used := d.Used
		if used == "" {
			used = "-"
		}
		fmt.Printf("%-12s %-6s %-9.1f %-20s %-8s %s %-6s %-12s\n",
			d.DevPath, d.Type, bytesToGB(d.Size), truncate(d.Model, 20),
			wear, healthIcon(d.Health), d.Health, used)
	}

	fmt.Println(strings.Repeat("-", 100))
	fmt.Printf("Total disks: %d\n", len(disks))
}

// DisplaySMART renders SMART health for a single disk.
func DisplaySMART(status *SMARTStatus) {
	fmt.Printf("\nSMART Health: %s (node %s)\n", status.DevPath, status.Node)
	fmt.Println(strings.Repeat("=", 70))
	fmt.Printf("Overall: %s %s\n", healthIcon(status.Health), status.Health)
	if status.Type != "" {
		fmt.Printf("Type:    %s\n", status.Type)
	}

	if len(status.Attributes) > 0 {
		fmt.Println(strings.Repeat("-", 70))
		fmt.Printf("%-4s %-26s %-6s %-6s %-6s %-8s\n",
			"ID", "Attribute", "Value", "Worst", "Thresh", "Failing")
		for _, a := range status.Attributes {
			failing := a.Failing
			if failing == "" {
				failing = "-"
			}
			fmt.Printf("%-4d %-26s %-6d %-6d %-6d %-8s\n",
				a.ID, truncate(a.Name, 26), a.Value, a.Worst, a.Threshold, failing)
		}
	} else if status.Text != "" {
		fmt.Println(strings.Repeat("-", 70))
		fmt.Println(status.Text)
	}
	fmt.Println(strings.Repeat("-", 70))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}
