package task

import (
	"fmt"
	"strings"
)

// DisplayTasks shows tasks in formatted table
func DisplayTasks(tasks []*Task) {
	if len(tasks) == 0 {
		fmt.Println("❌ No tasks found")
		return
	}

	fmt.Println("\nProxmox Tasks:")
	fmt.Println(strings.Repeat("=", 140))
	fmt.Printf("%-12s %-20s %-15s %-10s %-15s %-20s %-10s\n",
		"Node", "Type", "User", "Status", "Exit Status", "Started", "Duration")
	fmt.Println(strings.Repeat("-", 140))

	for _, task := range tasks {
		status := formatStatus(task.Status, task.ExitStatus)
		exitStatus := task.ExitStatus
		if exitStatus == "" {
			exitStatus = "-"
		}

		started := task.StartedAt.Format("2006-01-02 15:04")
		duration := formatDuration(task.Duration)

		fmt.Printf("%-12s %-20s %-15s %-10s %-15s %-20s %-10s\n",
			task.Node,
			truncate(task.Type, 20),
			truncate(task.User, 15),
			status,
			exitStatus,
			started,
			duration)
	}

	fmt.Println(strings.Repeat("-", 140))
	fmt.Printf("Total tasks: %d\n", len(tasks))
}

// DisplayTaskDetails shows detailed information for a single task
func DisplayTaskDetails(task *Task) {
	fmt.Printf("\n📋 Task Details\n")
	fmt.Println(strings.Repeat("=", 80))

	fmt.Printf("UPID:        %s\n", task.UPID)
	fmt.Printf("Node:        %s\n", task.Node)
	fmt.Printf("Type:        %s\n", task.Type)
	fmt.Printf("User:        %s\n", task.User)

	if task.ID != "" {
		fmt.Printf("ID:          %s\n", task.ID)
	}

	status := formatStatus(task.Status, task.ExitStatus)
	fmt.Printf("Status:      %s\n", status)

	if task.ExitStatus != "" {
		fmt.Printf("Exit Status: %s\n", task.ExitStatus)
	}

	if task.Progress > 0 {
		fmt.Printf("Progress:    %.0f%%\n", task.Progress*100)
	}

	fmt.Printf("Started:     %s\n", task.StartedAt.Format("2006-01-02 15:04:05"))

	if task.Status == TaskStatusStopped {
		fmt.Printf("Ended:       %s\n", task.EndedAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("Duration:    %s\n", formatDuration(task.Duration))
	}

	fmt.Println(strings.Repeat("=", 80))
}

// DisplayTaskLog shows task log output
func DisplayTaskLog(nodeName, upid string, logs []*TaskLog, tail int) {
	if len(logs) == 0 {
		fmt.Printf("❌ No log entries found for task %s\n", upid)
		return
	}

	// If tail is specified, show only last N lines
	startIdx := 0
	if tail > 0 && len(logs) > tail {
		startIdx = len(logs) - tail
	}

	fmt.Printf("\n📜 Task Log: %s@%s\n", truncate(upid, 40), nodeName)
	if tail > 0 {
		fmt.Printf("(showing last %d lines)\n", tail)
	}
	fmt.Println(strings.Repeat("=", 100))

	for i := startIdx; i < len(logs); i++ {
		log := logs[i]
		fmt.Printf("%4d: %s\n", log.LineNumber, log.Text)
	}

	fmt.Println(strings.Repeat("-", 100))
	fmt.Printf("Total log lines: %d\n", len(logs))
}

// DisplayRunningTasks shows currently running tasks
func DisplayRunningTasks(tasks []*Task) {
	if len(tasks) == 0 {
		fmt.Println("✅ No running tasks")
		return
	}

	fmt.Println("\n🔄 Running Tasks:")
	fmt.Println(strings.Repeat("=", 120))
	fmt.Printf("%-12s %-20s %-15s %-20s %-10s\n",
		"Node", "Type", "User", "Started", "Progress")
	fmt.Println(strings.Repeat("-", 120))

	for _, task := range tasks {
		started := task.StartedAt.Format("2006-01-02 15:04")
		progress := "-"
		if task.Progress > 0 {
			progress = fmt.Sprintf("%.0f%%", task.Progress*100)
		}

		fmt.Printf("%-12s %-20s %-15s %-20s %-10s\n",
			task.Node,
			truncate(task.Type, 20),
			truncate(task.User, 15),
			started,
			progress)
	}

	fmt.Println(strings.Repeat("-", 120))
	fmt.Printf("Total running: %d\n", len(tasks))
}

// DisplayFailedTasks shows failed tasks
func DisplayFailedTasks(tasks []*Task) {
	if len(tasks) == 0 {
		fmt.Println("✅ No failed tasks")
		return
	}

	fmt.Println("\n❌ Failed Tasks:")
	fmt.Println(strings.Repeat("=", 120))
	fmt.Printf("%-12s %-20s %-15s %-15s %-20s\n",
		"Node", "Type", "User", "Exit Status", "Started")
	fmt.Println(strings.Repeat("-", 120))

	for _, task := range tasks {
		started := task.StartedAt.Format("2006-01-02 15:04")
		exitStatus := task.ExitStatus
		if exitStatus == "" {
			exitStatus = "unknown"
		}

		fmt.Printf("%-12s %-20s %-15s %-15s %-20s\n",
			task.Node,
			truncate(task.Type, 20),
			truncate(task.User, 15),
			exitStatus,
			started)
	}

	fmt.Println(strings.Repeat("-", 120))
	fmt.Printf("Total failed: %d\n", len(tasks))
}

// Helper functions

func formatStatus(status, exitStatus string) string {
	if status == TaskStatusRunning {
		return "🔄 running"
	}

	if exitStatus == ExitStatusOK || exitStatus == "" {
		return "✅ OK"
	}

	return "❌ " + exitStatus
}

func formatDuration(d interface{}) string {
	switch v := d.(type) {
	case string:
		return v
	case int64:
		if v == 0 {
			return "-"
		}
		duration := fmt.Sprintf("%ds", v)
		if v >= 3600 {
			hours := v / 3600
			minutes := (v % 3600) / 60
			duration = fmt.Sprintf("%dh %dm", hours, minutes)
		} else if v >= 60 {
			minutes := v / 60
			seconds := v % 60
			duration = fmt.Sprintf("%dm %ds", minutes, seconds)
		}
		return duration
	default:
		// For time.Duration type
		if dur, ok := d.(interface{ String() string }); ok {
			durStr := dur.String()
			if durStr == "0s" {
				return "-"
			}
			return durStr
		}
		return "-"
	}
}

func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max-3] + "..."
	}
	return s
}
