package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/yg-codes/proxmox-cli/pkg/vm"
)

// vmAgentCmd groups the QEMU guest agent operations.
var vmAgentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Query and control the QEMU guest agent",
	Long: `Talk to the QEMU guest agent running inside a VM.

These commands need 'agent: 1' in the VM config AND qemu-guest-agent installed
and running in the guest. Two failure modes look alike and are worth knowing
apart:

  * "guest agent is not running" — the agent is absent, or the VM has not
    finished booting. On Rocky 10 the agent took about five minutes after boot
    to answer, and reports exactly this until it does.
  * "Command X has been disabled" — the agent IS running and refused. That is
    the guest's own policy: hardened images commonly disable guest-exec and all
    guest-file-* commands. 'pve vm agent info' lists what a given guest permits.

Examples:
  pve vm agent ping --vmid 100
  pve vm agent info --vmid 100
  pve vm agent osinfo --vmid 100
  pve vm agent network --vmid 100
  pve vm agent fsinfo --vmid 100
  pve vm agent users --vmid 100
  pve vm agent exec --vmid 100 -- uname -a
  pve vm agent fstrim --vmid 100`,
}

// simple read-only agent queries: subcommand name -> endpoint
var agentQueries = []struct {
	use      string
	endpoint string
	short    string
}{
	{"osinfo", "get-osinfo", "Guest OS name, version and kernel"},
	{"hostname", "get-host-name", "Guest hostname"},
	{"time", "get-time", "Guest clock, as nanoseconds since the epoch"},
	{"timezone", "get-timezone", "Guest timezone"},
	{"users", "get-users", "Users currently logged in to the guest"},
	{"vcpus", "get-vcpus", "Guest vCPU list and online state"},
	{"fsinfo", "get-fsinfo", "Guest filesystems, mountpoints and usage"},
	{"network", "network-get-interfaces", "Guest network interfaces and addresses"},
	{"memory-blocks", "get-memory-blocks", "Guest memory blocks"},
	{"memory-block-info", "get-memory-block-info", "Guest memory block size info"},
}

// action endpoints that take no parameters
var agentActions = []struct {
	use      string
	endpoint string
	short    string
	warn     string
}{
	{"ping", "ping", "Check that the guest agent responds", ""},
	{"fstrim", "fstrim", "Run fstrim on the guest's mounted filesystems", ""},
	{"fsfreeze-freeze", "fsfreeze-freeze", "Freeze the guest's filesystems",
		"the guest's filesystems stay frozen until thawed — I/O blocks meanwhile"},
	{"fsfreeze-thaw", "fsfreeze-thaw", "Thaw the guest's filesystems", ""},
	{"fsfreeze-status", "fsfreeze-status", "Report whether the guest is frozen", ""},
	{"shutdown", "shutdown", "Shut down the guest via the agent",
		"this powers the guest off"},
	{"suspend-ram", "suspend-ram", "Suspend the guest to RAM", "this suspends the guest"},
	{"suspend-disk", "suspend-disk", "Suspend the guest to disk", "this suspends the guest"},
	{"suspend-hybrid", "suspend-hybrid", "Hybrid-suspend the guest", "this suspends the guest"},
}

func initVMAgentCommands() {
	vmAgentCmd.PersistentFlags().String("vmid", "", "VM ID (or use --vmname)")
	vmAgentCmd.PersistentFlags().String("vmname", "", "VM name, matched exactly (alternative to --vmid)")
	vmAgentCmd.PersistentFlags().Bool("json", false, "Print the raw agent payload as JSON")

	// info gets its own handler: it summarises rather than dumping, because the
	// enabled/disabled split is the useful part.
	agentInfoCmd := &cobra.Command{
		Use:   "info",
		Short: "Show which agent commands this guest permits",
		RunE:  runAgentInfo,
	}
	vmAgentCmd.AddCommand(agentInfoCmd)

	for _, q := range agentQueries {
		endpoint := q.endpoint
		vmAgentCmd.AddCommand(&cobra.Command{
			Use:   q.use,
			Short: q.short,
			RunE: func(cmd *cobra.Command, args []string) error {
				return runAgentQuery(cmd, endpoint)
			},
		})
	}

	for _, a := range agentActions {
		endpoint, warn := a.endpoint, a.warn
		vmAgentCmd.AddCommand(&cobra.Command{
			Use:   a.use,
			Short: a.short,
			RunE: func(cmd *cobra.Command, args []string) error {
				return runAgentAction(cmd, endpoint, warn)
			},
		})
	}

	agentExecCmd := &cobra.Command{
		Use:   "exec",
		Short: "Run a command inside the guest and wait for its output",
		Long: `Run a command inside the guest via the agent.

The API returns only a PID, so this polls exec-status until the command exits
and then prints its stdout, stderr and exit code — otherwise the result would be
invisible.

Put the command after -- so its own flags are not parsed by pve:
  pve vm agent exec --vmid 100 -- systemctl is-active sshd

Requires guest-exec to be enabled INSIDE the guest. Rocky's default
configuration disables it, along with all guest-file-* commands; check with
'pve vm agent info'.`,
		RunE: runAgentExec,
	}
	agentExecCmd.Flags().Duration("timeout", 30*time.Second, "How long to wait for the command to finish")
	agentExecCmd.Flags().String("input-data", "", "Data to pass to the command on stdin")
	vmAgentCmd.AddCommand(agentExecCmd)

	agentPasswdCmd := &cobra.Command{
		Use:   "set-password",
		Short: "Set a user's password inside the guest",
		RunE:  runAgentSetPassword,
	}
	agentPasswdCmd.Flags().String("username", "", "Guest username (required)")
	agentPasswdCmd.Flags().String("password", "", "New password (required)")
	agentPasswdCmd.Flags().Bool("crypted", false, "The password is already crypt(3)-hashed")
	vmAgentCmd.AddCommand(agentPasswdCmd)
}

func agentVMID(cmd *cobra.Command) (string, error) {
	return vmTargetFromFlags(cmd)
}

func runAgentQuery(cmd *cobra.Command, endpoint string) error {
	vmid, err := agentVMID(cmd)
	if err != nil {
		return err
	}

	result, err := vmOps.AgentGet(vmid, endpoint)
	if err != nil {
		return err
	}

	out, err := vm.FormatAgentJSON(result)
	if err != nil {
		return err
	}
	fmt.Println(out)
	return nil
}

func runAgentAction(cmd *cobra.Command, endpoint, warn string) error {
	vmid, err := agentVMID(cmd)
	if err != nil {
		return err
	}

	if warn != "" {
		fmt.Printf("Agent %s on VM %s — %s\n\n", endpoint, vmid, warn)
		if dryRun {
			printDryRunHeader()
			printDryRunAction("agent "+endpoint, vmid, "", warn)
			printDryRunSummary("agent "+endpoint, 1)
			return nil
		}
		if !confirmOperation(fmt.Sprintf("Run agent %s on VM %s?", endpoint, vmid)) {
			fmt.Println("Operation cancelled")
			return nil
		}
	} else if dryRun {
		printDryRunHeader()
		printDryRunAction("agent "+endpoint, vmid, "", "")
		printDryRunSummary("agent "+endpoint, 1)
		return nil
	}

	result, err := vmOps.AgentPost(vmid, endpoint, nil)
	if err != nil {
		return err
	}

	// A bare ping returns an empty object; saying so beats printing "{}".
	if isEmptyAgentResult(result) {
		fmt.Printf("✅ agent %s on VM %s: OK\n", endpoint, vmid)
		return nil
	}
	out, err := vm.FormatAgentJSON(result)
	if err != nil {
		return err
	}
	fmt.Println(out)
	return nil
}

func runAgentInfo(cmd *cobra.Command, args []string) error {
	vmid, err := agentVMID(cmd)
	if err != nil {
		return err
	}

	info, err := vmOps.GetAgentInfo(vmid)
	if err != nil {
		return err
	}

	asJSON, _ := cmd.Flags().GetBool("json")
	if asJSON {
		out, err := vm.FormatAgentJSON(map[string]interface{}{
			"enabled": info.Enabled, "disabled": info.Disabled,
		})
		if err != nil {
			return err
		}
		fmt.Println(out)
		return nil
	}

	fmt.Printf("Guest agent on VM %s: %d command(s) enabled, %d disabled\n\n",
		vmid, len(info.Enabled), len(info.Disabled))

	if len(info.Disabled) > 0 {
		fmt.Println("Disabled by the guest (these will be refused whatever the API allows):")
		for _, name := range info.Disabled {
			fmt.Printf("  ❌ %s\n", name)
		}
		fmt.Println()
	}
	fmt.Println("Enabled:")
	for _, name := range info.Enabled {
		fmt.Printf("  ✅ %s\n", name)
	}
	return nil
}

func runAgentExec(cmd *cobra.Command, args []string) error {
	vmid, err := agentVMID(cmd)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return fmt.Errorf("a command is required, after --: pve vm agent exec --vmid %s -- uname -a", vmid)
	}

	timeout, _ := cmd.Flags().GetDuration("timeout")
	inputData, _ := cmd.Flags().GetString("input-data")

	if dryRun {
		printDryRunHeader()
		printDryRunAction("agent exec", vmid, "", strings.Join(args, " "))
		printDryRunSummary("agent exec", 1)
		return nil
	}

	result, err := vmOps.AgentExec(vmid, args, inputData, timeout)
	if err != nil {
		return err
	}

	if result.Stdout != "" {
		fmt.Print(result.Stdout)
		if !strings.HasSuffix(result.Stdout, "\n") {
			fmt.Println()
		}
	}
	if result.Stderr != "" {
		_, _ = fmt.Fprint(cmd.ErrOrStderr(), result.Stderr)
		if !strings.HasSuffix(result.Stderr, "\n") {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr())
		}
	}

	if result.TimedOut {
		return fmt.Errorf("command did not finish within %s (pid %d is still running in the guest)",
			timeout, result.PID)
	}
	if result.Signal != 0 {
		return fmt.Errorf("command was killed by signal %d", result.Signal)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("command exited %d", result.ExitCode)
	}
	return nil
}

func runAgentSetPassword(cmd *cobra.Command, args []string) error {
	vmid, err := agentVMID(cmd)
	if err != nil {
		return err
	}
	username, _ := cmd.Flags().GetString("username")
	password, _ := cmd.Flags().GetString("password")
	crypted, _ := cmd.Flags().GetBool("crypted")

	if username == "" {
		return fmt.Errorf("--username must be specified")
	}
	if password == "" {
		return fmt.Errorf("--password must be specified")
	}

	if dryRun {
		printDryRunHeader()
		printDryRunAction("agent set-password", vmid, "", "for guest user "+username)
		printDryRunSummary("agent set-password", 1)
		return nil
	}

	if !confirmOperation(fmt.Sprintf("Set the password for guest user %s on VM %s?", username, vmid)) {
		fmt.Println("Operation cancelled")
		return nil
	}

	if err := vmOps.SetGuestUserPassword(vmid, username, password, crypted); err != nil {
		return err
	}
	fmt.Printf("✅ Password set for guest user %s on VM %s\n", username, vmid)
	return nil
}

// isEmptyAgentResult reports whether a payload carries nothing worth printing.
func isEmptyAgentResult(v interface{}) bool {
	switch t := v.(type) {
	case nil:
		return true
	case map[string]interface{}:
		return len(t) == 0
	case []interface{}:
		return len(t) == 0
	default:
		return false
	}
}
