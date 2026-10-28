package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yg-codes/proxmox-cli/pkg/vm"
)

// vmFirewallCmd groups the VM firewall operations.
var vmFirewallCmd = &cobra.Command{
	Use:     "firewall",
	Aliases: []string{"fw"},
	Short:   "Manage a VM's firewall rules, options, aliases and IP sets",
	Long: `Manage the per-VM Proxmox firewall.

Two things about rules that matter in practice:

  * Rules have NO stable id. A rule is identified by its position in the chain,
    so inserting or deleting one renumbers everything after it. Deleting several
    rules means working from the highest position downwards.
  * A new rule goes to the TOP of the chain unless --pos says otherwise, so it
    takes precedence over the existing ones.

The VM firewall only takes effect when it is enabled both on the VM
('pve vm firewall options set --vmid N --enable') and on the network device
(firewall=1 in the netN config).

Reads need VM.Audit; changes need VM.Config.Network.

Examples:
  pve vm firewall rules list --vmid 100
  pve vm firewall rules add --vmid 100 --type in --action ACCEPT --proto tcp --dport 22
  pve vm firewall rules delete --vmid 100 --pos 0
  pve vm firewall options get --vmid 100
  pve vm firewall alias list --vmid 100
  pve vm firewall ipset list --vmid 100
  pve vm firewall refs --vmid 100`,
}

// ----- rules ---------------------------------------------------------------

var vmFwRulesCmd = &cobra.Command{
	Use:   "rules",
	Short: "List, add, change, move and delete firewall rules",
}

var vmFwRulesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List a VM's firewall rules in chain order",
	RunE: func(cmd *cobra.Command, args []string) error {
		vmid, err := fwVMID(cmd)
		if err != nil {
			return err
		}
		rules, err := vmOps.ListFirewallRules(vmid)
		if err != nil {
			return err
		}
		if len(rules) == 0 {
			fmt.Printf("\nVM %s has no firewall rules\n", vmid)
			return nil
		}
		fmt.Printf("\nFirewall rules for VM %s (in chain order):\n\n", vmid)
		fmt.Printf("%-4s %-8s %-10s %-4s %-7s %-18s %-18s %-8s %s\n",
			"POS", "TYPE", "ACTION", "ON", "PROTO", "SOURCE", "DEST", "DPORT", "COMMENT")
		fmt.Println(strings.Repeat("-", 104))
		for _, r := range rules {
			on := "yes"
			if r.Enable == 0 {
				on = "no"
			}
			fmt.Printf("%-4d %-8s %-10s %-4s %-7s %-18s %-18s %-8s %s\n",
				r.Pos, r.Type, r.Action, on, dashFw(r.Proto),
				dashFw(r.Source), dashFw(r.Dest), dashFw(r.DPort), r.Comment)
		}
		fmt.Println(strings.Repeat("-", 104))
		fmt.Printf("Total: %d\n", len(rules))
		return nil
	},
}

var vmFwRulesAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a firewall rule",
	Long: `Add a rule to a VM's firewall chain.

--action takes ACCEPT, REJECT, DROP, or a security group name (the API validates
it as a name pattern, not a fixed enum, which is why a group name is accepted
here too).

Without --pos the rule is inserted at the TOP of the chain and therefore takes
precedence over existing rules.

Examples:
  # Allow inbound SSH
  pve vm firewall rules add --vmid 100 --type in --action ACCEPT --proto tcp --dport 22

  # Drop everything from one subnet, logged
  pve vm firewall rules add --vmid 100 --type in --action DROP \
    --source 192.0.2.0/16 --log warning --comment 'block lab subnet'

  # Add it disabled, to enable later
  pve vm firewall rules add --vmid 100 --type in --action ACCEPT --dport 443 --disabled`,
	RunE: runFwRuleAdd,
}

var vmFwRulesUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Change or move a rule at a position",
	Long: `Change the rule at --pos.

--move-to reorders the rule and cannot be combined with field edits: the API
ignores every other argument when moving, so this command refuses the
combination rather than silently dropping the edits.

type and action cannot be deleted — they are required. Change them instead.`,
	RunE: runFwRuleUpdate,
}

var vmFwRulesDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete the rule at a position",
	Long: `Delete a firewall rule by position.

Positions shift after a delete: removing rule 0 makes the old rule 1 become
rule 0. To remove several, work from the highest position downwards, or re-list
between deletes.`,
	RunE: runFwRuleDelete,
}

// ----- options -------------------------------------------------------------

var vmFwOptionsCmd = &cobra.Command{
	Use:   "options",
	Short: "Read and change a VM's firewall options",
}

var vmFwOptionsGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Show a VM's firewall options",
	RunE: func(cmd *cobra.Command, args []string) error {
		vmid, err := fwVMID(cmd)
		if err != nil {
			return err
		}
		o, err := vmOps.GetFirewallOptions(vmid)
		if err != nil {
			return err
		}
		fmt.Printf("\nFirewall options for VM %s:\n\n", vmid)
		fmt.Printf("  %-14s %s\n", "enable", boolLabel(o.Enable, "0 (firewall off)"))
		fmt.Printf("  %-14s %s\n", "policy_in", defaultLabel(o.PolicyIn, "DROP (API default)"))
		fmt.Printf("  %-14s %s\n", "policy_out", defaultLabel(o.PolicyOut, "ACCEPT (API default)"))
		fmt.Printf("  %-14s %s\n", "macfilter", boolLabel(o.MACFilter, "1 (default)"))
		fmt.Printf("  %-14s %s\n", "dhcp", boolLabel(o.DHCP, "0 (default)"))
		fmt.Printf("  %-14s %s\n", "ndp", boolLabel(o.NDP, "1 (default)"))
		fmt.Printf("  %-14s %s\n", "radv", boolLabel(o.RADV, "unset"))
		fmt.Printf("  %-14s %s\n", "ipfilter", boolLabel(o.IPFilter, "unset"))
		fmt.Printf("  %-14s %s\n", "log_level_in", defaultLabel(o.LogLevelIn, "unset"))
		fmt.Printf("  %-14s %s\n", "log_level_out", defaultLabel(o.LogLevelOut, "unset"))
		if o.Digest != "" {
			fmt.Printf("\n  digest: %s\n", o.Digest)
		}
		if o.Enable == nil || !*o.Enable {
			fmt.Println("\n  ⚠️  The firewall is not enabled on this VM, so no rules are applied.")
			fmt.Println("     It must also be enabled per network device (firewall=1 in netN).")
		}
		return nil
	},
}

var vmFwOptionsSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Change a VM's firewall options",
	RunE:  runFwOptionsSet,
}

// ----- aliases -------------------------------------------------------------

var vmFwAliasCmd = &cobra.Command{
	Use:     "alias",
	Aliases: []string{"aliases"},
	Short:   "Manage named CIDRs usable in rules",
}

var vmFwAliasListCmd = &cobra.Command{
	Use:   "list",
	Short: "List a VM's firewall aliases",
	RunE: func(cmd *cobra.Command, args []string) error {
		vmid, err := fwVMID(cmd)
		if err != nil {
			return err
		}
		aliases, err := vmOps.ListFirewallAliases(vmid)
		if err != nil {
			return err
		}
		if len(aliases) == 0 {
			fmt.Printf("\nVM %s has no firewall aliases\n", vmid)
			return nil
		}
		fmt.Printf("\nFirewall aliases for VM %s:\n\n", vmid)
		fmt.Printf("%-20s %-24s %s\n", "NAME", "CIDR", "COMMENT")
		fmt.Println(strings.Repeat("-", 70))
		for _, a := range aliases {
			fmt.Printf("%-20s %-24s %s\n", a.Name, a.CIDR, a.Comment)
		}
		fmt.Println(strings.Repeat("-", 70))
		fmt.Printf("Total: %d\n", len(aliases))
		return nil
	},
}

var vmFwAliasAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a named CIDR",
	RunE:  runFwAliasAdd,
}

var vmFwAliasUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Change an alias's CIDR, comment or name",
	RunE:  runFwAliasUpdate,
}

var vmFwAliasDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete an alias",
	RunE:  runFwAliasDelete,
}

// ----- ipset ---------------------------------------------------------------

var vmFwIPSetCmd = &cobra.Command{
	Use:   "ipset",
	Short: "Manage IP sets — named collections of CIDRs",
	Long: `Manage a VM's IP sets.

An IP set groups CIDRs under one name, referenced in a rule as +name. An entry
marked --nomatch inverts that entry: the set matches everything except it.

The API has no update for the set itself; renaming or changing its comment goes
through 'add' with --rename (set --rename equal to --name to change only the
comment).`,
}

var vmFwIPSetListCmd = &cobra.Command{
	Use:   "list",
	Short: "List IP sets, or the entries in one with --name",
	RunE:  runFwIPSetList,
}

var vmFwIPSetCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create an IP set, or rename/re-comment one with --rename",
	RunE:  runFwIPSetCreate,
}

var vmFwIPSetDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete an IP set",
	Long: `Delete an IP set.

Proxmox refuses to delete a set that still has entries; --force removes its
members along with it.`,
	RunE: runFwIPSetDelete,
}

var vmFwIPSetAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a CIDR to an IP set",
	RunE:  runFwIPSetAdd,
}

var vmFwIPSetUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Change an entry's nomatch flag or comment",
	RunE:  runFwIPSetUpdate,
}

var vmFwIPSetRemoveCmd = &cobra.Command{
	Use:   "remove",
	Short: "Remove a CIDR from an IP set",
	RunE:  runFwIPSetRemove,
}

// ----- refs ----------------------------------------------------------------

var vmFwRefsCmd = &cobra.Command{
	Use:   "refs",
	Short: "List aliases and IP sets that rules may reference",
	Long: `List the aliases and IP sets available to this VM's rules.

This includes datacenter- and SDN-scoped entries, not just the VM's own — which
is why a rule can reference a name that 'alias list' does not show.`,
	RunE: runFwRefs,
}

func initVMFirewallCommands() {
	vmFirewallCmd.PersistentFlags().String("vmid", "", "VM ID (or use --vmname)")
	vmFirewallCmd.PersistentFlags().String("vmname", "", "VM name, matched exactly (alternative to --vmid)")

	vmFirewallCmd.AddCommand(vmFwRulesCmd, vmFwOptionsCmd, vmFwAliasCmd, vmFwIPSetCmd, vmFwRefsCmd)
	vmFwRulesCmd.AddCommand(vmFwRulesListCmd, vmFwRulesAddCmd, vmFwRulesUpdateCmd, vmFwRulesDeleteCmd)
	vmFwOptionsCmd.AddCommand(vmFwOptionsGetCmd, vmFwOptionsSetCmd)
	vmFwAliasCmd.AddCommand(vmFwAliasListCmd, vmFwAliasAddCmd, vmFwAliasUpdateCmd, vmFwAliasDeleteCmd)
	vmFwIPSetCmd.AddCommand(vmFwIPSetListCmd, vmFwIPSetCreateCmd, vmFwIPSetDeleteCmd,
		vmFwIPSetAddCmd, vmFwIPSetUpdateCmd, vmFwIPSetRemoveCmd)

	// Rule field flags, shared by add and update.
	for _, c := range []*cobra.Command{vmFwRulesAddCmd, vmFwRulesUpdateCmd} {
		c.Flags().String("type", "", "Rule type: "+strings.Join(vm.FirewallRuleTypes(), ", "))
		c.Flags().String("action", "", "ACCEPT, REJECT, DROP, or a security group name")
		c.Flags().String("macro", "", "Predefined macro, e.g. SSH, HTTP")
		c.Flags().String("iface", "", "Network device, e.g. net0")
		c.Flags().String("source", "", "Source IP, CIDR, alias or +ipset")
		c.Flags().String("dest", "", "Destination IP, CIDR, alias or +ipset")
		c.Flags().String("proto", "", "Protocol name or number, e.g. tcp, udp, icmp")
		c.Flags().String("sport", "", "Source port, range or list")
		c.Flags().String("dport", "", "Destination port, range or list")
		c.Flags().String("comment", "", "Free-text comment")
		c.Flags().String("log", "", "Log level: "+strings.Join(vm.FirewallLogLevels(), ", "))
		c.Flags().String("icmp-type", "", "ICMP type (only with --proto icmp or icmpv6)")
		c.Flags().Bool("disabled", false, "Add/leave the rule disabled")
	}
	vmFwRulesAddCmd.Flags().Int("pos", -1, "Insert at this position (default: top of the chain)")
	vmFwRulesUpdateCmd.Flags().Int("pos", -1, "Position of the rule to change (required)")
	vmFwRulesUpdateCmd.Flags().Int("move-to", -1, "Move the rule to this position (cannot be combined with edits)")
	vmFwRulesUpdateCmd.Flags().StringSlice("delete", nil, "Rule fields to reset (type and action cannot be deleted)")
	vmFwRulesDeleteCmd.Flags().Int("pos", -1, "Position of the rule to delete (required)")

	vmFwOptionsSetCmd.Flags().String("policy-in", "", "Input policy: "+strings.Join(vm.FirewallPolicies(), ", "))
	vmFwOptionsSetCmd.Flags().String("policy-out", "", "Output policy: "+strings.Join(vm.FirewallPolicies(), ", "))
	vmFwOptionsSetCmd.Flags().String("log-level-in", "", "Log level for inbound traffic")
	vmFwOptionsSetCmd.Flags().String("log-level-out", "", "Log level for outbound traffic")
	vmFwOptionsSetCmd.Flags().Bool("enable", false, "Enable the firewall on this VM")
	vmFwOptionsSetCmd.Flags().Bool("disable", false, "Disable the firewall on this VM")
	vmFwOptionsSetCmd.Flags().Bool("dhcp", false, "Allow DHCP")
	vmFwOptionsSetCmd.Flags().Bool("no-dhcp", false, "Disallow DHCP")
	vmFwOptionsSetCmd.Flags().Bool("macfilter", false, "Enable the MAC address filter")
	vmFwOptionsSetCmd.Flags().Bool("no-macfilter", false, "Disable the MAC address filter")
	vmFwOptionsSetCmd.Flags().Bool("ipfilter", false, "Enable default IP filters")
	vmFwOptionsSetCmd.Flags().Bool("no-ipfilter", false, "Disable default IP filters")
	vmFwOptionsSetCmd.Flags().Bool("ndp", false, "Allow NDP")
	vmFwOptionsSetCmd.Flags().Bool("no-ndp", false, "Disallow NDP")
	vmFwOptionsSetCmd.Flags().Bool("radv", false, "Allow router advertisements")
	vmFwOptionsSetCmd.Flags().Bool("no-radv", false, "Disallow router advertisements")
	vmFwOptionsSetCmd.Flags().StringSlice("delete", nil, "Options to reset to their default")

	for _, c := range []*cobra.Command{vmFwAliasAddCmd, vmFwAliasUpdateCmd, vmFwAliasDeleteCmd} {
		c.Flags().String("name", "", "Alias name (required)")
	}
	for _, c := range []*cobra.Command{vmFwAliasAddCmd, vmFwAliasUpdateCmd} {
		c.Flags().String("cidr", "", "IP or CIDR")
		c.Flags().String("comment", "", "Free-text comment")
	}
	vmFwAliasUpdateCmd.Flags().String("rename", "", "New name for the alias")

	vmFwIPSetListCmd.Flags().String("name", "", "Show the entries in this set instead of listing sets")
	for _, c := range []*cobra.Command{vmFwIPSetCreateCmd, vmFwIPSetDeleteCmd,
		vmFwIPSetAddCmd, vmFwIPSetUpdateCmd, vmFwIPSetRemoveCmd} {
		c.Flags().String("name", "", "IP set name (required)")
	}
	vmFwIPSetCreateCmd.Flags().String("comment", "", "Free-text comment")
	vmFwIPSetCreateCmd.Flags().String("rename", "", "Rename an existing set (same value as --name changes only the comment)")
	vmFwIPSetDeleteCmd.Flags().Bool("force", false, "Delete the set even if it still has entries")
	for _, c := range []*cobra.Command{vmFwIPSetAddCmd, vmFwIPSetUpdateCmd, vmFwIPSetRemoveCmd} {
		c.Flags().String("cidr", "", "IP or CIDR (required)")
	}
	for _, c := range []*cobra.Command{vmFwIPSetAddCmd, vmFwIPSetUpdateCmd} {
		c.Flags().String("comment", "", "Free-text comment")
		c.Flags().Bool("nomatch", false, "Invert this entry: match everything except it")
	}

	vmFwRefsCmd.Flags().String("type", "", "Filter by type: alias or ipset")
}

func fwVMID(cmd *cobra.Command) (string, error) {
	return vmTargetFromFlags(cmd)
}

func fwRuleFromFlags(cmd *cobra.Command) *vm.FirewallRule {
	r := &vm.FirewallRule{}
	r.Type, _ = cmd.Flags().GetString("type")
	r.Action, _ = cmd.Flags().GetString("action")
	r.Macro, _ = cmd.Flags().GetString("macro")
	r.Iface, _ = cmd.Flags().GetString("iface")
	r.Source, _ = cmd.Flags().GetString("source")
	r.Dest, _ = cmd.Flags().GetString("dest")
	r.Proto, _ = cmd.Flags().GetString("proto")
	r.SPort, _ = cmd.Flags().GetString("sport")
	r.DPort, _ = cmd.Flags().GetString("dport")
	r.Comment, _ = cmd.Flags().GetString("comment")
	r.Log, _ = cmd.Flags().GetString("log")
	r.ICMPType, _ = cmd.Flags().GetString("icmp-type")
	// enable=1 unless --disabled; the API treats an absent enable as 0 on
	// create, which would silently add a rule that does nothing.
	if disabled, _ := cmd.Flags().GetBool("disabled"); !disabled {
		r.Enable = 1
	}
	return r
}

func runFwRuleAdd(cmd *cobra.Command, args []string) error {
	vmid, err := fwVMID(cmd)
	if err != nil {
		return err
	}
	rule := fwRuleFromFlags(cmd)
	if pos, _ := cmd.Flags().GetInt("pos"); pos >= 0 {
		rule.Pos = pos
	}

	if err := vm.ValidateFirewallRule(rule); err != nil {
		return err
	}

	where := "the top of the chain"
	if rule.Pos > 0 {
		where = fmt.Sprintf("position %d", rule.Pos)
	}
	fmt.Printf("Add firewall rule to VM %s at %s:\n\n", vmid, where)
	printRuleSummary(rule)
	fmt.Println()

	if dryRun {
		printDryRunHeader()
		printDryRunAction("firewall rule add", vmid, "",
			fmt.Sprintf("%s %s", rule.Type, rule.Action))
		printDryRunSummary("firewall rule add", 1)
		return nil
	}
	if !confirmOperation(fmt.Sprintf("Add this rule to VM %s?", vmid)) {
		fmt.Println("Operation cancelled")
		return nil
	}

	if err := vmOps.CreateFirewallRule(vmid, rule); err != nil {
		return err
	}
	fmt.Printf("✅ Rule added to VM %s\n", vmid)
	return nil
}

func runFwRuleUpdate(cmd *cobra.Command, args []string) error {
	vmid, err := fwVMID(cmd)
	if err != nil {
		return err
	}
	pos, _ := cmd.Flags().GetInt("pos")
	if pos < 0 {
		return fmt.Errorf("--pos must be specified (see 'pve vm firewall rules list')")
	}
	moveTo, _ := cmd.Flags().GetInt("move-to")
	deleteKeys, _ := cmd.Flags().GetStringSlice("delete")

	// Only send fields the user actually passed: fwRuleFromFlags defaults
	// enable to 1, which on an update would silently re-enable a disabled rule.
	rule := &vm.FirewallRule{}
	for flag, target := range map[string]*string{
		"type": &rule.Type, "action": &rule.Action, "macro": &rule.Macro,
		"iface": &rule.Iface, "source": &rule.Source, "dest": &rule.Dest,
		"proto": &rule.Proto, "sport": &rule.SPort, "dport": &rule.DPort,
		"comment": &rule.Comment, "log": &rule.Log, "icmp-type": &rule.ICMPType,
	} {
		if cmd.Flags().Changed(flag) {
			*target, _ = cmd.Flags().GetString(flag)
		}
	}
	if cmd.Flags().Changed("disabled") {
		if disabled, _ := cmd.Flags().GetBool("disabled"); disabled {
			// enable=0 is the API's own default for an absent value, so an
			// explicit 0 cannot be expressed through the int field; delete the
			// key instead, which resets it to 0.
			deleteKeys = append(deleteKeys, "enable")
		} else {
			rule.Enable = 1
		}
	}

	// Carry the digest so a concurrent edit is rejected rather than overwritten.
	if current, err := vmOps.GetFirewallRule(vmid, pos); err == nil {
		rule.Digest = current.Digest
	}

	if moveTo >= 0 {
		fmt.Printf("Move firewall rule %d to position %d on VM %s\n", pos, moveTo, vmid)
	} else {
		fmt.Printf("Update firewall rule %d on VM %s\n", pos, vmid)
	}

	if dryRun {
		printDryRunHeader()
		printDryRunAction("firewall rule update", vmid, "", fmt.Sprintf("pos %d", pos))
		printDryRunSummary("firewall rule update", 1)
		return nil
	}
	if !confirmOperation(fmt.Sprintf("Apply this change to rule %d on VM %s?", pos, vmid)) {
		fmt.Println("Operation cancelled")
		return nil
	}

	if err := vmOps.UpdateFirewallRule(vmid, pos, rule, deleteKeys, moveTo); err != nil {
		return err
	}
	fmt.Printf("✅ Rule %d updated on VM %s\n", pos, vmid)
	return nil
}

func runFwRuleDelete(cmd *cobra.Command, args []string) error {
	vmid, err := fwVMID(cmd)
	if err != nil {
		return err
	}
	pos, _ := cmd.Flags().GetInt("pos")
	if pos < 0 {
		return fmt.Errorf("--pos must be specified (see 'pve vm firewall rules list')")
	}

	// Show the rule before deleting it: position is not a durable reference, so
	// the operator needs to see which rule that position currently holds.
	rule, err := vmOps.GetFirewallRule(vmid, pos)
	if err != nil {
		return fmt.Errorf("could not read rule %d on VM %s: %w", pos, vmid, err)
	}

	fmt.Printf("Delete firewall rule %d on VM %s:\n\n", pos, vmid)
	printRuleSummary(rule)
	fmt.Println()
	fmt.Println("  Positions shift after a delete: rules below this one move up by one.")
	fmt.Println()

	if dryRun {
		printDryRunHeader()
		printDryRunAction("firewall rule delete", vmid, "",
			fmt.Sprintf("pos %d (%s %s)", pos, rule.Type, rule.Action))
		printDryRunSummary("firewall rule delete", 1)
		return nil
	}
	if !confirmOperation(fmt.Sprintf("Delete rule %d on VM %s?", pos, vmid)) {
		fmt.Println("Operation cancelled")
		return nil
	}

	if err := vmOps.DeleteFirewallRule(vmid, pos, rule.Digest); err != nil {
		return err
	}
	fmt.Printf("✅ Rule %d deleted on VM %s\n", pos, vmid)
	return nil
}

func runFwOptionsSet(cmd *cobra.Command, args []string) error {
	vmid, err := fwVMID(cmd)
	if err != nil {
		return err
	}

	o := &vm.FirewallOptions{}
	o.PolicyIn, _ = cmd.Flags().GetString("policy-in")
	o.PolicyOut, _ = cmd.Flags().GetString("policy-out")
	o.LogLevelIn, _ = cmd.Flags().GetString("log-level-in")
	o.LogLevelOut, _ = cmd.Flags().GetString("log-level-out")

	// Paired --x / --no-x flags, so "set false" is expressible. A bare --x with
	// a default of false could not distinguish "unset" from "set to false".
	for _, p := range []struct {
		on, off string
		target  **bool
	}{
		{"enable", "disable", &o.Enable},
		{"dhcp", "no-dhcp", &o.DHCP},
		{"macfilter", "no-macfilter", &o.MACFilter},
		{"ipfilter", "no-ipfilter", &o.IPFilter},
		{"ndp", "no-ndp", &o.NDP},
		{"radv", "no-radv", &o.RADV},
	} {
		onSet := cmd.Flags().Changed(p.on)
		offSet := cmd.Flags().Changed(p.off)
		if onSet && offSet {
			return fmt.Errorf("--%s and --%s are mutually exclusive", p.on, p.off)
		}
		if onSet {
			v := true
			*p.target = &v
		} else if offSet {
			v := false
			*p.target = &v
		}
	}

	deleteKeys, _ := cmd.Flags().GetStringSlice("delete")

	if current, err := vmOps.GetFirewallOptions(vmid); err == nil {
		o.Digest = current.Digest
	}

	fmt.Printf("Update firewall options on VM %s\n", vmid)
	if dryRun {
		printDryRunHeader()
		printDryRunAction("firewall options set", vmid, "", strings.Join(deleteKeys, ","))
		printDryRunSummary("firewall options set", 1)
		return nil
	}
	if !confirmOperation(fmt.Sprintf("Apply firewall option changes to VM %s?", vmid)) {
		fmt.Println("Operation cancelled")
		return nil
	}

	if err := vmOps.SetFirewallOptions(vmid, o, deleteKeys); err != nil {
		return err
	}
	fmt.Printf("✅ Firewall options updated on VM %s\n", vmid)
	return nil
}

func runFwAliasAdd(cmd *cobra.Command, args []string) error {
	vmid, err := fwVMID(cmd)
	if err != nil {
		return err
	}
	alias := &vm.FirewallAlias{}
	alias.Name, _ = cmd.Flags().GetString("name")
	alias.CIDR, _ = cmd.Flags().GetString("cidr")
	alias.Comment, _ = cmd.Flags().GetString("comment")

	if dryRun {
		printDryRunHeader()
		printDryRunAction("firewall alias add", vmid, alias.Name, alias.CIDR)
		printDryRunSummary("firewall alias add", 1)
		return nil
	}
	if err := vmOps.CreateFirewallAlias(vmid, alias); err != nil {
		return err
	}
	fmt.Printf("✅ Alias %s → %s added to VM %s\n", alias.Name, alias.CIDR, vmid)
	return nil
}

func runFwAliasUpdate(cmd *cobra.Command, args []string) error {
	vmid, err := fwVMID(cmd)
	if err != nil {
		return err
	}
	name, _ := cmd.Flags().GetString("name")
	rename, _ := cmd.Flags().GetString("rename")
	alias := &vm.FirewallAlias{}
	alias.CIDR, _ = cmd.Flags().GetString("cidr")
	alias.Comment, _ = cmd.Flags().GetString("comment")

	if dryRun {
		printDryRunHeader()
		printDryRunAction("firewall alias update", vmid, name, alias.CIDR)
		printDryRunSummary("firewall alias update", 1)
		return nil
	}
	if err := vmOps.UpdateFirewallAlias(vmid, name, alias, rename); err != nil {
		return err
	}
	fmt.Printf("✅ Alias %s updated on VM %s\n", name, vmid)
	return nil
}

func runFwAliasDelete(cmd *cobra.Command, args []string) error {
	vmid, err := fwVMID(cmd)
	if err != nil {
		return err
	}
	name, _ := cmd.Flags().GetString("name")

	if dryRun {
		printDryRunHeader()
		printDryRunAction("firewall alias delete", vmid, name, "")
		printDryRunSummary("firewall alias delete", 1)
		return nil
	}
	if !confirmOperation(fmt.Sprintf("Delete alias %q on VM %s?", name, vmid)) {
		fmt.Println("Operation cancelled")
		return nil
	}
	if err := vmOps.DeleteFirewallAlias(vmid, name); err != nil {
		return err
	}
	fmt.Printf("✅ Alias %s deleted on VM %s\n", name, vmid)
	return nil
}

func runFwIPSetList(cmd *cobra.Command, args []string) error {
	vmid, err := fwVMID(cmd)
	if err != nil {
		return err
	}
	name, _ := cmd.Flags().GetString("name")

	if name != "" {
		entries, err := vmOps.ListFirewallIPSetEntries(vmid, name)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			fmt.Printf("\nIP set %q on VM %s is empty\n", name, vmid)
			return nil
		}
		fmt.Printf("\nEntries in IP set %q on VM %s:\n\n", name, vmid)
		fmt.Printf("%-24s %-8s %s\n", "CIDR", "NOMATCH", "COMMENT")
		fmt.Println(strings.Repeat("-", 60))
		for _, e := range entries {
			nomatch := "no"
			if e.NoMatch {
				nomatch = "yes"
			}
			fmt.Printf("%-24s %-8s %s\n", e.CIDR, nomatch, e.Comment)
		}
		fmt.Println(strings.Repeat("-", 60))
		fmt.Printf("Total: %d\n", len(entries))
		return nil
	}

	sets, err := vmOps.ListFirewallIPSets(vmid)
	if err != nil {
		return err
	}
	if len(sets) == 0 {
		fmt.Printf("\nVM %s has no IP sets\n", vmid)
		return nil
	}
	fmt.Printf("\nIP sets on VM %s:\n\n", vmid)
	fmt.Printf("%-24s %s\n", "NAME", "COMMENT")
	fmt.Println(strings.Repeat("-", 60))
	for _, s := range sets {
		fmt.Printf("%-24s %s\n", s.Name, s.Comment)
	}
	fmt.Println(strings.Repeat("-", 60))
	fmt.Printf("Total: %d  (use --name to list a set's entries)\n", len(sets))
	return nil
}

func runFwIPSetCreate(cmd *cobra.Command, args []string) error {
	vmid, err := fwVMID(cmd)
	if err != nil {
		return err
	}
	name, _ := cmd.Flags().GetString("name")
	comment, _ := cmd.Flags().GetString("comment")
	rename, _ := cmd.Flags().GetString("rename")

	if dryRun {
		printDryRunHeader()
		printDryRunAction("firewall ipset create", vmid, name, comment)
		printDryRunSummary("firewall ipset create", 1)
		return nil
	}
	if err := vmOps.CreateFirewallIPSet(vmid, name, comment, rename); err != nil {
		return err
	}
	fmt.Printf("✅ IP set %s created on VM %s\n", name, vmid)
	return nil
}

func runFwIPSetDelete(cmd *cobra.Command, args []string) error {
	vmid, err := fwVMID(cmd)
	if err != nil {
		return err
	}
	name, _ := cmd.Flags().GetString("name")
	force, _ := cmd.Flags().GetBool("force")

	if dryRun {
		printDryRunHeader()
		printDryRunAction("firewall ipset delete", vmid, name, fmt.Sprintf("force=%v", force))
		printDryRunSummary("firewall ipset delete", 1)
		return nil
	}
	if !confirmOperation(fmt.Sprintf("Delete IP set %q on VM %s?", name, vmid)) {
		fmt.Println("Operation cancelled")
		return nil
	}
	if err := vmOps.DeleteFirewallIPSet(vmid, name, force); err != nil {
		return err
	}
	fmt.Printf("✅ IP set %s deleted on VM %s\n", name, vmid)
	return nil
}

func runFwIPSetAdd(cmd *cobra.Command, args []string) error {
	vmid, err := fwVMID(cmd)
	if err != nil {
		return err
	}
	name, _ := cmd.Flags().GetString("name")
	entry := &vm.FirewallIPSetEntry{}
	entry.CIDR, _ = cmd.Flags().GetString("cidr")
	entry.Comment, _ = cmd.Flags().GetString("comment")
	entry.NoMatch, _ = cmd.Flags().GetBool("nomatch")

	if dryRun {
		printDryRunHeader()
		printDryRunAction("firewall ipset add", vmid, name, entry.CIDR)
		printDryRunSummary("firewall ipset add", 1)
		return nil
	}
	if err := vmOps.AddFirewallIPSetEntry(vmid, name, entry); err != nil {
		return err
	}
	fmt.Printf("✅ %s added to IP set %s on VM %s\n", entry.CIDR, name, vmid)
	return nil
}

func runFwIPSetUpdate(cmd *cobra.Command, args []string) error {
	vmid, err := fwVMID(cmd)
	if err != nil {
		return err
	}
	name, _ := cmd.Flags().GetString("name")
	entry := &vm.FirewallIPSetEntry{}
	entry.CIDR, _ = cmd.Flags().GetString("cidr")
	entry.Comment, _ = cmd.Flags().GetString("comment")
	entry.NoMatch, _ = cmd.Flags().GetBool("nomatch")

	if dryRun {
		printDryRunHeader()
		printDryRunAction("firewall ipset update", vmid, name, entry.CIDR)
		printDryRunSummary("firewall ipset update", 1)
		return nil
	}
	if err := vmOps.UpdateFirewallIPSetEntry(vmid, name, entry); err != nil {
		return err
	}
	fmt.Printf("✅ %s updated in IP set %s on VM %s\n", entry.CIDR, name, vmid)
	return nil
}

func runFwIPSetRemove(cmd *cobra.Command, args []string) error {
	vmid, err := fwVMID(cmd)
	if err != nil {
		return err
	}
	name, _ := cmd.Flags().GetString("name")
	cidr, _ := cmd.Flags().GetString("cidr")

	if dryRun {
		printDryRunHeader()
		printDryRunAction("firewall ipset remove", vmid, name, cidr)
		printDryRunSummary("firewall ipset remove", 1)
		return nil
	}
	if err := vmOps.DeleteFirewallIPSetEntry(vmid, name, cidr); err != nil {
		return err
	}
	fmt.Printf("✅ %s removed from IP set %s on VM %s\n", cidr, name, vmid)
	return nil
}

func runFwRefs(cmd *cobra.Command, args []string) error {
	vmid, err := fwVMID(cmd)
	if err != nil {
		return err
	}
	refType, _ := cmd.Flags().GetString("type")

	refs, err := vmOps.ListFirewallRefs(vmid, refType)
	if err != nil {
		return err
	}
	if len(refs) == 0 {
		fmt.Printf("\nNo firewall references available to VM %s\n", vmid)
		return nil
	}
	fmt.Printf("\nFirewall references available to VM %s:\n\n", vmid)
	fmt.Printf("%-8s %-24s %-10s %s\n", "TYPE", "NAME", "SCOPE", "COMMENT")
	fmt.Println(strings.Repeat("-", 70))
	for _, r := range refs {
		fmt.Printf("%-8s %-24s %-10s %s\n", r.Type, r.Name, dashFw(r.Scope), r.Comment)
	}
	fmt.Println(strings.Repeat("-", 70))
	fmt.Printf("Total: %d\n", len(refs))
	return nil
}

func printRuleSummary(r *vm.FirewallRule) {
	fmt.Printf("  %-10s %s\n", "type:", r.Type)
	fmt.Printf("  %-10s %s\n", "action:", r.Action)
	if r.Enable == 0 {
		fmt.Printf("  %-10s no (the rule is present but not applied)\n", "enabled:")
	}
	for label, value := range map[string]string{
		"macro:": r.Macro, "iface:": r.Iface, "source:": r.Source,
		"dest:": r.Dest, "proto:": r.Proto, "sport:": r.SPort,
		"dport:": r.DPort, "log:": r.Log, "icmp-type:": r.ICMPType,
		"comment:": r.Comment,
	} {
		if value != "" {
			fmt.Printf("  %-10s %s\n", label, value)
		}
	}
}

func boolLabel(v *bool, unsetLabel string) string {
	if v == nil {
		return unsetLabel
	}
	if *v {
		return "1"
	}
	return "0"
}

func defaultLabel(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func dashFw(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
