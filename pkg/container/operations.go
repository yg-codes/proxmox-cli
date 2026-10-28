package container

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/yg-codes/proxmox-cli/pkg/api"
)

// Operations handles container management operations
type Operations struct {
	client *api.Client
	logger *logrus.Logger
}

// NewOperations creates a new container operations instance
func NewOperations(client *api.Client, logger *logrus.Logger) *Operations {
	if logger == nil {
		logger = logrus.New()
	}

	return &Operations{
		client: client,
		logger: logger,
	}
}

// GetContainers lists all LXC containers across the cluster with optional filtering
// API: GET /cluster/resources?type=vm
func (ops *Operations) GetContainers(filter *ContainerFilter) ([]*Container, error) {
	ops.logger.Debug("Fetching containers from cluster")

	// Note: We request "vm" type which includes both qemu and lxc, then filter for lxc
	params := map[string]string{
		"type": "vm",
	}

	resp, err := ops.client.Get("/cluster/resources", params)
	if err != nil {
		return nil, fmt.Errorf("failed to get containers: %w", err)
	}

	var containers []*Container

	if items, ok := resp["items"].([]interface{}); ok {
		for _, item := range items {
			if ctMap, ok := item.(map[string]interface{}); ok {
				// Only include lxc type containers
				if ctType, ok := ctMap["type"].(string); ok && ctType == "lxc" {
					ct := parseContainer(ctMap)
					if matchesContainerFilter(ct, filter) {
						containers = append(containers, ct)
					}
				}
			}
		}
	}

	ops.logger.Infof("Found %d containers", len(containers))
	return containers, nil
}

// GetContainer gets detailed information about a specific container
// API: GET /nodes/{node}/lxc/{vmid}/status/current
func (ops *Operations) GetContainer(node string, vmid int) (*Container, error) {
	ops.logger.Debugf("Fetching container %d on node %s", vmid, node)

	path := fmt.Sprintf("/nodes/%s/lxc/%d/status/current", node, vmid)
	resp, err := ops.client.Get(path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get container %d: %w", vmid, err)
	}

	ct := parseContainer(resp)
	ct.VMID = vmid
	ct.Node = node
	ct.Type = "lxc"

	return ct, nil
}

// GetContainerStatus gets the current status of a container
// API: GET /nodes/{node}/lxc/{vmid}/status/current
func (ops *Operations) GetContainerStatus(node string, vmid int) (*ContainerStatus, error) {
	ops.logger.Debugf("Fetching status for container %d on node %s", vmid, node)

	path := fmt.Sprintf("/nodes/%s/lxc/%d/status/current", node, vmid)
	resp, err := ops.client.Get(path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get container status: %w", err)
	}

	status := parseContainerStatus(resp)
	status.VMID = vmid
	status.Node = node

	return status, nil
}

// GetContainerConfig gets the configuration of a container
// API: GET /nodes/{node}/lxc/{vmid}/config
func (ops *Operations) GetContainerConfig(node string, vmid int) (map[string]interface{}, error) {
	ops.logger.Debugf("Fetching config for container %d on node %s", vmid, node)

	path := fmt.Sprintf("/nodes/%s/lxc/%d/config", node, vmid)
	resp, err := ops.client.Get(path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get container config for %d: %w", vmid, err)
	}

	return resp, nil
}

// CreateContainer creates a new LXC container
// API: POST /nodes/{node}/lxc
func (ops *Operations) CreateContainer(config *ContainerConfig) (string, error) {
	ops.logger.Infof("Creating container %d on node %s", config.VMID, config.Node)

	data := map[string]string{
		"vmid":       strconv.Itoa(config.VMID),
		"ostemplate": config.OSTemplate,
		"hostname":   config.Hostname,
		"storage":    config.Storage,
	}

	// Optional fields
	if config.Description != "" {
		data["description"] = config.Description
	}
	if config.Password != "" {
		data["password"] = config.Password
	}
	if config.SSHKeys != "" {
		data["ssh-public-keys"] = config.SSHKeys
	}
	if config.RootFS != "" {
		data["rootfs"] = config.RootFS
	}

	// Resources
	if config.CPUs > 0 {
		data["cores"] = strconv.Itoa(config.CPUs)
	}
	if config.Memory > 0 {
		data["memory"] = strconv.FormatInt(config.Memory/(1024*1024), 10) // Convert to MB
	}
	if config.Swap > 0 {
		data["swap"] = strconv.FormatInt(config.Swap/(1024*1024), 10)
	}

	// Features
	features := []string{}
	if config.Nesting {
		features = append(features, "nesting=1")
	}
	if config.KeyCTL {
		features = append(features, "keyctl=1")
	}
	if config.Fuse {
		features = append(features, "fuse=1")
	}
	if config.MountCIFS {
		features = append(features, "mount=cifs")
	}
	if config.MountNFS {
		features = append(features, "mount=nfs")
	}
	if len(features) > 0 {
		data["features"] = strings.Join(features, ",")
	}

	// Flags
	if config.Protected {
		data["protection"] = "1"
	}
	if config.OnBoot || config.StartOnBoot {
		data["onboot"] = "1"
	}
	if config.Unprivileged {
		data["unprivileged"] = "1"
	}

	// Network configuration
	for i, net := range config.Network {
		netConfig := fmt.Sprintf("name=%s,bridge=%s", net.Name, net.Bridge)
		if net.IP != "" {
			netConfig += fmt.Sprintf(",ip=%s", net.IP)
		}
		if net.IP6 != "" {
			netConfig += fmt.Sprintf(",ip6=%s", net.IP6)
		}
		if net.Gateway != "" {
			netConfig += fmt.Sprintf(",gw=%s", net.Gateway)
		}
		if net.Gateway6 != "" {
			netConfig += fmt.Sprintf(",gw6=%s", net.Gateway6)
		}
		if net.VLAN > 0 {
			netConfig += fmt.Sprintf(",tag=%d", net.VLAN)
		}
		if net.Firewall {
			netConfig += ",firewall=1"
		}
		if net.Rate > 0 {
			netConfig += fmt.Sprintf(",rate=%.2f", net.Rate)
		}
		data[fmt.Sprintf("net%d", i)] = netConfig
	}

	path := fmt.Sprintf("/nodes/%s/lxc", config.Node)
	resp, err := ops.client.Post(path, data)
	if err != nil {
		return "", fmt.Errorf("failed to create container: %w", err)
	}

	taskID, _ := resp["data"].(string)
	ops.logger.Infof("Container creation initiated, task ID: %s", taskID)
	return taskID, nil
}

// StartContainer starts a stopped container
// API: POST /nodes/{node}/lxc/{vmid}/status/start
func (ops *Operations) StartContainer(node string, vmid int) (string, error) {
	ops.logger.Infof("Starting container %d on node %s", vmid, node)

	path := fmt.Sprintf("/nodes/%s/lxc/%d/status/start", node, vmid)
	resp, err := ops.client.Post(path, nil)
	if err != nil {
		return "", fmt.Errorf("failed to start container %d: %w", vmid, err)
	}

	taskID, _ := resp["data"].(string)
	ops.logger.Infof("Container %d start initiated, task ID: %s", vmid, taskID)
	return taskID, nil
}

// StopContainer forcefully stops a running container
// API: POST /nodes/{node}/lxc/{vmid}/status/stop
func (ops *Operations) StopContainer(node string, vmid int) (string, error) {
	ops.logger.Infof("Stopping container %d on node %s", vmid, node)

	path := fmt.Sprintf("/nodes/%s/lxc/%d/status/stop", node, vmid)
	resp, err := ops.client.Post(path, nil)
	if err != nil {
		return "", fmt.Errorf("failed to stop container %d: %w", vmid, err)
	}

	taskID, _ := resp["data"].(string)
	ops.logger.Infof("Container %d stop initiated, task ID: %s", vmid, taskID)
	return taskID, nil
}

// ShutdownContainer gracefully shuts down a running container
// API: POST /nodes/{node}/lxc/{vmid}/status/shutdown
func (ops *Operations) ShutdownContainer(node string, vmid int, timeout int) (string, error) {
	ops.logger.Infof("Shutting down container %d on node %s (timeout: %ds)", vmid, node, timeout)

	data := make(map[string]string)
	if timeout > 0 {
		data["timeout"] = strconv.Itoa(timeout)
	}

	path := fmt.Sprintf("/nodes/%s/lxc/%d/status/shutdown", node, vmid)
	resp, err := ops.client.Post(path, data)
	if err != nil {
		return "", fmt.Errorf("failed to shutdown container %d: %w", vmid, err)
	}

	taskID, _ := resp["data"].(string)
	ops.logger.Infof("Container %d shutdown initiated, task ID: %s", vmid, taskID)
	return taskID, nil
}

// RestartContainer restarts a running container
// API: POST /nodes/{node}/lxc/{vmid}/status/reboot
func (ops *Operations) RestartContainer(node string, vmid int) (string, error) {
	ops.logger.Infof("Restarting container %d on node %s", vmid, node)

	path := fmt.Sprintf("/nodes/%s/lxc/%d/status/reboot", node, vmid)
	resp, err := ops.client.Post(path, nil)
	if err != nil {
		return "", fmt.Errorf("failed to restart container %d: %w", vmid, err)
	}

	taskID, _ := resp["data"].(string)
	ops.logger.Infof("Container %d restart initiated, task ID: %s", vmid, taskID)
	return taskID, nil
}

// DeleteContainer deletes a container
// API: DELETE /nodes/{node}/lxc/{vmid}
func (ops *Operations) DeleteContainer(node string, vmid int, purge bool) (string, error) {
	ops.logger.Infof("Deleting container %d on node %s (purge: %v)", vmid, node, purge)

	path := fmt.Sprintf("/nodes/%s/lxc/%d", node, vmid)
	if purge {
		path += "?purge=1"
	}

	resp, err := ops.client.Delete(path)
	if err != nil {
		return "", fmt.Errorf("failed to delete container %d: %w", vmid, err)
	}

	taskID, _ := resp["data"].(string)
	ops.logger.Infof("Container %d deletion initiated, task ID: %s", vmid, taskID)
	return taskID, nil
}

// CloneContainer clones a container
// API: POST /nodes/{node}/lxc/{vmid}/clone
func (ops *Operations) CloneContainer(node string, vmid int, options *ContainerCloneOptions) (string, error) {
	ops.logger.Infof("Cloning container %d to %d", vmid, options.NewID)

	data := map[string]string{
		"newid": strconv.Itoa(options.NewID),
	}

	if options.Hostname != "" {
		data["hostname"] = options.Hostname
	}
	if options.Description != "" {
		data["description"] = options.Description
	}
	if options.Pool != "" {
		data["pool"] = options.Pool
	}
	if options.SnapName != "" {
		data["snapname"] = options.SnapName
	}
	if options.Storage != "" {
		data["storage"] = options.Storage
	}
	if options.Target != "" {
		data["target"] = options.Target
	}
	if options.Full {
		data["full"] = "1"
	}

	path := fmt.Sprintf("/nodes/%s/lxc/%d/clone", node, vmid)
	resp, err := ops.client.Post(path, data)
	if err != nil {
		return "", fmt.Errorf("failed to clone container %d: %w", vmid, err)
	}

	taskID, _ := resp["data"].(string)
	ops.logger.Infof("Container %d clone initiated, task ID: %s", vmid, taskID)
	return taskID, nil
}

// CreateSnapshot creates a snapshot of a container
// API: POST /nodes/{node}/lxc/{vmid}/snapshot
func (ops *Operations) CreateSnapshot(node string, vmid int, snapName, description string) (string, error) {
	ops.logger.Infof("Creating snapshot '%s' for container %d", snapName, vmid)

	data := map[string]string{
		"snapname": snapName,
	}
	if description != "" {
		data["description"] = description
	}

	path := fmt.Sprintf("/nodes/%s/lxc/%d/snapshot", node, vmid)
	resp, err := ops.client.Post(path, data)
	if err != nil {
		return "", fmt.Errorf("failed to create snapshot for container %d: %w", vmid, err)
	}

	taskID, _ := resp["data"].(string)
	ops.logger.Infof("Snapshot '%s' creation initiated, task ID: %s", snapName, taskID)
	return taskID, nil
}

// ListSnapshots lists all snapshots for a container
// API: GET /nodes/{node}/lxc/{vmid}/snapshot
func (ops *Operations) ListSnapshots(node string, vmid int) ([]*ContainerSnapshot, error) {
	ops.logger.Debugf("Listing snapshots for container %d", vmid)

	path := fmt.Sprintf("/nodes/%s/lxc/%d/snapshot", node, vmid)
	resp, err := ops.client.Get(path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list snapshots for container %d: %w", vmid, err)
	}

	var snapshots []*ContainerSnapshot

	if items, ok := resp["items"].([]interface{}); ok {
		for _, item := range items {
			if snapMap, ok := item.(map[string]interface{}); ok {
				snap := parseContainerSnapshot(snapMap)
				snapshots = append(snapshots, snap)
			}
		}
	}

	ops.logger.Infof("Found %d snapshots for container %d", len(snapshots), vmid)
	return snapshots, nil
}

// RollbackSnapshot rolls back a container to a snapshot
// API: POST /nodes/{node}/lxc/{vmid}/snapshot/{snapname}/rollback
func (ops *Operations) RollbackSnapshot(node string, vmid int, snapName string) (string, error) {
	ops.logger.Infof("Rolling back container %d to snapshot '%s'", vmid, snapName)

	path := fmt.Sprintf("/nodes/%s/lxc/%d/snapshot/%s/rollback", node, vmid, snapName)
	resp, err := ops.client.Post(path, nil)
	if err != nil {
		return "", fmt.Errorf("failed to rollback container %d to snapshot '%s': %w", vmid, snapName, err)
	}

	taskID, _ := resp["data"].(string)
	ops.logger.Infof("Snapshot rollback initiated, task ID: %s", taskID)
	return taskID, nil
}

// DeleteSnapshot deletes a snapshot
// API: DELETE /nodes/{node}/lxc/{vmid}/snapshot/{snapname}
func (ops *Operations) DeleteSnapshot(node string, vmid int, snapName string) (string, error) {
	ops.logger.Infof("Deleting snapshot '%s' for container %d", snapName, vmid)

	path := fmt.Sprintf("/nodes/%s/lxc/%d/snapshot/%s", node, vmid, snapName)
	resp, err := ops.client.Delete(path)
	if err != nil {
		return "", fmt.Errorf("failed to delete snapshot '%s' for container %d: %w", snapName, vmid, err)
	}

	taskID, _ := resp["data"].(string)
	ops.logger.Infof("Snapshot deletion initiated, task ID: %s", taskID)
	return taskID, nil
}

// Helper functions

func parseContainer(ctMap map[string]interface{}) *Container {
	ct := &Container{}

	if vmid, ok := ctMap["vmid"].(float64); ok {
		ct.VMID = int(vmid)
	}
	if name, ok := ctMap["name"].(string); ok {
		ct.Name = name
	}
	if node, ok := ctMap["node"].(string); ok {
		ct.Node = node
	}
	if status, ok := ctMap["status"].(string); ok {
		ct.Status = status
	}
	if ctType, ok := ctMap["type"].(string); ok {
		ct.Type = ctType
	}

	// Resource allocation
	if cpus, ok := ctMap["cpus"].(float64); ok {
		ct.CPUs = int(cpus)
	}
	if maxcpu, ok := ctMap["maxcpu"].(float64); ok {
		ct.CPUs = int(maxcpu)
	}
	if mem, ok := ctMap["mem"].(float64); ok {
		ct.Memory = int64(mem)
	}
	if maxmem, ok := ctMap["maxmem"].(float64); ok {
		ct.Memory = int64(maxmem)
	}
	if swap, ok := ctMap["swap"].(float64); ok {
		ct.Swap = int64(swap)
	}
	if maxswap, ok := ctMap["maxswap"].(float64); ok {
		ct.Swap = int64(maxswap)
	}
	if disk, ok := ctMap["disk"].(float64); ok {
		ct.Disk = int64(disk)
	}
	if maxdisk, ok := ctMap["maxdisk"].(float64); ok {
		ct.Disk = int64(maxdisk)
	}

	// Runtime info
	if uptime, ok := ctMap["uptime"].(float64); ok {
		ct.Uptime = int64(uptime)
	}
	if pid, ok := ctMap["pid"].(float64); ok {
		ct.PID = int(pid)
	}

	// Network
	if netin, ok := ctMap["netin"].(float64); ok {
		ct.NetIn = int64(netin)
	}
	if netout, ok := ctMap["netout"].(float64); ok {
		ct.NetOut = int64(netout)
	}

	// Configuration
	if ostype, ok := ctMap["ostype"].(string); ok {
		ct.OSType = ostype
	}
	if arch, ok := ctMap["arch"].(string); ok {
		ct.Arch = arch
	}
	if hostname, ok := ctMap["hostname"].(string); ok {
		ct.Hostname = hostname
	}
	if desc, ok := ctMap["description"].(string); ok {
		ct.Description = desc
	}
	if protected, ok := ctMap["protected"].(float64); ok {
		ct.Protected = protected == 1
	}
	if template, ok := ctMap["template"].(float64); ok {
		ct.Template = template == 1
	}
	if lock, ok := ctMap["lock"].(string); ok {
		ct.Lock = lock
	}
	if rootfs, ok := ctMap["rootfs"].(string); ok {
		ct.RootFS = rootfs
	}

	return ct
}

func parseContainerStatus(statusMap map[string]interface{}) *ContainerStatus {
	status := &ContainerStatus{}

	if vmid, ok := statusMap["vmid"].(float64); ok {
		status.VMID = int(vmid)
	}
	if name, ok := statusMap["name"].(string); ok {
		status.Name = name
	}
	if statusStr, ok := statusMap["status"].(string); ok {
		status.Status = statusStr
	}
	if uptime, ok := statusMap["uptime"].(float64); ok {
		status.Uptime = int64(uptime)
	}
	if pid, ok := statusMap["pid"].(float64); ok {
		status.PID = int(pid)
	}

	// Resource usage
	if cpu, ok := statusMap["cpu"].(float64); ok {
		status.CPU = cpu
	}
	if maxcpu, ok := statusMap["maxcpu"].(float64); ok {
		status.MaxCPU = int(maxcpu)
		if status.MaxCPU > 0 {
			status.CPUPercent = (status.CPU / float64(status.MaxCPU)) * 100
		}
	}
	if mem, ok := statusMap["mem"].(float64); ok {
		status.Memory = int64(mem)
	}
	if maxmem, ok := statusMap["maxmem"].(float64); ok {
		status.MaxMemory = int64(maxmem)
		if status.MaxMemory > 0 {
			status.MemPercent = (float64(status.Memory) / float64(status.MaxMemory)) * 100
		}
	}
	if swap, ok := statusMap["swap"].(float64); ok {
		status.Swap = int64(swap)
	}
	if maxswap, ok := statusMap["maxswap"].(float64); ok {
		status.MaxSwap = int64(maxswap)
	}
	if disk, ok := statusMap["disk"].(float64); ok {
		status.Disk = int64(disk)
	}
	if maxdisk, ok := statusMap["maxdisk"].(float64); ok {
		status.MaxDisk = int64(maxdisk)
	}

	// Network
	if netin, ok := statusMap["netin"].(float64); ok {
		status.NetIn = int64(netin)
	}
	if netout, ok := statusMap["netout"].(float64); ok {
		status.NetOut = int64(netout)
	}

	// HA status
	if hastate, ok := statusMap["ha"].(map[string]interface{}); ok {
		if state, ok := hastate["state"].(string); ok {
			status.HAState = state
		}
	}

	return status
}

func parseContainerSnapshot(snapMap map[string]interface{}) *ContainerSnapshot {
	snap := &ContainerSnapshot{}

	if name, ok := snapMap["name"].(string); ok {
		snap.Name = name
	}
	if desc, ok := snapMap["description"].(string); ok {
		snap.Description = desc
	}
	if parent, ok := snapMap["parent"].(string); ok {
		snap.Parent = parent
	}
	if snaptime, ok := snapMap["snaptime"].(float64); ok {
		snap.SnapTime = parseTimestamp(int64(snaptime))
	}

	return snap
}

func matchesContainerFilter(ct *Container, filter *ContainerFilter) bool {
	if filter == nil {
		return true
	}

	if filter.Node != "" && ct.Node != filter.Node {
		return false
	}
	if filter.Status != "" && ct.Status != filter.Status {
		return false
	}
	if filter.Name != "" && !strings.Contains(strings.ToLower(ct.Name), strings.ToLower(filter.Name)) {
		return false
	}
	if filter.Template && !ct.Template {
		return false
	}

	return true
}

func parseTimestamp(timestamp int64) time.Time {
	if timestamp == 0 {
		return time.Time{}
	}
	return time.Unix(timestamp, 0)
}
