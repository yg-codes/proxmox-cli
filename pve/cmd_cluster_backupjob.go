package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/yg-codes/proxmox-cli/pkg/backupjob"
)

var backupJobOps *backupjob.Operations

// clusterBackupJobCmd groups vzdump backup job management.
//
// Backup jobs are the scheduled definitions in jobs.cfg (Datacenter → Backup
// in the web UI) — distinct from the one-shot `pve vm backup create` and from
// backup file management. Reads need Sys.Audit on /; changes need Sys.Modify
// on /, which PVEVMAdmin/PVEAdmin do not include.
var clusterBackupJobCmd = &cobra.Command{
	Use:     "backup-job",
	Aliases: []string{"backup-jobs", "job"},
	Short:   "Manage scheduled vzdump backup jobs",
	Long: `Manage scheduled vzdump backup jobs (the cluster's jobs.cfg).

Backup jobs run vzdump on a calendar schedule and write to a storage. Guest
selection is one of --all, --vmid or --pool; --exclude removes guests from an
--all backup.

There is no digest/optimistic locking on these endpoints, and no include/exclude
filter mode — selection is the classic trio above.

Permissions: reads need Sys.Audit on /, changes need Sys.Modify on /
(PVEAuditor / PVESysAdmin respectively; PVEVMAdmin includes neither).
tmpdir/dumpdir/script are root@pam-only and are not exposed.

Examples:
  pve cluster backup-job list
  pve cluster backup-job create --schedule 'sat 02:30' --storage local --vmid 4001,4002
  pve cluster backup-job update --id weekly-vms --disable
  pve cluster backup-job delete --id weekly-vms
  pve cluster backup-job not-covered`,
}

var clusterBackupJobListCmd = &cobra.Command{
	Use:   "list",
	Short: "List scheduled backup jobs",
	RunE:  runBackupJobList,
}

var clusterBackupJobCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a backup job",
	RunE:  runBackupJobCreate,
}

var clusterBackupJobUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Change a backup job",
	RunE:  runBackupJobUpdate,
}

var clusterBackupJobDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Remove a backup job",
	Long: `Remove a backup job definition.

Only the schedule is deleted — existing backup files are untouched.`,
	RunE: runBackupJobDelete,
}

var clusterBackupJobNotCoveredCmd = &cobra.Command{
	Use:   "not-covered",
	Short: "Show guests covered by no backup job",
	RunE:  runBackupJobNotCovered,
}

func initClusterBackupJobCommands() {
	clusterCmd.AddCommand(clusterBackupJobCmd)
	clusterBackupJobCmd.AddCommand(clusterBackupJobListCmd, clusterBackupJobCreateCmd,
		clusterBackupJobUpdateCmd, clusterBackupJobDeleteCmd, clusterBackupJobNotCoveredCmd)

	clusterBackupJobListCmd.Flags().String("id", "", "Show only this job ID (exact match)")

	backupJobCreateFlags(clusterBackupJobCreateCmd.Flags())
	clusterBackupJobCreateCmd.Flags().String("id", "",
		"Optional job ID (pve-configid, e.g. weekly-vms; generated when omitted)")

	backupJobCreateFlags(clusterBackupJobUpdateCmd.Flags())
	clusterBackupJobUpdateCmd.Flags().String("id", "", "Job ID to change (required)")
	clusterBackupJobUpdateCmd.Flags().StringSlice("delete", nil,
		"Settings to reset to their default, e.g. bwlimit,protected")

	clusterBackupJobDeleteCmd.Flags().String("id", "", "Job ID to remove (required)")
}

func runBackupJobList(cmd *cobra.Command, args []string) error {
	jobs, err := backupJobOps.ListJobs()
	if err != nil {
		return err
	}
	idFilter, _ := cmd.Flags().GetString("id")
	if idFilter != "" {
		filtered := jobs[:0]
		for _, j := range jobs {
			if j.ID == idFilter {
				filtered = append(filtered, j)
			}
		}
		jobs = filtered
	}
	if len(jobs) == 0 {
		if idFilter != "" {
			fmt.Printf("\nNo backup job with ID %q\n", idFilter)
		} else {
			fmt.Println("\nNo backup jobs configured")
			fmt.Println("  Create one with: pve cluster backup-job create --schedule 'sat 02:30' \\")
			fmt.Println("      --storage <storage> --vmid <ids> (or --all / --pool <pool>)")
		}
		return nil
	}
	fmt.Printf("\n%-24s %-8s %-16s %-19s %-9s %-12s %s\n",
		"JOB", "STATE", "SCHEDULE", "NEXT-RUN", "MODE", "STORAGE", "GUESTS")
	fmt.Println(strings.Repeat("-", 118))
	for _, j := range jobs {
		state := "enabled"
		if j.Enabled != nil && !*j.Enabled {
			state = "disabled"
		}
		nextRun := "-"
		if j.NextRun > 0 {
			nextRun = epoch(float64(j.NextRun))
		}
		fmt.Printf("%-24s %-8s %-16s %-19s %-9s %-12s %s\n",
			dash(j.ID), state, dash(j.Schedule), nextRun, dash(j.Mode), dash(j.Storage),
			backupJobGuests(j))
		if j.Comment != "" {
			fmt.Printf("  ↳ %s\n", j.Comment)
		}
	}
	fmt.Println(strings.Repeat("-", 118))
	fmt.Printf("Total: %d\n", len(jobs))
	return nil
}

// backupJobGuests renders the selection triplet as one column.
func backupJobGuests(j *backupjob.Job) string {
	switch {
	case j.All && j.Exclude != "":
		return "all -" + j.Exclude
	case j.All:
		return "all"
	case j.Pool != "":
		return "pool:" + j.Pool
	default:
		return dash(j.VMID)
	}
}

// backupJobCreateFlags registers the create/update flag set. The two commands
// share it because update accepts every mutable field create does.
//
// --enable/--disable exist instead of one --enabled bool because a cobra bool
// flag cannot distinguish "not passed" from "passed false" — and update needs
// all three states (leave unchanged / enable / disable).
func backupJobCreateFlags(fs *pflag.FlagSet) {
	fs.Bool("all", false, "Back up all guests")
	fs.String("vmid", "", "Guest IDs to back up, comma-separated (IDs only, no names)")
	fs.String("pool", "", "Back up all guests in this pool")
	fs.String("exclude", "", "Guest IDs to exclude from an --all backup")
	fs.StringArray("exclude-path", nil, "Path/glob to exclude (repeatable)")
	fs.String("schedule", "", "Calendar event, e.g. 'sat 02:30' or '*/15' (create: required)")
	fs.String("storage", "", "Target storage ID (create: required)")
	fs.Bool("enable", false, "Enable the job (default state)")
	fs.Bool("disable", false, "Disable the job")
	fs.String("mode", "", "snapshot|suspend|stop")
	fs.String("compress", "", "0|1|gzip|lzo|zstd")
	fs.String("comment", "", "Free-text comment")
	fs.String("notes-template", "", "Template for backup notes")
	fs.String("mailto", "", "Deprecated: comma-separated mail recipients")
	fs.String("mailnotification", "", "Deprecated: always|failure")
	fs.String("notification-mode", "", "auto|legacy-sendmail|notification-system")
	fs.String("prune-backups", "", "Retention property string, e.g. 'keep-last=7,keep-daily=14'")
	fs.String("performance", "", "Performance property string (requires Sys.Modify)")
	fs.String("fleecing", "", "Fleecing property string (VM only)")
	fs.String("pbs-change-detection-mode", "", "legacy|data|metadata")
	fs.Bool("protected", false, "Mark resulting backups protected")
	fs.Bool("remove", false, "Prune older backups per prune-backups")
	fs.Bool("repeat-missed", false, "Run ASAP if a scheduled run was missed")
	fs.String("node", "", "Only run when executed on this node")
	fs.Int("bwlimit", 0, "I/O bandwidth limit in KiB/s (requires Sys.Modify)")
	fs.Int("ionice", 0, "BFQ IO priority (requires Sys.Modify)")
	fs.Int("zstd", 0, "zstd threads (0 = half the cores)")
}

func backupJobFromFlags(cmd *cobra.Command) (*backupjob.Job, error) {
	flags := cmd.Flags()
	job := &backupjob.Job{}
	job.ID, _ = flags.GetString("id")
	job.VMID, _ = flags.GetString("vmid")
	job.Pool, _ = flags.GetString("pool")
	job.Exclude, _ = flags.GetString("exclude")
	job.ExcludePath, _ = flags.GetStringArray("exclude-path")
	job.Schedule, _ = flags.GetString("schedule")
	job.Storage, _ = flags.GetString("storage")
	job.Mode, _ = flags.GetString("mode")
	job.Compress, _ = flags.GetString("compress")
	job.Comment, _ = flags.GetString("comment")
	job.NotesTempl, _ = flags.GetString("notes-template")
	job.Mailto, _ = flags.GetString("mailto")
	job.MailNotification, _ = flags.GetString("mailnotification")
	job.NotificationMode, _ = flags.GetString("notification-mode")
	job.PruneBackups, _ = flags.GetString("prune-backups")
	job.Performance, _ = flags.GetString("performance")
	job.Fleecing, _ = flags.GetString("fleecing")
	job.PBSChangeDet, _ = flags.GetString("pbs-change-detection-mode")
	job.Node, _ = flags.GetString("node")
	job.BWLimit, _ = flags.GetInt("bwlimit")
	job.IOnice, _ = flags.GetInt("ionice")
	job.Zstd, _ = flags.GetInt("zstd")
	job.All, _ = flags.GetBool("all")
	job.Protected, _ = flags.GetBool("protected")
	job.Remove, _ = flags.GetBool("remove")
	job.RepeatMissed, _ = flags.GetBool("repeat-missed")

	// Exactly one of --enable/--disable, or neither (leave unchanged).
	enable, _ := flags.GetBool("enable")
	disable, _ := flags.GetBool("disable")
	if enable && disable {
		return nil, fmt.Errorf("--enable and --disable are mutually exclusive")
	}
	if enable {
		t := true
		job.Enabled = &t
	} else if disable {
		f := false
		job.Enabled = &f
	}
	return job, nil
}

func runBackupJobCreate(cmd *cobra.Command, args []string) error {
	job, err := backupJobFromFlags(cmd)
	if err != nil {
		return err
	}

	// Plan/apply segregation: show what will be created before it is sent —
	// the same gate update and delete carry.
	id := job.ID
	if id == "" {
		id = "(server-generated)"
	}
	fmt.Printf("Create backup job %s\n", id)
	fmt.Printf("  schedule: %s\n", job.Schedule)
	fmt.Printf("  storage:  %s\n", job.Storage)
	fmt.Printf("  guests:   %s\n", backupJobGuests(job))
	if job.Enabled != nil && !*job.Enabled {
		fmt.Println("  state:    disabled (job will never run until enabled)")
	}
	if job.Comment != "" {
		fmt.Printf("  comment:  %s\n", job.Comment)
	}

	if dryRun {
		printDryRunHeader()
		printDryRunAction("backup-job create", job.ID, "", "schedule="+job.Schedule+" storage="+job.Storage)
		printDryRunSummary("backup-job create", 1)
		return nil
	}
	if !confirmOperation(fmt.Sprintf("Create backup job %s?", id)) {
		fmt.Println("Operation cancelled")
		return nil
	}

	if err := backupJobOps.CreateJob(job); err != nil {
		return err
	}
	if job.ID == "" {
		id = "(server-generated — check `pve cluster backup-job list`)"
	}
	fmt.Printf("✅ Backup job created: %s\n", id)
	return nil
}

func runBackupJobUpdate(cmd *cobra.Command, args []string) error {
	job, err := backupJobFromFlags(cmd)
	if err != nil {
		return err
	}
	if job.ID == "" {
		return fmt.Errorf("--id must be specified")
	}
	deleteKeys, _ := cmd.Flags().GetStringSlice("delete")

	// Pre-fetch the current definition both for the diff and so a typo'd ID
	// fails before anything is sent.
	current, err := backupJobOps.GetJob(job.ID)
	if err != nil {
		return err
	}

	fmt.Printf("Update backup job %s\n", job.ID)
	for _, line := range backupJobDiff(current, job) {
		fmt.Printf("  %s\n", line)
	}
	if len(deleteKeys) > 0 {
		fmt.Printf("  reset to default: %s\n", strings.Join(deleteKeys, ", "))
	}

	if dryRun {
		printDryRunHeader()
		printDryRunAction("backup-job update", job.ID, "", strings.Join(deleteKeys, ","))
		printDryRunSummary("backup-job update", 1)
		return nil
	}
	if !confirmOperation(fmt.Sprintf("Update backup job %s?", job.ID)) {
		fmt.Println("Operation cancelled")
		return nil
	}

	if err := backupJobOps.UpdateJob(job.ID, job, deleteKeys); err != nil {
		return err
	}
	fmt.Printf("✅ Backup job %s updated\n", job.ID)
	return nil
}

// backupJobDiff renders before→after lines for every field the update changes.
func backupJobDiff(current, next *backupjob.Job) []string {
	var lines []string
	add := func(name, before, after string) {
		if after != "" {
			lines = append(lines, fmt.Sprintf("%s: %s → %s", name, dash(before), after))
		}
	}
	add("schedule", current.Schedule, next.Schedule)
	add("storage", current.Storage, next.Storage)
	add("vmid", current.VMID, next.VMID)
	add("pool", current.Pool, next.Pool)
	add("exclude", current.Exclude, next.Exclude)
	if len(next.ExcludePath) > 0 {
		add("exclude-path", strings.Join(current.ExcludePath, ","), strings.Join(next.ExcludePath, ","))
	}
	add("mode", current.Mode, next.Mode)
	add("compress", current.Compress, next.Compress)
	add("comment", current.Comment, next.Comment)
	add("notes-template", current.NotesTempl, next.NotesTempl)
	add("mailto", current.Mailto, next.Mailto)
	add("mailnotification", current.MailNotification, next.MailNotification)
	add("notification-mode", current.NotificationMode, next.NotificationMode)
	add("prune-backups", current.PruneBackups, next.PruneBackups)
	add("performance", current.Performance, next.Performance)
	add("fleecing", current.Fleecing, next.Fleecing)
	add("pbs-change-detection-mode", current.PBSChangeDet, next.PBSChangeDet)
	add("node", current.Node, next.Node)
	if next.BWLimit > 0 {
		add("bwlimit", fmt.Sprintf("%d", current.BWLimit), fmt.Sprintf("%d", next.BWLimit))
	}
	if next.IOnice > 0 {
		add("ionice", fmt.Sprintf("%d", current.IOnice), fmt.Sprintf("%d", next.IOnice))
	}
	if next.Zstd > 0 {
		add("zstd", fmt.Sprintf("%d", current.Zstd), fmt.Sprintf("%d", next.Zstd))
	}
	boolLine := func(name string, before, after bool) {
		lines = append(lines, fmt.Sprintf("%s: %t → %t", name, before, after))
	}
	if next.All && !current.All {
		boolLine("all", current.All, true)
	}
	if next.Protected {
		boolLine("protected", current.Protected, true)
	}
	if next.Remove {
		boolLine("remove", current.Remove, true)
	}
	if next.RepeatMissed {
		boolLine("repeat-missed", current.RepeatMissed, true)
	}
	if next.Enabled != nil {
		before := current.Enabled == nil || *current.Enabled
		if before != *next.Enabled {
			boolLine("enabled", before, *next.Enabled)
		}
	}
	if len(lines) == 0 {
		lines = append(lines, "no field changes (only key resets, if any)")
	}
	return lines
}

func runBackupJobDelete(cmd *cobra.Command, args []string) error {
	id, _ := cmd.Flags().GetString("id")
	if id == "" {
		return fmt.Errorf("--id must be specified")
	}

	// Show what is about to be removed; a failed pre-fetch (permissions,
	// typo'd ID) must not silently proceed to the prompt.
	current, err := backupJobOps.GetJob(id)
	if err != nil {
		return err
	}
	fmt.Printf("Delete backup job %s\n", id)
	fmt.Printf("  schedule: %s\n", dash(current.Schedule))
	fmt.Printf("  storage:  %s\n", dash(current.Storage))
	fmt.Printf("  guests:   %s\n", backupJobGuests(current))
	fmt.Println("  existing backup files are NOT deleted")

	if dryRun {
		printDryRunHeader()
		printDryRunAction("backup-job delete", id, "", "")
		printDryRunSummary("backup-job delete", 1)
		return nil
	}

	// Typed confirmation: the user types the exact job ID back. This command
	// deliberately does NOT call autoDetectBatchMode — it is single-target, so
	// batch auto-detection has nothing to detect and would only risk making
	// this prompt unreachable (the config-set trap).
	if !confirmExactText(fmt.Sprintf("Type '%s' to delete this backup job: ", id), id) {
		fmt.Println("Operation cancelled")
		return nil
	}

	if err := backupJobOps.DeleteJob(id); err != nil {
		return err
	}
	fmt.Printf("✅ Backup job %s deleted\n", id)
	return nil
}

func runBackupJobNotCovered(cmd *cobra.Command, args []string) error {
	guests, err := backupJobOps.NotCoveredGuests()
	if err != nil {
		return err
	}
	if len(guests) == 0 {
		fmt.Println("\nEvery guest is covered by a backup job")
		return nil
	}
	fmt.Printf("\n%-8s %-30s %s\n", "VMID", "NAME", "TYPE")
	fmt.Println(strings.Repeat("-", 46))
	for _, g := range guests {
		fmt.Printf("%-8d %-30s %s\n", g.VMID, dash(g.Name), dash(g.Type))
	}
	fmt.Println(strings.Repeat("-", 46))
	fmt.Printf("Total: %d guest(s) not covered by any backup job\n", len(guests))
	return nil
}
