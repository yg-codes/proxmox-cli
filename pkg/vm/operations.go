package vm

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/yg-codes/proxmox-cli/pkg/api"
)

// VM represents a Proxmox virtual machine
type VM struct {
	VMID   string `json:"vmid"`
	Name   string `json:"name"`
	Node   string `json:"node"`
	Status string `json:"status"`
	// QMPStatus is the QEMU-level run state. It matters because a paused VM
	// still reports Status "running" — only qmpstatus reports "paused", so it
	// is the only reliable way to tell a suspended VM from a live one.
	QMPStatus string `json:"qmpstatus"`
	Running   bool   `json:"running"`
	Paused    bool   `json:"paused"`
	Memory    int64  `json:"memory"`
	CPUs      int    `json:"cpus"`
	DiskSize  int64  `json:"disksize"`
}

// Node represents a Proxmox node
type Node struct {
	Name   string `json:"node"`
	Status string `json:"status"`
	Online bool   `json:"online"`
}

// TaskStatus represents the status of a Proxmox task
type TaskStatus struct {
	UPID     string  `json:"upid"`
	Type     string  `json:"type"`
	Status   string  `json:"status"`
	ExitCode string  `json:"exitstatus"`
	Progress float64 `json:"progress"`
}

// Operations handles VM operations
type Operations struct {
	client    *api.Client
	logger    *logrus.Logger
	nodeCache map[string]*Node
	// cacheMu guards nodeCache: bulk workers call GetNodes concurrently, and
	// an unsynchronized map write there is a data race (and a potential panic)
	// under any concurrent bulk operation.
	cacheMu sync.Mutex
	// taskTimeout bounds how long MonitorTask polls one task. Zero until
	// SetTaskTimeout runs, which also enforces the default.
	taskTimeout time.Duration
}

// NewOperations creates a new VM operations instance
func NewOperations(client *api.Client, logger *logrus.Logger) *Operations {
	if logger == nil {
		logger = logrus.New()
	}

	return &Operations{
		client:    client,
		logger:    logger,
		nodeCache: make(map[string]*Node),
	}
}

// SetTaskTimeout bounds how long MonitorTask polls a single task before
// giving up. Zero or negative restores the five-minute default.
func (ops *Operations) SetTaskTimeout(d time.Duration) {
	if d <= 0 {
		d = 5 * time.Minute
	}
	ops.taskTimeout = d
}

// GetNodes retrieves all nodes in the cluster
func (ops *Operations) GetNodes() ([]*Node, error) {
	ops.cacheMu.Lock()
	defer ops.cacheMu.Unlock()

	if len(ops.nodeCache) > 0 {
		nodes := make([]*Node, 0, len(ops.nodeCache))
		for _, node := range ops.nodeCache {
			nodes = append(nodes, node)
		}
		return nodes, nil
	}

	resp, err := ops.client.Get("/nodes", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get nodes: %w", err)
	}

	var nodes []*Node
	if items, ok := resp["items"].([]interface{}); ok {
		for _, item := range items {
			if nodeMap, ok := item.(map[string]interface{}); ok {
				node := &Node{}
				if name, ok := nodeMap["node"].(string); ok {
					node.Name = name
				}
				if status, ok := nodeMap["status"].(string); ok {
					node.Status = status
					node.Online = status == "online"
				}
				nodes = append(nodes, node)
				ops.nodeCache[node.Name] = node
			}
		}
	}

	return nodes, nil
}

// FindVMNode finds which node a VM is located on
func (ops *Operations) FindVMNode(vmid string) (string, error) {
	nodes, err := ops.GetNodes()
	if err != nil {
		return "", err
	}

	for _, node := range nodes {
		// Try to get VM status on this node
		path := fmt.Sprintf("/nodes/%s/qemu/%s/status/current", node.Name, vmid)
		_, err := ops.client.Get(path, nil)
		if err == nil {
			return node.Name, nil
		}
	}

	return "", fmt.Errorf("VM %s not found on any node", vmid)
}

// GetAllVMs retrieves all VMs from all nodes. It queries every node even
// when some fail — the partial slice documents what was readable — but the
// aggregate error names each failed node, and callers must abort before any
// mutation when it is non-nil: a partial inventory must never be presented as
// the complete cluster.
func (ops *Operations) GetAllVMs() ([]*VM, error) {
	nodes, err := ops.GetNodes()
	if err != nil {
		return nil, err
	}

	var allVMs []*VM
	var failures []error
	for _, node := range nodes {
		vms, err := ops.getVMsFromNode(node.Name)
		if err != nil {
			ops.logger.Errorf("Failed to get VMs from node %s: %v", node.Name, err)
			failures = append(failures, fmt.Errorf("node %s: %w", node.Name, err))
			continue
		}
		allVMs = append(allVMs, vms...)
	}

	if len(failures) > 0 {
		return allVMs, fmt.Errorf("cluster inventory incomplete — %d node(s) failed: %w", len(failures), errors.Join(failures...))
	}
	return allVMs, nil
}

// getVMsFromNode retrieves VMs from a specific node
func (ops *Operations) getVMsFromNode(nodeName string) ([]*VM, error) {
	path := fmt.Sprintf("/nodes/%s/qemu", nodeName)
	resp, err := ops.client.Get(path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get VMs from node %s: %w", nodeName, err)
	}

	var vms []*VM
	if items, ok := resp["items"].([]interface{}); ok {
		for _, item := range items {
			if vmMap, ok := item.(map[string]interface{}); ok {
				vm := &VM{Node: nodeName}

				if vmid, ok := vmMap["vmid"].(float64); ok {
					vm.VMID = fmt.Sprintf("%.0f", vmid)
				}
				if name, ok := vmMap["name"].(string); ok {
					vm.Name = name
				}
				if status, ok := vmMap["status"].(string); ok {
					vm.Status = status
					vm.Running = status == "running"
				}
				if memory, ok := vmMap["memory"].(float64); ok {
					vm.Memory = int64(memory)
				}
				if cpus, ok := vmMap["cpus"].(float64); ok {
					vm.CPUs = int(cpus)
				}
				if disksize, ok := vmMap["disksize"].(float64); ok {
					vm.DiskSize = int64(disksize)
				}

				vms = append(vms, vm)
			}
		}
	}

	return vms, nil
}

// GetVMStatus retrieves the current status of a VM
func (ops *Operations) GetVMStatus(vmid string) (*VM, error) {
	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("/nodes/%s/qemu/%s/status/current", node, vmid)
	resp, err := ops.client.Get(path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get VM %s status: %w", vmid, err)
	}

	vm := &VM{
		VMID: vmid,
		Node: node,
	}

	if name, ok := resp["name"].(string); ok {
		vm.Name = name
	}
	if status, ok := resp["status"].(string); ok {
		vm.Status = status
		vm.Running = status == "running"
	}
	// A paused VM still reports status "running"; only qmpstatus distinguishes
	// it, so derive Paused from qmpstatus and correct Running accordingly.
	if qmpStatus, ok := resp["qmpstatus"].(string); ok {
		vm.QMPStatus = qmpStatus
		vm.Paused = qmpStatus == "paused"
		if vm.Paused {
			vm.Running = false
		}
	}
	if memory, ok := resp["memory"].(float64); ok {
		vm.Memory = int64(memory)
	}
	if cpus, ok := resp["cpus"].(float64); ok {
		vm.CPUs = int(cpus)
	}

	return vm, nil
}

// StartVM starts a virtual machine
func (ops *Operations) StartVM(vmid string) error {
	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/nodes/%s/qemu/%s/status/start", node, vmid)
	_, err = ops.client.Post(path, nil)
	if err != nil {
		return fmt.Errorf("failed to start VM %s: %w", vmid, err)
	}

	ops.logger.Infof("Started VM %s", vmid)
	return nil
}

// StopVM stops a virtual machine
func (ops *Operations) StopVM(vmid string) error {
	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/nodes/%s/qemu/%s/status/stop", node, vmid)
	_, err = ops.client.Post(path, nil)
	if err != nil {
		return fmt.Errorf("failed to stop VM %s: %w", vmid, err)
	}

	ops.logger.Infof("Stopped VM %s", vmid)
	return nil
}

// ShutdownVM gracefully shuts down a virtual machine
func (ops *Operations) ShutdownVM(vmid string) error {
	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/nodes/%s/qemu/%s/status/shutdown", node, vmid)
	_, err = ops.client.Post(path, nil)
	if err != nil {
		return fmt.Errorf("failed to shutdown VM %s: %w", vmid, err)
	}

	ops.logger.Infof("Shutdown VM %s", vmid)
	return nil
}

// RebootVM reboots a virtual machine by shutting it down (ACPI) and starting
// it again, which applies any pending configuration changes.
func (ops *Operations) RebootVM(vmid string) error {
	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/nodes/%s/qemu/%s/status/reboot", node, vmid)
	_, err = ops.client.Post(path, nil)
	if err != nil {
		return fmt.Errorf("failed to reboot VM %s: %w", vmid, err)
	}

	ops.logger.Infof("Reboot VM %s", vmid)
	return nil
}

// ResetVM resets a virtual machine
func (ops *Operations) ResetVM(vmid string) error {
	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/nodes/%s/qemu/%s/status/reset", node, vmid)
	_, err = ops.client.Post(path, nil)
	if err != nil {
		return fmt.Errorf("failed to reset VM %s: %w", vmid, err)
	}

	ops.logger.Infof("Reset VM %s", vmid)
	return nil
}

// SuspendVM suspends a virtual machine.
//
// With toDisk false this pauses the VM in RAM (task type qmpause) and the
// guest stays resident on the node. With toDisk true the VM state is written
// out to storage (task type qmsuspend), which additionally requires
// Datastore.AllocateSpace on the state storage. stateStorage may be empty to
// let Proxmox pick the storage from the VM config; it is only meaningful when
// toDisk is set.
func (ops *Operations) SuspendVM(vmid string, toDisk bool, stateStorage string) error {
	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return err
	}

	data := url.Values{}
	if toDisk {
		data.Set("todisk", "1")
		if stateStorage != "" {
			data.Set("statestorage", stateStorage)
		}
	}

	path := fmt.Sprintf("/nodes/%s/qemu/%s/status/suspend", node, vmid)
	_, err = ops.client.Post(path, data)
	if err != nil {
		return fmt.Errorf("failed to suspend VM %s: %w", vmid, err)
	}

	ops.logger.Infof("Suspend VM %s (todisk=%v)", vmid, toDisk)
	return nil
}

// ResumeVM resumes a suspended virtual machine
func (ops *Operations) ResumeVM(vmid string) error {
	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/nodes/%s/qemu/%s/status/resume", node, vmid)
	_, err = ops.client.Post(path, nil)
	if err != nil {
		return fmt.Errorf("failed to resume VM %s: %w", vmid, err)
	}

	ops.logger.Infof("Resume VM %s", vmid)
	return nil
}

// GetVMConfig retrieves the current configuration of a VM. The returned map
// includes a "digest" key, which SetVMConfig can use to reject the write if
// the config changed in the meantime.
func (ops *Operations) GetVMConfig(vmid string) (map[string]interface{}, error) {
	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("/nodes/%s/qemu/%s/config", node, vmid)
	resp, err := ops.client.Get(path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get VM %s config: %w", vmid, err)
	}

	return resp, nil
}

// SetVMConfig updates the configuration of a VM.
//
// settings holds the keys to set; deleteKeys names keys to remove. When digest
// is non-empty the API rejects the write if the VM config has changed since
// that digest was read, which turns a concurrent modification into a clean
// error instead of a lost update.
//
// This uses PUT, which applies synchronously and returns no task. Changes that
// cannot be applied to a running VM are staged as pending and take effect on
// the next full stop/start or reboot — see GetVMPending.
func (ops *Operations) SetVMConfig(vmid string, settings map[string]string, deleteKeys []string, digest string) error {
	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return err
	}

	if len(settings) == 0 && len(deleteKeys) == 0 {
		return fmt.Errorf("no configuration changes specified for VM %s", vmid)
	}

	data := url.Values{}
	for key, value := range settings {
		data.Set(key, value)
	}
	if len(deleteKeys) > 0 {
		data.Set("delete", strings.Join(deleteKeys, ","))
	}
	if digest != "" {
		data.Set("digest", digest)
	}

	path := fmt.Sprintf("/nodes/%s/qemu/%s/config", node, vmid)
	_, err = ops.client.Put(path, data)
	if err != nil {
		return fmt.Errorf("failed to update VM %s config: %w", vmid, err)
	}

	ops.logger.Infof("Updated config for VM %s (%d set, %d deleted)", vmid, len(settings), len(deleteKeys))
	return nil
}

// PendingChange is a single VM config key with its current and pending values.
type PendingChange struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Pending string `json:"pending"`
	Delete  bool   `json:"delete"`
}

// GetVMPending returns the VM configuration with both current and pending
// values, so callers can see which changes are staged awaiting a power cycle.
func (ops *Operations) GetVMPending(vmid string) ([]*PendingChange, error) {
	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("/nodes/%s/qemu/%s/pending", node, vmid)
	resp, err := ops.client.Get(path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending changes for VM %s: %w", vmid, err)
	}

	var changes []*PendingChange
	items, ok := resp["items"].([]interface{})
	if !ok {
		return changes, nil
	}

	for _, item := range items {
		entry, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		change := &PendingChange{}
		if key, ok := entry["key"].(string); ok {
			change.Key = key
		}
		change.Value = stringifyConfigValue(entry["value"])
		change.Pending = stringifyConfigValue(entry["pending"])
		// delete is 1 for a pending delete and 2 for a forced one; both mean
		// the key is staged for removal.
		if del, ok := entry["delete"].(float64); ok && del != 0 {
			change.Delete = true
		}

		changes = append(changes, change)
	}

	return changes, nil
}

// CheckVMFeature reports whether a feature (e.g. "snapshot", "clone",
// "copy") is available for a VM. snapname is optional.
func (ops *Operations) CheckVMFeature(vmid, feature, snapname string) (bool, error) {
	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return false, err
	}

	params := map[string]string{"feature": feature}
	if snapname != "" {
		params["snapname"] = snapname
	}

	path := fmt.Sprintf("/nodes/%s/qemu/%s/feature", node, vmid)
	resp, err := ops.client.Get(path, params)
	if err != nil {
		return false, fmt.Errorf("failed to check feature %q for VM %s: %w", feature, vmid, err)
	}

	// The API reports availability as hasFeature 0/1.
	if hasFeature, ok := resp["hasFeature"].(float64); ok {
		return hasFeature != 0, nil
	}
	if hasFeature, ok := resp["hasFeature"].(bool); ok {
		return hasFeature, nil
	}

	return false, nil
}

// stringifyConfigValue renders a config value for display. Proxmox returns
// config values as strings or numbers depending on the key, so normalise both
// rather than losing numeric values to a failed type assertion.
func stringifyConfigValue(value interface{}) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		if v {
			return "1"
		}
		return "0"
	default:
		return fmt.Sprintf("%v", v)
	}
}

// taskPollInterval is how often MonitorTask re-reads a running task's
// status between polls.
const taskPollInterval = 1 * time.Second

// MonitorTask monitors a Proxmox task until completion, bounded both by the
// caller's context and by the configured task timeout (see SetTaskTimeout).
// It returns context.DeadlineExceeded when the timeout elapses and
// context.Canceled when the caller cancels — a task that never stops can no
// longer be polled forever. The HTTP client's per-request timeout remains a
// separate, additional bound on each individual poll.
func (ops *Operations) MonitorTask(ctx context.Context, node, upid string) error {
	timeout := ops.taskTimeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	path := fmt.Sprintf("/nodes/%s/tasks/%s/status", node, upid)

	for {
		resp, err := ops.client.Get(path, nil)
		if err != nil {
			return fmt.Errorf("failed to get task status: %w", err)
		}

		status := "running"
		if s, ok := resp["status"].(string); ok {
			status = s
		}

		exitStatus := ""
		if es, ok := resp["exitstatus"].(string); ok {
			exitStatus = es
		}

		if status == "stopped" {
			if exitStatus == "OK" || exitStatus == "" {
				return nil
			}
			return fmt.Errorf("task failed with exit status: %s", exitStatus)
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("task %s: %w", upid, ctx.Err())
		case <-time.After(taskPollInterval):
		}
	}
}

// GetTaskStatus gets the status of a Proxmox task
func (ops *Operations) GetTaskStatus(node, upid string) (*TaskStatus, error) {
	path := fmt.Sprintf("/nodes/%s/tasks/%s/status", node, upid)
	resp, err := ops.client.Get(path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get task status: %w", err)
	}

	task := &TaskStatus{UPID: upid}

	if status, ok := resp["status"].(string); ok {
		task.Status = status
	}
	if taskType, ok := resp["type"].(string); ok {
		task.Type = taskType
	}
	if exitCode, ok := resp["exitstatus"].(string); ok {
		task.ExitCode = exitCode
	}
	if progress, ok := resp["progress"].(float64); ok {
		task.Progress = progress
	}

	return task, nil
}

// VMExists checks if a VM exists
func (ops *Operations) VMExists(vmid string) bool {
	_, err := ops.FindVMNode(vmid)
	return err == nil
}

// ValidateVMID validates that a VM ID is numeric
func ValidateVMID(vmid string) error {
	if _, err := strconv.Atoi(vmid); err != nil {
		return fmt.Errorf("invalid VM ID '%s': must be numeric", vmid)
	}
	return nil
}
