package main

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
	"github.com/yg-codes/proxmox-cli/pkg/api"
)

// nodeCapabilityCmd probes which report endpoints the current token can reach,
// so users learn upfront what will work with their ACLs. It is read-only.
var nodeCapabilityCmd = &cobra.Command{
	Use:   "capability",
	Short: "Check which report endpoints the current token can access",
	Long: `Probe the report endpoints used by capacity, ZFS/disk, SMART, and health
commands with the current token and print a ✅/❌ matrix.

These endpoints do not all check the same ACL path: node status, physical disks
and SMART check Sys.Audit on /nodes, while ZFS pools, cluster health and HA
status check it on /. A token can therefore reach the disk list and still be
refused the ZFS report. Failed probes print the exact grant each one needs.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCapabilityCheck()
	},
}

func initNodeCapabilityCommand() {
	nodeCmd.AddCommand(nodeCapabilityCmd)
}

type capabilityProbe struct {
	name string
	path string
	hint string
}

func runCapabilityCheck() error {
	// Resolve a node name for node-scoped probes.
	nodeName, err := firstNodeName()
	if err != nil {
		return err
	}

	// Each hint must name the path the endpoint ACTUALLY checks. These are not
	// uniform: the ZFS endpoint checks Sys.Audit on '/' while its neighbours in
	// the same /disks/ subtree check '/nodes'. A hint that sends the operator to
	// the wrong path cannot fix the failure it describes — they apply it, the
	// check still fails, and they reasonably conclude the grant did not take.
	probes := []capabilityProbe{
		{"capacity / resources", "/cluster/resources", "needs at least VM Audit"},
		{"cluster health", "/cluster/status", "needs Sys.Audit on /"},
		{"HA status", "/cluster/ha/status/current", "needs Sys.Audit on / (or HA.Audit)"},
	}
	if nodeName != "" {
		probes = append(probes,
			capabilityProbe{"node status", fmt.Sprintf("/nodes/%s/status", nodeName), "needs Sys.Audit on /nodes"},
			capabilityProbe{"ZFS pools", fmt.Sprintf("/nodes/%s/disks/zfs", nodeName), "needs Sys.Audit on / (not /nodes)"},
			capabilityProbe{"physical disks", fmt.Sprintf("/nodes/%s/disks/list", nodeName), "needs Sys.Audit on /nodes"},
		)
	}

	fmt.Println("\n🔎 Token Capability Check")
	fmt.Printf("Probing with node: %s\n", nodeName)
	fmt.Println("--------------------------------------------------------------------")
	anyFailed := false
	for _, p := range probes {
		_, err := client.Get(p.path, nil)
		if err == nil {
			fmt.Printf("✅ %-22s %s\n", p.name, p.path)
			continue
		}
		anyFailed = true
		var apiErr *api.ProxmoxAPIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusForbidden {
			fmt.Printf("❌ %-22s 403 forbidden — %s\n", p.name, p.hint)
		} else {
			fmt.Printf("⚠️  %-22s error: %v\n", p.name, err)
		}
	}
	fmt.Println("--------------------------------------------------------------------")
	if anyFailed {
		fmt.Println("Grant the missing audit privileges on the Proxmox node:")
		fmt.Println()
		fmt.Println("  # ZFS pools, cluster health, HA status — these check '/'")
		fmt.Println("  pveum aclmod / -user '<user>@<realm>' -role PVEAuditor")
		fmt.Println()
		fmt.Println("  # node status, physical disks, SMART — these check '/nodes'")
		fmt.Println("  pveum aclmod /nodes -user '<user>@<realm>' -role PVEAuditor")
		fmt.Println()
		fmt.Println("Note: grant to -user, not -token. A token created with --privsep 0")
		fmt.Println("inherits the USER's privileges and its own -token ACL is ignored, so")
		fmt.Println("a token-only grant silently has no effect. Check with:")
		fmt.Println("  pveum user token list <user>@<realm>    # privsep column")
	} else {
		fmt.Println("All report endpoints reachable with the current token — no extra permissions needed.")
	}
	return nil
}

// firstNodeName returns the name of the first cluster node, for node-scoped probes.
func firstNodeName() (string, error) {
	resp, err := client.Get("/nodes", nil)
	if err != nil {
		return "", fmt.Errorf("failed to list nodes: %w", err)
	}
	items, ok := resp["items"].([]interface{})
	if !ok {
		return "", nil
	}
	for _, n := range items {
		if m, ok := n.(map[string]interface{}); ok {
			if name, ok := m["node"].(string); ok {
				return name, nil
			}
		}
	}
	return "", nil
}
