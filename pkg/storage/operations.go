package storage

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/yg-codes/proxmox-cli/pkg/api"
)

// Storage represents a Proxmox storage
type Storage struct {
	Name         string
	Type         string
	Node         string
	Active       bool
	ContentTypes string
	AvailableGB  float64
	TotalGB      float64
}

// Capacity is one storage's free space as reported by a specific node.
//
// Node matters: a shared store (Ceph, NFS) reports the same pool from every
// node, but a local store (ZFS, LVM, dir) reports only that node's disk. A
// capacity check that ignores the node can therefore pass against the wrong
// pool entirely.
type Capacity struct {
	Storage   string
	Node      string
	Type      string
	Shared    bool
	Active    bool
	AvailByte int64
	TotalByte int64
}

// GetStorageCapacity returns free space for one storage as seen by one node.
//
// Returns a nil Capacity with no error when the node does not have the storage
// configured — callers should treat that as "cannot determine", not as zero
// space, because refusing an operation on a missing reading is worse than
// letting Proxmox give the authoritative answer.
func (ops *Operations) GetStorageCapacity(node, storageName string) (*Capacity, error) {
	resp, err := ops.client.Get(fmt.Sprintf("/nodes/%s/storage", node), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to read storage on node %s: %w", node, err)
	}

	items, ok := resp["items"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected storage response for node %s", node)
	}

	for _, raw := range items {
		s, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if name, _ := s["storage"].(string); name != storageName {
			continue
		}

		c := &Capacity{Storage: storageName, Node: node}
		if v, ok := s["type"].(string); ok {
			c.Type = v
		}
		if v, ok := s["shared"].(float64); ok {
			c.Shared = v != 0
		}
		if v, ok := s["active"].(float64); ok {
			c.Active = v != 0
		}
		if v, ok := s["avail"].(float64); ok {
			c.AvailByte = int64(v)
		}
		if v, ok := s["total"].(float64); ok {
			c.TotalByte = int64(v)
		}
		return c, nil
	}

	return nil, nil // not configured on this node
}

// Operations handles storage operations
type Operations struct {
	client *api.Client
	logger *logrus.Logger
}

// NewOperations creates a new storage operations handler
func NewOperations(client *api.Client, logger *logrus.Logger) *Operations {
	return &Operations{
		client: client,
		logger: logger,
	}
}

// GetVMStorages gets all available storages suitable for VM disks
func (ops *Operations) GetVMStorages() ([]*Storage, error) {
	ops.logger.Debug("Fetching VM disk storages")

	// Get all nodes
	nodesResp, err := ops.client.Get("/nodes", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get nodes: %w", err)
	}

	// API client wraps array responses in "items" key
	nodes, ok := nodesResp["items"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid nodes response")
	}

	uniqueStorages := make(map[string]*Storage)

	for _, nodeData := range nodes {
		node, ok := nodeData.(map[string]interface{})
		if !ok {
			continue
		}

		nodeName := node["node"].(string)

		// Get storages for this node
		storagesResp, err := ops.client.Get(fmt.Sprintf("/nodes/%s/storage", nodeName), nil)
		if err != nil {
			ops.logger.Debugf("Failed to get storages for node %s: %v", nodeName, err)
			continue
		}

		// API client wraps array responses in "items" key
		storages, ok := storagesResp["items"].([]interface{})
		if !ok {
			continue
		}

		for _, storageData := range storages {
			storage, ok := storageData.(map[string]interface{})
			if !ok {
				continue
			}

			storageName := storage["storage"].(string)

			// Get storage status to check content types
			statusResp, err := ops.client.Get(
				fmt.Sprintf("/nodes/%s/storage/%s/status", nodeName, storageName),
				nil,
			)
			if err != nil {
				continue
			}

			// Status endpoint returns data directly at root level (no "data" wrapper)
			statusData := statusResp

			content := ""
			if c, ok := statusData["content"].(string); ok {
				content = c
			}

			// Check if storage supports VM images or root directories
			if strings.Contains(content, "images") || strings.Contains(content, "rootdir") {
				if _, exists := uniqueStorages[storageName]; !exists {
					storageType := storage["type"].(string)
					active := statusData["active"].(float64) == 1

					avail := 0.0
					total := 0.0
					if a, ok := statusData["avail"].(float64); ok {
						avail = a / (1024 * 1024 * 1024) // Convert to GB
					}
					if t, ok := statusData["total"].(float64); ok {
						total = t / (1024 * 1024 * 1024) // Convert to GB
					}

					uniqueStorages[storageName] = &Storage{
						Name:         storageName,
						Type:         storageType,
						Node:         nodeName,
						Active:       active,
						ContentTypes: content,
						AvailableGB:  avail,
						TotalGB:      total,
					}
				}
			}
		}
	}

	// Convert map to slice
	result := make([]*Storage, 0, len(uniqueStorages))
	for _, storage := range uniqueStorages {
		result = append(result, storage)
	}

	ops.logger.Infof("Found %d VM disk storages", len(result))
	return result, nil
}

// GetBackupStorages gets all available storages suitable for backups
func (ops *Operations) GetBackupStorages() ([]*Storage, error) {
	ops.logger.Debug("Fetching backup storages")

	// Get all nodes
	nodesResp, err := ops.client.Get("/nodes", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get nodes: %w", err)
	}

	// API client wraps array responses in "items" key
	nodes, ok := nodesResp["items"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid nodes response")
	}

	uniqueStorages := make(map[string]*Storage)

	for _, nodeData := range nodes {
		node, ok := nodeData.(map[string]interface{})
		if !ok {
			continue
		}

		nodeName := node["node"].(string)

		// Get storages for this node
		storagesResp, err := ops.client.Get(fmt.Sprintf("/nodes/%s/storage", nodeName), nil)
		if err != nil {
			ops.logger.Debugf("Failed to get storages for node %s: %v", nodeName, err)
			continue
		}

		// API client wraps array responses in "items" key
		storages, ok := storagesResp["items"].([]interface{})
		if !ok {
			continue
		}

		for _, storageData := range storages {
			storage, ok := storageData.(map[string]interface{})
			if !ok {
				continue
			}

			storageName := storage["storage"].(string)

			// Get storage status to check content types
			statusResp, err := ops.client.Get(
				fmt.Sprintf("/nodes/%s/storage/%s/status", nodeName, storageName),
				nil,
			)
			if err != nil {
				continue
			}

			// Status endpoint returns data directly at root level (no "data" wrapper)
			statusData := statusResp

			content := ""
			if c, ok := statusData["content"].(string); ok {
				content = c
			}

			// Check if storage supports backups
			if strings.Contains(content, "backup") || strings.Contains(content, "vztmpl") {
				if _, exists := uniqueStorages[storageName]; !exists {
					storageType := storage["type"].(string)
					active := statusData["active"].(float64) == 1

					avail := 0.0
					total := 0.0
					if a, ok := statusData["avail"].(float64); ok {
						avail = a / (1024 * 1024 * 1024) // Convert to GB
					}
					if t, ok := statusData["total"].(float64); ok {
						total = t / (1024 * 1024 * 1024) // Convert to GB
					}

					uniqueStorages[storageName] = &Storage{
						Name:         storageName,
						Type:         storageType,
						Node:         nodeName,
						Active:       active,
						ContentTypes: content,
						AvailableGB:  avail,
						TotalGB:      total,
					}
				}
			}
		}
	}

	// Convert map to slice
	result := make([]*Storage, 0, len(uniqueStorages))
	for _, storage := range uniqueStorages {
		result = append(result, storage)
	}

	ops.logger.Infof("Found %d backup storages", len(result))
	return result, nil
}

// DisplayVMStorages displays VM storages in a formatted table
func (ops *Operations) DisplayVMStorages() error {
	storages, err := ops.GetVMStorages()
	if err != nil {
		return err
	}

	if len(storages) == 0 {
		fmt.Println("❌ No VM disk storages found")
		return nil
	}

	fmt.Println("\nAvailable VM Disk Storages:")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("%-3s %-15s %-10s %-10s %-20s %-15s\n",
		"#", "Storage", "Type", "Status", "Content Types", "Free Space")
	fmt.Println(strings.Repeat("-", 80))

	for i, storage := range storages {
		status := "🔴 inactive"
		if storage.Active {
			status = "🟢 active"
		}

		freeSpace := "N/A"
		if storage.AvailableGB > 0 {
			freeSpace = fmt.Sprintf("%.1f GB", storage.AvailableGB)
		}

		fmt.Printf("%-3d %-15s %-10s %-10s %-20s %-15s\n",
			i+1, storage.Name, storage.Type, status, storage.ContentTypes, freeSpace)
	}

	fmt.Println(strings.Repeat("-", 80))
	fmt.Printf("Total VM disk storages: %d\n", len(storages))

	return nil
}

// DisplayBackupStorages displays backup storages in a formatted table
func (ops *Operations) DisplayBackupStorages() error {
	storages, err := ops.GetBackupStorages()
	if err != nil {
		return err
	}

	if len(storages) == 0 {
		fmt.Println("❌ No backup-capable storages found")
		return nil
	}

	fmt.Println("\nAvailable Backup Storages:")
	fmt.Println(strings.Repeat("=", 70))
	fmt.Printf("%-3s %-15s %-10s %-10s %-15s %-15s\n",
		"#", "Storage", "Type", "Status", "Free Space", "Total Space")
	fmt.Println(strings.Repeat("-", 70))

	for i, storage := range storages {
		status := "🔴 inactive"
		if storage.Active {
			status = "🟢 active"
		}

		freeSpace := "N/A"
		totalSpace := "N/A"
		if storage.TotalGB > 0 {
			freeSpace = fmt.Sprintf("%.1f GB", storage.AvailableGB)
			totalSpace = fmt.Sprintf("%.1f GB", storage.TotalGB)
		}

		fmt.Printf("%-3d %-15s %-10s %-10s %-15s %-15s\n",
			i+1, storage.Name, storage.Type, status, freeSpace, totalSpace)
	}

	fmt.Println(strings.Repeat("-", 70))
	fmt.Printf("Total storages: %d\n", len(storages))

	return nil
}

// ValidateStorage checks if a storage exists and is active
func (ops *Operations) ValidateStorage(storageName string) error {
	backupStorages, err := ops.GetBackupStorages()
	if err != nil {
		return err
	}

	for _, storage := range backupStorages {
		if storage.Name == storageName {
			if !storage.Active {
				return fmt.Errorf("storage %s is inactive", storageName)
			}
			if storage.AvailableGB < 1 {
				return fmt.Errorf("storage %s has insufficient space", storageName)
			}
			return nil
		}
	}

	return fmt.Errorf("storage %s not found or not suitable for backups", storageName)
}
