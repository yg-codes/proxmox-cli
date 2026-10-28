package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/yg-codes/proxmox-cli/pkg/replication"
)

var replicationOps *replication.Operations

// zfsReplicationCmd groups replication job management.
//
// Replication lives under `pve zfs` because zfspool is the only
// replication-capable storage plugin: the engine is zfs send/recv between
// nodes. It is the one dataset-level ZFS mechanism the REST API exposes.
var zfsReplicationCmd = &cobra.Command{
	Use:     "replication",
	Aliases: []string{"repl"},
	Short:   "Manage ZFS-backed storage replication jobs",
	Long: `Manage Proxmox storage replication jobs.

Replication periodically zfs-sends a guest's disks to another node, so a
failover target holds a recent copy. It works only on zfspool storage — that is
a Proxmox limitation, not a CLI one.

Job IDs have the form <vmid>-<number>, e.g. 100-0. The number lets one guest
replicate to several targets. The VMID part must be at least three digits, which
is the API's own pattern.

Examples:
  pve zfs replication list
  pve zfs replication status --node pve1
  pve zfs replication create --id 100-0 --target pve2 --schedule '*/15'
  pve zfs replication run --node pve1 --id 100-0
  pve zfs replication delete --id 100-0`,
}

var zfsReplListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured replication jobs",
	RunE: func(cmd *cobra.Command, args []string) error {
		jobs, err := replicationOps.ListJobs()
		if err != nil {
			return err
		}
		if len(jobs) == 0 {
			fmt.Println("\nNo replication jobs configured")
			fmt.Println("  Create one with: pve zfs replication create --id <vmid>-0 --target <node>")
			return nil
		}
		fmt.Printf("\n%-12s %-10s %-10s %-14s %-8s %s\n",
			"JOB", "SOURCE", "TARGET", "SCHEDULE", "STATE", "COMMENT")
		fmt.Println(strings.Repeat("-", 82))
		for _, j := range jobs {
			state := "enabled"
			if j.Disable {
				state = "disabled"
			}
			fmt.Printf("%-12s %-10s %-10s %-14s %-8s %s\n",
				j.ID, dash(j.Source), dash(j.Target), dash(j.Schedule), state, j.Comment)
		}
		fmt.Println(strings.Repeat("-", 82))
		fmt.Printf("Total: %d\n", len(jobs))
		return nil
	},
}

var zfsReplStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show replication runtime state on a node",
	Long: `Show each job's last sync, next sync, duration and failure count.

fail_count is cumulative, not current — a non-zero value does not mean the job
is failing now. Compare last_sync against the schedule to judge that.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		node, _ := cmd.Flags().GetString("node")
		if node == "" {
			return fmt.Errorf("--node must be specified")
		}
		states, err := replicationOps.GetStatus(node)
		if err != nil {
			return err
		}
		if len(states) == 0 {
			fmt.Printf("\nNo replication jobs on %s\n", node)
			return nil
		}
		fmt.Printf("\nReplication status on %s:\n", node)
		fmt.Println(strings.Repeat("-", 92))
		fmt.Printf("%-12s %-8s %-20s %-20s %-9s %s\n",
			"JOB", "TARGET", "LAST SYNC", "NEXT SYNC", "FAILS", "STATE")
		fmt.Println(strings.Repeat("-", 92))
		for _, s := range states {
			state := "idle"
			if s.IsRunning {
				state = fmt.Sprintf("running (pid %d)", s.PID)
			}
			if s.Error != "" {
				state = "ERROR: " + s.Error
			}
			fmt.Printf("%-12s %-8s %-20s %-20s %-9d %s\n",
				s.ID, dash(s.Target), epoch(s.LastSync), epoch(s.NextSync), s.FailCount, state)
		}
		fmt.Println(strings.Repeat("-", 92))
		return nil
	},
}

var zfsReplCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a replication job",
	RunE:  runReplCreate,
}

var zfsReplUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Change a replication job",
	RunE:  runReplUpdate,
}

var zfsReplDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Remove a replication job",
	Long: `Mark a replication job for removal.

By default the replicated data on the target is also removed. --keep leaves it
in place. --force is needed when the target is unreachable, which otherwise
blocks the removal.`,
	RunE: runReplDelete,
}

var zfsReplRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Trigger a replication job now, outside its schedule",
	RunE:  runReplNow,
}

func initZFSReplicationCommands() {
	zfsCmd.AddCommand(zfsReplicationCmd)
	zfsReplicationCmd.AddCommand(zfsReplListCmd, zfsReplStatusCmd,
		zfsReplCreateCmd, zfsReplUpdateCmd, zfsReplDeleteCmd, zfsReplRunCmd)

	zfsReplStatusCmd.Flags().String("node", "", "Node to report on (required)")

	for _, c := range []*cobra.Command{zfsReplCreateCmd, zfsReplUpdateCmd} {
		c.Flags().String("id", "", "Job ID as <vmid>-<number>, e.g. 100-0 (required)")
		c.Flags().String("target", "", "Target node")
		c.Flags().String("schedule", "", "Calendar event, e.g. '*/15' or '02:00' (default */15)")
		c.Flags().Float64("rate", 0, "Bandwidth limit in MB/s")
		c.Flags().String("comment", "", "Free-text comment")
		c.Flags().Bool("disable", false, "Create/leave the job disabled")
	}
	zfsReplUpdateCmd.Flags().StringSlice("delete", nil, "Settings to reset to their default")

	zfsReplDeleteCmd.Flags().String("id", "", "Job ID (required)")
	zfsReplDeleteCmd.Flags().Bool("keep", false, "Keep the replicated data on the target")
	zfsReplDeleteCmd.Flags().Bool("force", false, "Proceed even if the target is unreachable")

	zfsReplRunCmd.Flags().String("node", "", "Node the job runs from (required)")
	zfsReplRunCmd.Flags().String("id", "", "Job ID (required)")
}

func replJobFromFlags(cmd *cobra.Command) *replication.Job {
	job := &replication.Job{}
	job.ID, _ = cmd.Flags().GetString("id")
	job.Target, _ = cmd.Flags().GetString("target")
	job.Schedule, _ = cmd.Flags().GetString("schedule")
	job.Comment, _ = cmd.Flags().GetString("comment")
	job.Rate, _ = cmd.Flags().GetFloat64("rate")
	job.Disable, _ = cmd.Flags().GetBool("disable")
	return job
}

func runReplCreate(cmd *cobra.Command, args []string) error {
	job := replJobFromFlags(cmd)
	if err := replication.ValidateJobID(job.ID); err != nil {
		return err
	}
	if job.Target == "" {
		return fmt.Errorf("--target must be specified")
	}

	fmt.Printf("Create replication job %s → %s\n", job.ID, job.Target)
	if job.Schedule != "" {
		fmt.Printf("  schedule: %s\n", job.Schedule)
	} else {
		fmt.Printf("  schedule: */15 (API default)\n")
	}
	if job.Rate > 0 {
		fmt.Printf("  rate:     %g MB/s\n", job.Rate)
	}
	fmt.Println()

	if dryRun {
		printDryRunHeader()
		printDryRunAction("replication create", job.ID, "", "target "+job.Target)
		printDryRunSummary("replication create", 1)
		return nil
	}
	if !confirmOperation(fmt.Sprintf("Create replication job %s?", job.ID)) {
		fmt.Println("Operation cancelled")
		return nil
	}

	if err := replicationOps.CreateJob(job); err != nil {
		return err
	}
	fmt.Printf("✅ Replication job %s created\n", job.ID)
	return nil
}

func runReplUpdate(cmd *cobra.Command, args []string) error {
	job := replJobFromFlags(cmd)
	if job.ID == "" {
		return fmt.Errorf("--id must be specified")
	}
	deleteKeys, _ := cmd.Flags().GetStringSlice("delete")

	fmt.Printf("Update replication job %s\n", job.ID)
	if dryRun {
		printDryRunHeader()
		printDryRunAction("replication update", job.ID, "", strings.Join(deleteKeys, ","))
		printDryRunSummary("replication update", 1)
		return nil
	}
	if !confirmOperation(fmt.Sprintf("Update replication job %s?", job.ID)) {
		fmt.Println("Operation cancelled")
		return nil
	}

	if err := replicationOps.UpdateJob(job, deleteKeys); err != nil {
		return err
	}
	fmt.Printf("✅ Replication job %s updated\n", job.ID)
	return nil
}

func runReplDelete(cmd *cobra.Command, args []string) error {
	id, _ := cmd.Flags().GetString("id")
	if id == "" {
		return fmt.Errorf("--id must be specified")
	}
	keep, _ := cmd.Flags().GetBool("keep")
	force, _ := cmd.Flags().GetBool("force")

	fmt.Printf("Remove replication job %s\n", id)
	if keep {
		fmt.Println("  --keep: replicated data on the target is preserved")
	} else {
		fmt.Println("  replicated data on the target is also removed (--keep to preserve it)")
	}
	fmt.Println()

	if dryRun {
		printDryRunHeader()
		printDryRunAction("replication delete", id, "", fmt.Sprintf("keep=%v force=%v", keep, force))
		printDryRunSummary("replication delete", 1)
		return nil
	}
	if !confirmOperation(fmt.Sprintf("Remove replication job %s?", id)) {
		fmt.Println("Operation cancelled")
		return nil
	}

	if err := replicationOps.DeleteJob(id, keep, force); err != nil {
		return err
	}
	fmt.Printf("✅ Replication job %s marked for removal\n", id)
	return nil
}

func runReplNow(cmd *cobra.Command, args []string) error {
	node, _ := cmd.Flags().GetString("node")
	id, _ := cmd.Flags().GetString("id")
	if node == "" || id == "" {
		return fmt.Errorf("both --node and --id are required")
	}

	if dryRun {
		printDryRunHeader()
		printDryRunAction("replication run", id, "", "on node "+node)
		printDryRunSummary("replication run", 1)
		return nil
	}

	upid, err := replicationOps.ScheduleNow(node, id)
	if err != nil {
		return err
	}
	fmt.Printf("✅ Replication job %s triggered on %s (task %s)\n", id, node, upid)
	return nil
}

// epoch renders a unix timestamp, or "-" when unset.
func epoch(v float64) string {
	if v <= 0 {
		return "-"
	}
	return time.Unix(int64(v), 0).Format("2006-01-02 15:04:05")
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
