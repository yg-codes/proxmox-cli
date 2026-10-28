package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/yg-codes/proxmox-cli/pkg/vm"
)

// vmConfigCmd groups VM configuration commands
var vmConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "View and modify VM configuration",
	Long: `View and modify virtual machine configuration.

  get      Show the current configuration
  set      Change configuration keys
  pending  Show changes staged for the next power cycle`,
}

// vmConfigGetCmd shows a VM's configuration
var vmConfigGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Show VM configuration",
	Long: `Show the current configuration of one or more virtual machines.

--vmid and --vmname accept the full selector syntax: comma-separated lists,
ranges, wildcards and the keywords all, running and stopped.

Examples:
  pve vm config get --vmid 4103
  pve vm config get --vmid 4103 --key cores,memory
  pve vm config get --vmid 4103,4105 --key cores,memory
  pve vm config get --vmid '4103-4105' --key cores
  pve vm config get --vmid running --key cores`,
	RunE: runConfigGetCommand,
}

// vmConfigSetCmd changes a VM's configuration
var vmConfigSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Change VM configuration",
	Long: `Change the configuration of one or more virtual machines.

Each --set takes a key=value pair; --delete removes a key. The same change set
is applied to every selected VM. The current and new values are shown per VM
for confirmation before anything is written.

--vmid and --vmname accept the full selector syntax (lists, ranges, wildcards,
and the keywords all, running, stopped). Because that makes it possible to
target many VMs at once, confirmation requires typing the exact number of VMs
shown, not just 'y'.

Changes that cannot be applied to a running VM are staged as pending and take
effect on the next full stop/start or 'pve vm reboot' — a shutdown and start
from inside the guest is not enough. Use 'pve vm config pending' to see what is
staged.

By default the write is guarded by the configuration digest read at the start
of the command, so a concurrent modification fails cleanly instead of silently
overwriting someone else's change. Pass --no-digest-check to disable that.

Examples:
  pve vm config set --vmid 4103 --set cores=4
  pve vm config set --vmid 4103 --set cores=4 --set memory=8192
  pve vm config set --vmid 4103 --delete description
  pve vm config set --vmid 4103,4105 --set cores=4`,
	RunE: runConfigSetCommand,
}

// vmConfigPendingCmd shows pending configuration changes
var vmConfigPendingCmd = &cobra.Command{
	Use:   "pending",
	Short: "Show pending VM configuration changes",
	Long: `Show configuration changes staged for the next power cycle.

Lists keys whose pending value differs from the running value, plus any keys
staged for deletion.

--vmid and --vmname accept the full selector syntax.

Examples:
  pve vm config pending --vmid 4103
  pve vm config pending --vmid 4103,4105
  pve vm config pending --vmid running`,
	RunE: runConfigPendingCommand,
}

// initVMConfigFlags registers flags for the config subcommands.
//
// --vmid/--vmname are StringSlice so they accept the same selector syntax as
// the rest of the CLI (lists, ranges, wildcards, keywords). --set/--delete are
// deliberately StringArray -- see the comment below.
func initVMConfigFlags() {
	vmConfigGetCmd.Flags().StringSlice("vmid", []string{}, "VM IDs, ranges, wildcards or keywords (or use --vmname)")
	vmConfigGetCmd.Flags().StringSlice("vmname", []string{}, "VM names or patterns (alternative to --vmid)")
	vmConfigGetCmd.Flags().StringSlice("key", []string{}, "Only show these configuration keys")

	vmConfigSetCmd.Flags().StringSlice("vmid", []string{}, "VM IDs, ranges, wildcards or keywords (or use --vmname)")
	vmConfigSetCmd.Flags().StringSlice("vmname", []string{}, "VM names or patterns (alternative to --vmid)")
	// StringArray, not StringSlice: slice flags parse their value as CSV, which
	// splits a value on commas (net0=virtio,bridge=vmbr0 becomes three broken
	// entries) and truncates it at an embedded newline (a multi-line
	// description silently loses every line but the first).
	vmConfigSetCmd.Flags().StringArray("set", []string{}, "Configuration to set as key=value (repeatable)")
	vmConfigSetCmd.Flags().StringArray("delete", []string{}, "Configuration keys to delete (repeatable)")
	vmConfigSetCmd.Flags().Bool("no-digest-check", false, "Do not guard the write against concurrent modification")

	vmConfigPendingCmd.Flags().StringSlice("vmid", []string{}, "VM IDs, ranges, wildcards or keywords (or use --vmname)")
	vmConfigPendingCmd.Flags().StringSlice("vmname", []string{}, "VM names or patterns (alternative to --vmid)")
}

// configTargets resolves the --vmid/--vmname selectors to VMs.
//
// Unlike the single-target commands (destroy, clone, agent, firewall, …) which
// use resolveSingleVMID, config operates on a set, so it goes through the full
// selector: keywords (all, running, stopped), wildcards (72*) and ranges
// (7201-7205) all work here.
func configTargets(cmd *cobra.Command) ([]*vm.VM, error) {
	vmids, _ := cmd.Flags().GetStringSlice("vmid")
	vmnames, _ := cmd.Flags().GetStringSlice("vmname")

	// Reject an empty selector here rather than letting it reach resolveVMs,
	// which would immediately dereference the API client to list every VM.
	if len(vmids) == 0 && len(vmnames) == 0 {
		return nil, fmt.Errorf("either --vmid or --vmname must be specified")
	}

	return resolveVMs(vmids, vmnames)
}

// configDisplayKeys are internal fields that are not VM settings and are
// filtered out of 'config get' output.
var configDisplayKeys = map[string]bool{
	"digest": true,
}

func runConfigGetCommand(cmd *cobra.Command, args []string) error {
	vms, err := configTargets(cmd)
	if err != nil {
		return err
	}
	keys, _ := cmd.Flags().GetStringSlice("key")

	wanted := make(map[string]bool, len(keys))
	for _, key := range keys {
		wanted[strings.TrimSpace(key)] = true
	}

	failed := 0
	for _, target := range vms {
		config, err := vmOps.GetVMConfig(target.VMID)
		if err != nil {
			// Keep going: a read sweep that drops unreadable VMs cannot be
			// told apart from one where those VMs had nothing to show.
			failed++
			fmt.Printf("❌ VM %s (%s): %v\n\n", target.VMID, target.Name, err)
			continue
		}

		names := make([]string, 0, len(config))
		for name := range config {
			if configDisplayKeys[name] {
				continue
			}
			if len(wanted) > 0 && !wanted[name] {
				continue
			}
			names = append(names, name)
		}
		sort.Strings(names)

		if len(names) == 0 {
			// With a single target and an explicit --key, no match is an error
			// (that is the existing single-VM contract). Across several VMs it
			// is a note, since keys legitimately differ between VMs.
			if len(wanted) > 0 {
				if len(vms) == 1 {
					return fmt.Errorf("none of the requested keys are set on VM %s", target.VMID)
				}
				fmt.Printf("VM %s (%s): none of the requested keys are set\n\n", target.VMID, target.Name)
				continue
			}
			fmt.Printf("VM %s (%s) has no configuration entries\n\n", target.VMID, target.Name)
			continue
		}

		fmt.Printf("Configuration for VM %s (%s):\n\n", target.VMID, target.Name)
		for _, name := range names {
			fmt.Printf("  %-20s %s\n", name, formatConfigValue(config[name]))
		}
		fmt.Println()
	}

	// Report the denominator so an empty result cannot be mistaken for a
	// failed scan.
	if len(vms) > 1 {
		fmt.Printf("%d VM(s) read, %d failed\n", len(vms)-failed, failed)
	}

	if failed > 0 {
		return fmt.Errorf("failed to read configuration for %d of %d VM(s)", failed, len(vms))
	}

	return nil
}

// configWriteTarget is one VM staged for a config write, carrying the digest
// read alongside the values used to build its diff. The digest is per-VM, so it
// has to be captured in the same read that produced the preview.
type configWriteTarget struct {
	vm     *vm.VM
	digest string
}

func runConfigSetCommand(cmd *cobra.Command, args []string) error {
	vms, err := configTargets(cmd)
	if err != nil {
		return err
	}
	sets, _ := cmd.Flags().GetStringArray("set")
	deletes, _ := cmd.Flags().GetStringArray("delete")
	noDigestCheck, _ := cmd.Flags().GetBool("no-digest-check")

	if len(sets) == 0 && len(deletes) == 0 {
		return fmt.Errorf("at least one --set or --delete must be specified")
	}

	// The same change set is applied to every VM, so parse it once.
	settings, err := parseKeyValuePairs(sets)
	if err != nil {
		return err
	}

	setKeys := make([]string, 0, len(settings))
	for key := range settings {
		setKeys = append(setKeys, key)
	}
	sort.Strings(setKeys)

	// Read every VM's config up front: it both builds the diff and captures the
	// digest guarding each write against a concurrent modification.
	targets := make([]configWriteTarget, 0, len(vms))
	readFailed := 0

	fmt.Printf("Configuration changes for %d VM(s):\n\n", len(vms))

	for _, target := range vms {
		current, err := vmOps.GetVMConfig(target.VMID)
		if err != nil {
			// Excluded from the write set, and reported -- never silently skipped.
			readFailed++
			fmt.Printf("❌ VM %s (%s): cannot read config, will not be modified: %v\n\n",
				target.VMID, target.Name, err)
			continue
		}

		digest := ""
		if !noDigestCheck {
			if d, ok := current["digest"].(string); ok {
				digest = d
			}
		}

		fmt.Printf("VM %s (%s):\n", target.VMID, target.Name)
		for _, key := range setKeys {
			old := "(unset)"
			if value, ok := current[key]; ok {
				old = formatConfigValue(value)
			}
			suffix := ""
			if old == settings[key] {
				suffix = "  (no change)"
			}
			fmt.Printf("  %-20s %s → %s%s\n", key, old, settings[key], suffix)
		}
		for _, key := range deletes {
			key = strings.TrimSpace(key)
			old := "(unset)"
			suffix := ""
			if value, ok := current[key]; ok {
				old = formatConfigValue(value)
			} else {
				suffix = "  (no change)"
			}
			fmt.Printf("  %-20s %s → (deleted)%s\n", key, old, suffix)
		}
		fmt.Println()

		targets = append(targets, configWriteTarget{vm: target, digest: digest})
	}

	if len(targets) == 0 {
		return fmt.Errorf("no VMs can be modified: failed to read configuration for all %d target(s)", len(vms))
	}

	if dryRun {
		printDryRunHeader()
		for _, t := range targets {
			printDryRunAction("config set", t.vm.VMID, t.vm.Name,
				fmt.Sprintf("%d set, %d deleted", len(settings), len(deletes)))
		}
		printDryRunSummary("config set", len(targets))
		return nil
	}

	// Typed count, not a plain y/N: the selector can target many VMs at once,
	// so the operator has to have read the number off the diff above.
	//
	// Deliberately NOT calling autoDetectBatchMode here. It sets batchMode for
	// >1 VM, and confirmExactText hard-fails when batchMode is set without -y,
	// which would make this prompt unreachable. cmd_vm_snapshot_cluster.go
	// omits it for the same reason.
	if !confirmExactText(fmt.Sprintf("Type '%d' to confirm: ", len(targets)), strconv.Itoa(len(targets))) {
		fmt.Println("Operation cancelled")
		return nil
	}

	// Applied sequentially: SetVMConfig is a synchronous PUT with no task to
	// track, and sequential output keeps per-VM attribution unambiguous.
	succeeded := make([]string, 0, len(targets))
	writeFailed := 0

	for _, t := range targets {
		if err := vmOps.SetVMConfig(t.vm.VMID, settings, deletes, t.digest); err != nil {
			writeFailed++
			fmt.Printf("❌ VM %s (%s): %v\n", t.vm.VMID, t.vm.Name, err)
			continue
		}
		succeeded = append(succeeded, t.vm.VMID)
		fmt.Printf("✅ VM %s (%s) configuration updated\n", t.vm.VMID, t.vm.Name)
	}

	fmt.Printf("\n%d succeeded, %d failed\n", len(succeeded), writeFailed+readFailed)

	if len(succeeded) > 0 {
		fmt.Printf("   Run 'pve vm config pending --vmid %s' to check for changes awaiting a power cycle.\n",
			strings.Join(succeeded, ","))
	}

	if total := writeFailed + readFailed; total > 0 {
		return fmt.Errorf("failed to update %d of %d VM(s)", total, len(vms))
	}

	return nil
}

func runConfigPendingCommand(cmd *cobra.Command, args []string) error {
	vms, err := configTargets(cmd)
	if err != nil {
		return err
	}

	withPending := 0
	withNone := 0
	failed := 0

	for _, target := range vms {
		changes, err := vmOps.GetVMPending(target.VMID)
		if err != nil {
			failed++
			fmt.Printf("❌ VM %s (%s): %v\n\n", target.VMID, target.Name, err)
			continue
		}

		// The endpoint returns the whole config; only entries with a pending
		// value or a pending delete are actually staged changes.
		staged := 0
		for _, change := range changes {
			if change.Pending == "" && !change.Delete {
				continue
			}
			if staged == 0 {
				fmt.Printf("Pending changes for VM %s (%s) (apply on next stop/start or reboot):\n\n",
					target.VMID, target.Name)
			}
			staged++

			if change.Delete {
				fmt.Printf("  %-20s %s → (deleted)\n", change.Key, change.Value)
				continue
			}

			current := change.Value
			if current == "" {
				current = "(unset)"
			}
			fmt.Printf("  %-20s %s → %s\n", change.Key, current, change.Pending)
		}

		if staged == 0 {
			withNone++
			fmt.Printf("VM %s (%s) has no pending configuration changes\n\n", target.VMID, target.Name)
			continue
		}

		withPending++
		fmt.Printf("\n%d pending change(s)\n\n", staged)
	}

	if len(vms) > 1 {
		fmt.Printf("%d VM(s) with pending changes, %d with none, %d failed\n",
			withPending, withNone, failed)
	}

	if failed > 0 {
		return fmt.Errorf("failed to read pending changes for %d of %d VM(s)", failed, len(vms))
	}

	return nil
}

// parseKeyValuePairs turns "key=value" arguments into a map. Values may
// themselves contain "=" (as in net0=virtio,bridge=vmbr0), so only the first
// separator is significant.
func parseKeyValuePairs(pairs []string) (map[string]string, error) {
	settings := make(map[string]string, len(pairs))

	for _, pair := range pairs {
		key, value, found := strings.Cut(pair, "=")
		if !found {
			return nil, fmt.Errorf("invalid --set %q: expected key=value", pair)
		}

		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("invalid --set %q: empty key", pair)
		}

		settings[key] = value
	}

	return settings, nil
}

// formatConfigValue renders a config value from the API for display. Values
// arrive as strings or numbers depending on the key.
func formatConfigValue(value interface{}) string {
	switch v := value.(type) {
	case nil:
		return "(unset)"
	case string:
		return v
	case float64:
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v))
		}
		return fmt.Sprintf("%g", v)
	case bool:
		if v {
			return "1"
		}
		return "0"
	default:
		return fmt.Sprintf("%v", v)
	}
}
