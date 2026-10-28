package main

import (
	"fmt"
	"strings"

	"time"

	"github.com/spf13/cobra"
	"github.com/yg-codes/proxmox-cli/pkg/task"
)

var (
	// Task-specific flags
	taskNodeFlag    string
	taskRunningFlag bool
	taskErrorsFlag  bool
	taskTypeFlag    string
	taskUserFlag    string
	taskLimitFlag   int
)

var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "Task management and monitoring",
	Long:  "List, monitor, and manage Proxmox tasks including long-running operations",
}

var taskListCmd = &cobra.Command{
	Use:   "list",
	Short: "List tasks",
	Long:  "List Proxmox tasks with optional filtering by node, status, type, or user",
	RunE: func(cmd *cobra.Command, args []string) error {
		filter := &task.TaskFilter{
			Node:    taskNodeFlag,
			Running: taskRunningFlag,
			Errors:  taskErrorsFlag,
			TypeID:  taskTypeFlag,
			User:    taskUserFlag,
			Limit:   taskLimitFlag,
		}

		var tasks []*task.Task
		var err error

		if taskNodeFlag != "" {
			tasks, err = taskOps.GetNodeTasks(taskNodeFlag, filter)
		} else {
			tasks, err = taskOps.GetTasks(filter)
		}

		if err != nil {
			return fmt.Errorf("failed to list tasks: %w", err)
		}

		task.DisplayTasks(tasks)
		return nil
	},
}

var taskRunningCmd = &cobra.Command{
	Use:   "running",
	Short: "List running tasks",
	Long:  "Show all currently running tasks across the cluster",
	RunE: func(cmd *cobra.Command, args []string) error {
		tasks, err := taskOps.GetRunningTasks()
		if err != nil {
			return fmt.Errorf("failed to get running tasks: %w", err)
		}

		task.DisplayRunningTasks(tasks)
		return nil
	},
}

var taskFailedCmd = &cobra.Command{
	Use:   "failed",
	Short: "List failed tasks",
	Long:  "Show all failed tasks across the cluster",
	RunE: func(cmd *cobra.Command, args []string) error {
		tasks, err := taskOps.GetFailedTasks()
		if err != nil {
			return fmt.Errorf("failed to get failed tasks: %w", err)
		}

		task.DisplayFailedTasks(tasks)
		return nil
	},
}

var taskStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Get task status",
	Long:  "Display detailed status information for a specific task",
	RunE: func(cmd *cobra.Command, args []string) error {
		nodeName, _ := cmd.Flags().GetString("node")
		upid, _ := cmd.Flags().GetString("upid")

		if nodeName == "" || upid == "" {
			return fmt.Errorf("both --node and --upid flags are required")
		}

		taskStatus, err := taskOps.GetTaskStatus(nodeName, upid)
		if err != nil {
			return fmt.Errorf("failed to get task status: %w", err)
		}

		task.DisplayTaskDetails(taskStatus)
		return nil
	},
}

var taskLogCmd = &cobra.Command{
	Use:   "log",
	Short: "Get task log output",
	Long:  "Display log output from a specific task",
	RunE: func(cmd *cobra.Command, args []string) error {
		nodeName, _ := cmd.Flags().GetString("node")
		upid, _ := cmd.Flags().GetString("upid")
		tail, _ := cmd.Flags().GetInt("tail")
		follow, _ := cmd.Flags().GetBool("follow")

		if nodeName == "" || upid == "" {
			return fmt.Errorf("both --node and --upid flags are required")
		}

		if follow {
			return followTaskLog(nodeName, upid, tail)
		}

		logs, err := taskOps.GetTaskLog(nodeName, upid, 0, 0)
		if err != nil {
			return fmt.Errorf("failed to get task log: %w", err)
		}

		task.DisplayTaskLog(nodeName, upid, logs, tail)
		return nil
	},
}

var taskStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop a running task",
	Long:  "Stop a specific running task (requires confirmation)",
	RunE: func(cmd *cobra.Command, args []string) error {
		nodeName, _ := cmd.Flags().GetString("node")
		upid, _ := cmd.Flags().GetString("upid")

		if nodeName == "" || upid == "" {
			return fmt.Errorf("both --node and --upid flags are required")
		}

		if dryRun {
			fmt.Printf("🔍 DRY-RUN: Would stop task %s on node %s\n", upid, nodeName)
			return nil
		}

		if !autoConfirm && !batchMode {
			fmt.Printf("⚠️  Are you sure you want to stop task %s on node %s? (yes/no): ", upid, nodeName)
			if !confirmAction() {
				fmt.Println("❌ Operation cancelled")
				return nil
			}
		}

		err := taskOps.StopTask(nodeName, upid)
		if err != nil {
			return err
		}

		fmt.Printf("✅ Task %s stopped on node %s\n", upid, nodeName)
		return nil
	},
}

func initTaskCommands() {
	// Note: taskCmd is now added by cmd_cluster.go
	// rootCmd.AddCommand(taskCmd)

	taskCmd.AddCommand(taskListCmd)
	taskCmd.AddCommand(taskRunningCmd)
	taskCmd.AddCommand(taskFailedCmd)
	taskCmd.AddCommand(taskStatusCmd)
	taskCmd.AddCommand(taskLogCmd)
	taskCmd.AddCommand(taskStopCmd)

	// Flags for list command
	taskListCmd.Flags().StringVar(&taskNodeFlag, "node", "", "Filter by node")
	taskListCmd.Flags().BoolVar(&taskRunningFlag, "running", false, "Show only running tasks")
	taskListCmd.Flags().BoolVar(&taskErrorsFlag, "errors", false, "Show only failed tasks")
	taskListCmd.Flags().StringVar(&taskTypeFlag, "type", "", "Filter by task type")
	taskListCmd.Flags().StringVar(&taskUserFlag, "user", "", "Filter by user")
	taskListCmd.Flags().IntVar(&taskLimitFlag, "limit", 50, "Maximum number of tasks")

	// Flags for status command
	taskStatusCmd.Flags().String("node", "", "Node name (required)")
	taskStatusCmd.Flags().String("upid", "", "Task UPID (required)")
	_ = taskStatusCmd.MarkFlagRequired("node")
	_ = taskStatusCmd.MarkFlagRequired("upid")

	// Flags for log command
	taskLogCmd.Flags().String("node", "", "Node name (required)")
	taskLogCmd.Flags().String("upid", "", "Task UPID (required)")
	taskLogCmd.Flags().Int("tail", 100, "Number of lines to show from end")
	taskLogCmd.Flags().Bool("follow", false, "Follow log output (like tail -f)")
	_ = taskLogCmd.MarkFlagRequired("node")
	_ = taskLogCmd.MarkFlagRequired("upid")

	// Flags for stop command
	taskStopCmd.Flags().String("node", "", "Node name (required)")
	taskStopCmd.Flags().String("upid", "", "Task UPID (required)")
	_ = taskStopCmd.MarkFlagRequired("node")
	_ = taskStopCmd.MarkFlagRequired("upid")
}

// Helper function to follow task log (like tail -f)
func followTaskLog(nodeName, upid string, tail int) error {
	fmt.Printf("🔄 Following task log: %s@%s (Ctrl+C to stop)\n", upid, nodeName)
	fmt.Println(strings.Repeat("=", 100))

	lastLineNum := 0

	for {
		logs, err := taskOps.GetTaskLog(nodeName, upid, lastLineNum, 0)
		if err != nil {
			return fmt.Errorf("failed to get task log: %w", err)
		}

		// Print new lines
		for _, log := range logs {
			if log.LineNumber > lastLineNum {
				fmt.Printf("%4d: %s\n", log.LineNumber, log.Text)
				lastLineNum = log.LineNumber
			}
		}

		// Check if task is still running
		taskStatus, err := taskOps.GetTaskStatus(nodeName, upid)
		if err != nil {
			return err
		}

		if taskStatus.Status != task.TaskStatusRunning {
			fmt.Println(strings.Repeat("-", 100))
			fmt.Printf("✅ Task completed with status: %s\n", taskStatus.ExitStatus)
			break
		}

		// Wait before next poll. The loop exits above once the task leaves the
		// running state, and Ctrl+C (SIGINT) terminates it in any mode.
		time.Sleep(2 * time.Second)
	}

	return nil
}
