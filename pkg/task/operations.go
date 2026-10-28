package task

import (
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/yg-codes/proxmox-cli/pkg/api"
)

// Operations handles task operations
type Operations struct {
	client *api.Client
	logger *logrus.Logger
}

// NewOperations creates a new task operations instance
func NewOperations(client *api.Client, logger *logrus.Logger) *Operations {
	if logger == nil {
		logger = logrus.New()
	}

	return &Operations{
		client: client,
		logger: logger,
	}
}

// GetTasks lists all cluster tasks with optional filtering
// API: GET /cluster/tasks
func (ops *Operations) GetTasks(filter *TaskFilter) ([]*Task, error) {
	ops.logger.Debug("Fetching cluster tasks")

	params := make(map[string]string)
	if filter != nil {
		if filter.Source != "" {
			params["source"] = filter.Source
		}
		if filter.TypeID != "" {
			params["typefilter"] = filter.TypeID
		}
		if filter.User != "" {
			params["userfilter"] = filter.User
		}
	}

	resp, err := ops.client.Get("/cluster/tasks", params)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster tasks: %w", err)
	}

	var tasks []*Task
	if items, ok := resp["items"].([]interface{}); ok {
		for _, item := range items {
			if taskMap, ok := item.(map[string]interface{}); ok {
				task := parseTask(taskMap)
				tasks = append(tasks, task)
			}
		}
	}

	// Apply client-side filtering
	if filter != nil {
		tasks = applyClientFilter(tasks, filter)
	}

	ops.logger.Infof("Found %d tasks", len(tasks))
	return tasks, nil
}

// GetNodeTasks lists tasks for a specific node
// API: GET /nodes/{node}/tasks
func (ops *Operations) GetNodeTasks(nodeName string, filter *TaskFilter) ([]*Task, error) {
	ops.logger.Debugf("Fetching tasks for node: %s", nodeName)

	params := make(map[string]string)

	path := fmt.Sprintf("/nodes/%s/tasks", nodeName)
	resp, err := ops.client.Get(path, params)
	if err != nil {
		return nil, fmt.Errorf("failed to get tasks for node %s: %w", nodeName, err)
	}

	var tasks []*Task
	if items, ok := resp["items"].([]interface{}); ok {
		for _, item := range items {
			if taskMap, ok := item.(map[string]interface{}); ok {
				task := parseTask(taskMap)
				tasks = append(tasks, task)
			}
		}
	}

	// Apply client-side filtering
	if filter != nil {
		tasks = applyClientFilter(tasks, filter)
	}

	ops.logger.Infof("Found %d tasks on node %s", len(tasks), nodeName)
	return tasks, nil
}

// GetTaskStatus gets status of a specific task
// API: GET /nodes/{node}/tasks/{upid}/status
func (ops *Operations) GetTaskStatus(nodeName, upid string) (*Task, error) {
	ops.logger.Debugf("Fetching task status: %s on node %s", upid, nodeName)

	path := fmt.Sprintf("/nodes/%s/tasks/%s/status", nodeName, upid)
	resp, err := ops.client.Get(path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get task status: %w", err)
	}

	task := parseTask(resp)
	task.UPID = upid
	task.Node = nodeName

	return task, nil
}

// GetTaskLog gets log output from a task
// API: GET /nodes/{node}/tasks/{upid}/log
func (ops *Operations) GetTaskLog(nodeName, upid string, start, limit int) ([]*TaskLog, error) {
	ops.logger.Debugf("Fetching task log: %s on node %s", upid, nodeName)

	params := make(map[string]string)
	if start > 0 {
		params["start"] = fmt.Sprintf("%d", start)
	}
	if limit > 0 {
		params["limit"] = fmt.Sprintf("%d", limit)
	}

	path := fmt.Sprintf("/nodes/%s/tasks/%s/log", nodeName, upid)
	resp, err := ops.client.Get(path, params)
	if err != nil {
		return nil, fmt.Errorf("failed to get task log: %w", err)
	}

	var logs []*TaskLog
	if items, ok := resp["items"].([]interface{}); ok {
		for _, item := range items {
			if logMap, ok := item.(map[string]interface{}); ok {
				log := &TaskLog{}
				if n, ok := logMap["n"].(float64); ok {
					log.LineNumber = int(n)
				}
				if t, ok := logMap["t"].(string); ok {
					log.Text = t
				}
				logs = append(logs, log)
			}
		}
	}

	ops.logger.Infof("Retrieved %d log lines for task %s", len(logs), upid)
	return logs, nil
}

// StopTask stops a running task
// API: DELETE /nodes/{node}/tasks/{upid}
func (ops *Operations) StopTask(nodeName, upid string) error {
	ops.logger.Infof("Stopping task %s on node %s", upid, nodeName)

	path := fmt.Sprintf("/nodes/%s/tasks/%s", nodeName, upid)
	_, err := ops.client.Delete(path)
	if err != nil {
		return fmt.Errorf("failed to stop task %s: %w", upid, err)
	}

	ops.logger.Infof("✅ Stopped task %s", upid)
	return nil
}

// GetRunningTasks gets all currently running tasks
func (ops *Operations) GetRunningTasks() ([]*Task, error) {
	filter := &TaskFilter{
		Running: true,
		Limit:   100,
	}
	return ops.GetTasks(filter)
}

// GetFailedTasks gets all failed tasks
func (ops *Operations) GetFailedTasks() ([]*Task, error) {
	filter := &TaskFilter{
		Errors: true,
		Limit:  100,
	}
	return ops.GetTasks(filter)
}

// Helper function to parse task data
func parseTask(taskMap map[string]interface{}) *Task {
	task := &Task{}

	if upid, ok := taskMap["upid"].(string); ok {
		task.UPID = upid
	}
	if node, ok := taskMap["node"].(string); ok {
		task.Node = node
	}
	if pid, ok := taskMap["pid"].(float64); ok {
		task.PID = int(pid)
	}
	if pstart, ok := taskMap["pstart"].(float64); ok {
		task.PStart = int64(pstart)
		task.StartedAt = time.Unix(int64(pstart), 0)
	}
	if taskType, ok := taskMap["type"].(string); ok {
		task.Type = taskType
	}
	if id, ok := taskMap["id"].(string); ok {
		task.ID = id
	}
	if user, ok := taskMap["user"].(string); ok {
		task.User = user
	}
	if status, ok := taskMap["status"].(string); ok {
		task.Status = status
	}
	if exitStatus, ok := taskMap["exitstatus"].(string); ok {
		task.ExitStatus = exitStatus
	}
	if progress, ok := taskMap["progress"].(float64); ok {
		task.Progress = progress
	}
	if saved, ok := taskMap["saved"].(string); ok {
		task.Saved = saved
	}

	// Calculate duration if task is stopped
	if task.Status == TaskStatusStopped {
		if endtime, ok := taskMap["endtime"].(float64); ok {
			task.EndedAt = time.Unix(int64(endtime), 0)
			task.Duration = task.EndedAt.Sub(task.StartedAt)
		}
	}

	return task
}

// applyClientFilter applies client-side filtering to tasks
func applyClientFilter(tasks []*Task, filter *TaskFilter) []*Task {
	var filtered []*Task

	for _, task := range tasks {
		// Filter by running status
		if filter.Running && task.Status != TaskStatusRunning {
			continue
		}

		// Filter by error status
		if filter.Errors && (task.ExitStatus == ExitStatusOK || task.ExitStatus == "") {
			continue
		}

		// Apply limit
		if filter.Limit > 0 && len(filtered) >= filter.Limit {
			break
		}

		filtered = append(filtered, task)
	}

	return filtered
}
