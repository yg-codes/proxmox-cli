package main

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yg-codes/proxmox-cli/pkg/vm"
)

// vmCloudInitCmd groups the cloudinit operations.
var vmCloudInitCmd = &cobra.Command{
	Use:     "cloudinit",
	Aliases: []string{"ci"},
	Short:   "Inspect and regenerate a VM's cloudinit configuration",
	Long: `Work with a VM's cloudinit drive.

Three things worth knowing before using these:

  * cloudinit SETTINGS are changed with 'pve vm config set' (ciuser, cipassword,
    citype, ciupgrade, sshkeys, nameserver, searchdomain, ipconfigN). This
    command group does not set them — it reports drift, regenerates the drive,
    and shows what the guest will actually read.

  * 'pending' shows settings that differ between the VM config and the drive as
    last generated. Non-empty means the guest is not yet seeing them.

  * A regenerated drive takes effect on the guest's NEXT BOOT. cloudinit reads
    it at boot, so regenerating alone changes nothing in a running guest — and
    most cloudinit modules only run once per instance anyway.

Examples:
  pve vm cloudinit pending --vmid 100
  pve vm cloudinit dump --vmid 100 --type user
  pve vm cloudinit dump --vmid 100 --type network
  pve vm cloudinit regenerate --vmid 100`,
}

var vmCloudInitPendingCmd = &cobra.Command{
	Use:   "pending",
	Short: "Show cloudinit settings not yet written to the drive",
	RunE:  runCloudInitPending,
}

var vmCloudInitDumpCmd = &cobra.Command{
	Use:   "dump",
	Short: "Print the rendered cloudinit data the guest will read",
	Long: `Print the generated cloudinit configuration for a VM.

  --type user     the #cloud-config document: hostname, users, ssh keys, packages
  --type network  the network configuration cloudinit will apply
  --type meta     instance metadata (instance-id and similar)

This is the GENERATED output, not the VM config that produced it. It is the
right thing to check when a guest is not picking up a setting you expect.`,
	RunE: runCloudInitDump,
}

var vmCloudInitRegenerateCmd = &cobra.Command{
	Use:     "regenerate",
	Aliases: []string{"update"},
	Short:   "Rebuild the cloudinit drive from the current VM config",
	Long: `Regenerate the VM's cloudinit drive.

This commits pending cloudinit settings to the drive. The guest reads that drive
at boot, so the settings reach it on the next boot, not immediately — and many
cloudinit modules run only once per instance, so an already-provisioned guest may
ignore some of them even after rebooting.`,
	RunE: runCloudInitRegenerate,
}

func initVMCloudInitCommands() {
	vmCloudInitCmd.PersistentFlags().String("vmid", "", "VM ID (or use --vmname)")
	vmCloudInitCmd.PersistentFlags().String("vmname", "", "VM name, matched exactly (alternative to --vmid)")
	vmCloudInitDumpCmd.Flags().String("type", "user",
		"Section to render: "+strings.Join(vm.CloudInitDumpTypes(), ", "))

	vmCloudInitCmd.AddCommand(vmCloudInitPendingCmd, vmCloudInitDumpCmd, vmCloudInitRegenerateCmd)
}

func cloudInitVMID(cmd *cobra.Command) (string, error) {
	return vmTargetFromFlags(cmd)
}

func runCloudInitPending(cmd *cobra.Command, args []string) error {
	vmid, err := cloudInitVMID(cmd)
	if err != nil {
		return err
	}

	pending, err := vmOps.GetCloudInitPending(vmid)
	if err != nil {
		return err
	}

	// Show the live cloudinit config alongside the drift, so "nothing pending"
	// can be told apart from "cloudinit is not configured at all" — those look
	// identical if only the pending list is printed.
	config, cfgErr := vmOps.GetVMConfig(vmid)
	if cfgErr == nil {
		var keys []string
		for k := range config {
			if vm.IsCloudInitKey(k) {
				keys = append(keys, k)
			}
		}
		if len(keys) == 0 {
			fmt.Printf("VM %s has no cloudinit settings in its config.\n", vmid)
		} else {
			fmt.Printf("Cloudinit settings on VM %s:\n\n", vmid)
			for _, k := range sortedStrings(keys) {
				value := formatConfigValue(config[k])
				// sshkeys comes back percent-encoded from the config endpoint.
				// The pending list decodes it, so leaving this raw would print
				// the same value two different ways in one output.
				if decoded, err := url.QueryUnescape(value); err == nil {
					value = decoded
				}
				if k == "cipassword" {
					value = "(set, not shown)"
				}
				fmt.Printf("  %-14s %s\n", k, value)
			}
			fmt.Println()
		}
	}

	if len(pending) == 0 {
		fmt.Printf("No pending cloudinit changes for VM %s — the drive matches the config.\n", vmid)
		return nil
	}

	fmt.Printf("Pending cloudinit changes for VM %s (%d):\n\n", vmid, len(pending))
	for _, p := range pending {
		value := p.Value
		if p.Key == "cipassword" {
			value = "(changed, not shown)"
		}
		fmt.Printf("  %-14s %s\n", p.Key, value)
	}
	fmt.Println()
	fmt.Println("These reach the guest after 'pve vm cloudinit regenerate' AND a reboot.")
	return nil
}

func runCloudInitDump(cmd *cobra.Command, args []string) error {
	vmid, err := cloudInitVMID(cmd)
	if err != nil {
		return err
	}
	dumpType, _ := cmd.Flags().GetString("type")

	// Validate locally so a typo names the valid set rather than returning the
	// server's enum error.
	if err := vm.ValidateCloudInitDumpType(dumpType); err != nil {
		return err
	}

	out, err := vmOps.DumpCloudInit(vmid, dumpType)
	if err != nil {
		return err
	}
	fmt.Print(out)
	if !strings.HasSuffix(out, "\n") {
		fmt.Println()
	}
	return nil
}

func runCloudInitRegenerate(cmd *cobra.Command, args []string) error {
	vmid, err := cloudInitVMID(cmd)
	if err != nil {
		return err
	}

	pending, err := vmOps.GetCloudInitPending(vmid)
	if err != nil {
		return err
	}

	fmt.Printf("Regenerate the cloudinit drive for VM %s\n\n", vmid)
	if len(pending) == 0 {
		fmt.Println("  No pending changes — the drive already matches the config.")
	} else {
		fmt.Printf("  Commits %d pending change(s):\n", len(pending))
		for _, p := range pending {
			value := p.Value
			if p.Key == "cipassword" {
				value = "(changed, not shown)"
			}
			fmt.Printf("    %-14s %s\n", p.Key, value)
		}
	}
	fmt.Println()
	fmt.Println("  The guest reads the drive at boot, so this takes effect on its next")
	fmt.Println("  boot — not immediately.")
	fmt.Println()

	if dryRun {
		printDryRunHeader()
		printDryRunAction("cloudinit regenerate", vmid, "",
			fmt.Sprintf("%d pending change(s)", len(pending)))
		printDryRunSummary("cloudinit regenerate", 1)
		return nil
	}

	if !confirmOperation(fmt.Sprintf("Regenerate the cloudinit drive for VM %s?", vmid)) {
		fmt.Println("Operation cancelled")
		return nil
	}

	if err := vmOps.RegenerateCloudInit(vmid); err != nil {
		return err
	}
	fmt.Printf("✅ Cloudinit drive regenerated for VM %s\n", vmid)
	return nil
}

// sortedStrings returns a sorted copy, so repeated runs print the same order.
func sortedStrings(in []string) []string {
	out := make([]string, len(in))
	copy(out, in)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
