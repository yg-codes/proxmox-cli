package node

import (
	"fmt"
	"net/url"

	"github.com/sirupsen/logrus"
	"github.com/yg-codes/proxmox-cli/pkg/api"
)

// Operations handles node operations
type Operations struct {
	client *api.Client
	logger *logrus.Logger
}

// NewOperations creates a new node operations instance
func NewOperations(client *api.Client, logger *logrus.Logger) *Operations {
	if logger == nil {
		logger = logrus.New()
	}

	return &Operations{
		client: client,
		logger: logger,
	}
}

// GetNodes lists all cluster nodes
// API: GET /nodes
func (ops *Operations) GetNodes() ([]*Node, error) {
	ops.logger.Debug("Fetching cluster nodes")

	resp, err := ops.client.Get("/nodes", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get nodes: %w", err)
	}

	var nodes []*Node
	if items, ok := resp["items"].([]interface{}); ok {
		for _, item := range items {
			if nodeMap, ok := item.(map[string]interface{}); ok {
				node := parseNodeBasic(nodeMap)
				nodes = append(nodes, node)
			}
		}
	}

	ops.logger.Infof("Found %d cluster nodes", len(nodes))
	return nodes, nil
}

// GetNodeStatus gets detailed status for a specific node
// API: GET /nodes/{node}/status
func (ops *Operations) GetNodeStatus(nodeName string) (*Node, error) {
	ops.logger.Debugf("Fetching status for node: %s", nodeName)

	path := fmt.Sprintf("/nodes/%s/status", nodeName)
	resp, err := ops.client.Get(path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get node %s status: %w", nodeName, err)
	}

	node := parseNodeDetailed(nodeName, resp)
	return node, nil
}

// GetNodeServices lists all services on a node
// API: GET /nodes/{node}/services
func (ops *Operations) GetNodeServices(nodeName string) ([]*Service, error) {
	ops.logger.Debugf("Fetching services for node: %s", nodeName)

	path := fmt.Sprintf("/nodes/%s/services", nodeName)
	resp, err := ops.client.Get(path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get services for node %s: %w", nodeName, err)
	}

	var services []*Service
	if items, ok := resp["items"].([]interface{}); ok {
		for _, item := range items {
			if svcMap, ok := item.(map[string]interface{}); ok {
				service := parseService(svcMap)
				services = append(services, service)
			}
		}
	}

	ops.logger.Infof("Found %d services on node %s", len(services), nodeName)
	return services, nil
}

// GetServiceStatus gets status of a specific service
// API: GET /nodes/{node}/services/{service}
func (ops *Operations) GetServiceStatus(nodeName, serviceName string) (*Service, error) {
	ops.logger.Debugf("Fetching service status: %s on node %s", serviceName, nodeName)

	path := fmt.Sprintf("/nodes/%s/services/%s", nodeName, serviceName)
	resp, err := ops.client.Get(path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get service %s status on node %s: %w", serviceName, nodeName, err)
	}

	service := parseService(resp)
	return service, nil
}

// StartService starts a node service
// API: POST /nodes/{node}/services/{service}/start
func (ops *Operations) StartService(nodeName, serviceName string) error {
	ops.logger.Infof("Starting service %s on node %s", serviceName, nodeName)

	path := fmt.Sprintf("/nodes/%s/services/%s/start", nodeName, serviceName)
	_, err := ops.client.Post(path, nil)
	if err != nil {
		return fmt.Errorf("failed to start service %s on node %s: %w", serviceName, nodeName, err)
	}

	ops.logger.Infof("✅ Started service %s on node %s", serviceName, nodeName)
	return nil
}

// StopService stops a node service
// API: POST /nodes/{node}/services/{service}/stop
func (ops *Operations) StopService(nodeName, serviceName string) error {
	ops.logger.Infof("Stopping service %s on node %s", serviceName, nodeName)

	path := fmt.Sprintf("/nodes/%s/services/%s/stop", nodeName, serviceName)
	_, err := ops.client.Post(path, nil)
	if err != nil {
		return fmt.Errorf("failed to stop service %s on node %s: %w", serviceName, nodeName, err)
	}

	ops.logger.Infof("✅ Stopped service %s on node %s", serviceName, nodeName)
	return nil
}

// RestartService restarts a node service
// API: POST /nodes/{node}/services/{service}/restart
func (ops *Operations) RestartService(nodeName, serviceName string) error {
	ops.logger.Infof("Restarting service %s on node %s", serviceName, nodeName)

	path := fmt.Sprintf("/nodes/%s/services/%s/restart", nodeName, serviceName)
	_, err := ops.client.Post(path, nil)
	if err != nil {
		return fmt.Errorf("failed to restart service %s on node %s: %w", serviceName, nodeName, err)
	}

	ops.logger.Infof("✅ Restarted service %s on node %s", serviceName, nodeName)
	return nil
}

// RebootNode reboots a node
// API: POST /nodes/{node}/status (command=reboot)
func (ops *Operations) RebootNode(nodeName string) error {
	ops.logger.Infof("Rebooting node %s", nodeName)

	path := fmt.Sprintf("/nodes/%s/status", nodeName)
	data := url.Values{
		"command": {"reboot"},
	}

	_, err := ops.client.Post(path, data)
	if err != nil {
		return fmt.Errorf("failed to reboot node %s: %w", nodeName, err)
	}

	ops.logger.Infof("✅ Reboot command sent to node %s", nodeName)
	return nil
}

// ShutdownNode shuts down a node
// API: POST /nodes/{node}/status (command=shutdown)
func (ops *Operations) ShutdownNode(nodeName string) error {
	ops.logger.Infof("Shutting down node %s", nodeName)

	path := fmt.Sprintf("/nodes/%s/status", nodeName)
	data := url.Values{
		"command": {"shutdown"},
	}

	_, err := ops.client.Post(path, data)
	if err != nil {
		return fmt.Errorf("failed to shutdown node %s: %w", nodeName, err)
	}

	ops.logger.Infof("✅ Shutdown command sent to node %s", nodeName)
	return nil
}

// GetNodeVersion gets node version information
// API: GET /nodes/{node}/version
func (ops *Operations) GetNodeVersion(nodeName string) (*VersionInfo, error) {
	ops.logger.Debugf("Fetching version info for node: %s", nodeName)

	path := fmt.Sprintf("/nodes/%s/version", nodeName)
	resp, err := ops.client.Get(path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get version for node %s: %w", nodeName, err)
	}

	version := &VersionInfo{}
	if v, ok := resp["version"].(string); ok {
		version.Version = v
	}
	if r, ok := resp["release"].(string); ok {
		version.Release = r
	}
	if repo, ok := resp["repoid"].(string); ok {
		version.RepoID = repo
	}
	if k, ok := resp["kernel"].(string); ok {
		version.Kernel = k
	}

	return version, nil
}

// Helper functions to parse API responses

func parseNodeBasic(nodeMap map[string]interface{}) *Node {
	node := &Node{}

	if name, ok := nodeMap["node"].(string); ok {
		node.Name = name
	}
	if status, ok := nodeMap["status"].(string); ok {
		node.Status = status
		node.Online = status == "online"
	}
	if ip, ok := nodeMap["ip"].(string); ok {
		node.IP = ip
	}
	if level, ok := nodeMap["level"].(string); ok {
		node.Level = level
	}
	if id, ok := nodeMap["id"].(string); ok {
		node.ID = id
	}
	if nodeID, ok := nodeMap["nodeid"].(float64); ok {
		node.NodeID = int(nodeID)
	}

	// Resource metrics (may not be available in basic list)
	if cpu, ok := nodeMap["cpu"].(float64); ok {
		node.CPUUsage = cpu
	}
	if maxCPU, ok := nodeMap["maxcpu"].(float64); ok {
		node.MaxCPU = int(maxCPU)
	}
	if mem, ok := nodeMap["mem"].(float64); ok {
		node.MemoryUsed = int64(mem)
	}
	if maxMem, ok := nodeMap["maxmem"].(float64); ok {
		node.Memory = int64(maxMem)
	}
	if disk, ok := nodeMap["disk"].(float64); ok {
		node.DiskUsed = int64(disk)
	}
	if maxDisk, ok := nodeMap["maxdisk"].(float64); ok {
		node.Disk = int64(maxDisk)
	}
	if uptime, ok := nodeMap["uptime"].(float64); ok {
		node.Uptime = int64(uptime)
	}

	return node
}

func parseNodeDetailed(nodeName string, resp map[string]interface{}) *Node {
	node := &Node{
		Name:   nodeName,
		Online: true,
	}

	if cpu, ok := resp["cpu"].(float64); ok {
		node.CPUUsage = cpu
	}
	if cpuInfo, ok := resp["cpuinfo"].(map[string]interface{}); ok {
		if cpus, ok := cpuInfo["cpus"].(float64); ok {
			node.MaxCPU = int(cpus)
		}
	}
	if mem, ok := resp["memory"].(map[string]interface{}); ok {
		if total, ok := mem["total"].(float64); ok {
			node.Memory = int64(total)
		}
		if used, ok := mem["used"].(float64); ok {
			node.MemoryUsed = int64(used)
		}
		if free, ok := mem["free"].(float64); ok {
			node.MemoryFree = int64(free)
		}
	}
	if rootfs, ok := resp["rootfs"].(map[string]interface{}); ok {
		if total, ok := rootfs["total"].(float64); ok {
			node.Disk = int64(total)
		}
		if used, ok := rootfs["used"].(float64); ok {
			node.DiskUsed = int64(used)
		}
		if free, ok := rootfs["free"].(float64); ok {
			node.DiskFree = int64(free)
		}
	}
	if uptime, ok := resp["uptime"].(float64); ok {
		node.Uptime = int64(uptime)
	}
	if kversion, ok := resp["kversion"].(string); ok {
		node.KernelVersion = kversion
	}
	if pveversion, ok := resp["pveversion"].(string); ok {
		node.PVEVersion = pveversion
	}

	return node
}

func parseService(svcMap map[string]interface{}) *Service {
	service := &Service{}

	if name, ok := svcMap["name"].(string); ok {
		service.Name = name
	}
	if state, ok := svcMap["state"].(string); ok {
		service.State = state
		service.Running = state == "running"
	}
	if desc, ok := svcMap["desc"].(string); ok {
		service.Description = desc
	}
	if unitState, ok := svcMap["unit-state"].(string); ok {
		service.UnitState = unitState
		service.Active = unitState == "active"
	}

	// Some services report "active-state" instead
	if activeState, ok := svcMap["active-state"].(string); ok {
		service.Active = activeState == "active"
	}

	return service
}
