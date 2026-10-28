package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yg-codes/proxmox-cli/pkg/resource"
)

// clusterHealthCmd is a REST-only cluster health snapshot (quorum, node state,
// HA resources, storage availability). It intentionally does NOT cover on-node
// signals (systemd, journalctl, zpool status, replication) which need SSH.
var clusterHealthCmd = &cobra.Command{
	Use:   "health",
	Short: "Show cluster health (quorum, HA, storage) via REST",
	Long: `Display a REST-only cluster health snapshot: quorum status, node
membership, HA resource states, and storage availability.

This does NOT replace on-node health checks (systemd services, journalctl/dmesg
errors, zpool status, pvesr replication) — those require SSH access to nodes.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		health, err := resourceOps.GetClusterHealth()
		if err != nil {
			return fmt.Errorf("failed to get cluster health: %w", err)
		}

		resource.DisplayClusterHealth(health)
		return nil
	},
}

func initClusterHealthCommand() {
	clusterCmd.AddCommand(clusterHealthCmd)
}
