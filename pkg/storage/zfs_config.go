package storage

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/yg-codes/proxmox-cli/pkg/api"
)

// ZFSStorageOptions holds the configurable fields of a zfspool storage.
//
// Only the fields PVE::Storage::ZFSPoolPlugin actually accepts are here
// (options(): pool, blocksize, sparse, nodes, disable, content, bwlimit,
// mountpoint). Offering more would produce API rejections at apply time.
type ZFSStorageOptions struct {
	// Storage is the PVE storage ID. Required on create.
	Storage string
	// Pool is the ZFS pool (or dataset) to back it. Required on create, and
	// fixed thereafter — the API refuses to change it on an existing storage.
	Pool string
	// Content is a comma-separated list; zfspool supports images and rootdir.
	Content string
	// Nodes restricts the storage to specific nodes. Empty means all nodes.
	Nodes string
	// Blocksize is the volblocksize for new zvols, a power of two with an
	// optional k/m suffix.
	Blocksize string
	// Sparse creates thin-provisioned zvols.
	Sparse bool
	// Disable marks the storage inactive without removing it.
	Disable bool
	// Mountpoint overrides the dataset mountpoint.
	Mountpoint string
	// BWLimit caps migration/restore bandwidth.
	BWLimit string
	// Digest is optimistic-lock protection against a lost update on PUT.
	Digest string
}

// storagePermHint annotates a 403 from a /storage endpoint. These need
// Datastore.Allocate, which is a different privilege from the
// Datastore.AllocateSpace that VM disk operations use — granting the latter
// does not enable storage configuration.
func storagePermHint(action string, err error) error {
	var apiErr *api.ProxmoxAPIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusForbidden {
		return fmt.Errorf(
			"insufficient permission to %s: this requires Datastore.Allocate on /storage.\n"+
				"  Note that is NOT the same as Datastore.AllocateSpace (which only permits\n"+
				"  allocating volumes on an existing storage):\n"+
				"    pveum aclmod /storage -user '<user>@<realm>' -role PVEDatastoreAdmin",
			action)
	}
	return err
}

// CreateZFSStorage registers a ZFS pool as a PVE storage.
// API: POST /storage
func (ops *Operations) CreateZFSStorage(opts *ZFSStorageOptions) error {
	if opts == nil || opts.Storage == "" {
		return fmt.Errorf("a storage ID is required")
	}
	if opts.Pool == "" {
		return fmt.Errorf("a ZFS pool is required, e.g. --pool rpool/data")
	}

	data := url.Values{}
	data.Set("storage", opts.Storage)
	data.Set("type", "zfspool")
	data.Set("pool", opts.Pool)
	applyZFSStorageFields(data, opts)

	if _, err := ops.client.Post("/storage", data); err != nil {
		return storagePermHint(fmt.Sprintf("create storage %q", opts.Storage), err)
	}
	ops.logger.Infof("Created zfspool storage %s backed by %s", opts.Storage, opts.Pool)
	return nil
}

// UpdateZFSStorage changes an existing zfspool storage.
//
// Pool is deliberately not sent: the plugin declares it fixed, so including it
// makes the API reject an otherwise valid update.
// API: PUT /storage/{storage}
func (ops *Operations) UpdateZFSStorage(opts *ZFSStorageOptions, deleteKeys []string) error {
	if opts == nil || opts.Storage == "" {
		return fmt.Errorf("a storage ID is required")
	}

	data := url.Values{}
	applyZFSStorageFields(data, opts)
	if opts.Digest != "" {
		data.Set("digest", opts.Digest)
	}
	if len(deleteKeys) > 0 {
		data.Set("delete", strings.Join(deleteKeys, ","))
	}
	if len(data) == 0 {
		return fmt.Errorf("nothing to update: pass at least one field to change or delete")
	}

	if _, err := ops.client.Put("/storage/"+opts.Storage, data); err != nil {
		return storagePermHint(fmt.Sprintf("update storage %q", opts.Storage), err)
	}
	ops.logger.Infof("Updated storage %s", opts.Storage)
	return nil
}

// DeleteStorage removes a storage from the cluster configuration.
//
// This unregisters the storage; it does NOT destroy the underlying ZFS pool or
// its data. Use the pool-destroy path for that.
// API: DELETE /storage/{storage}
func (ops *Operations) DeleteStorage(storageID string) error {
	if storageID == "" {
		return fmt.Errorf("a storage ID is required")
	}
	if _, err := ops.client.Delete("/storage/" + storageID); err != nil {
		return storagePermHint(fmt.Sprintf("delete storage %q", storageID), err)
	}
	ops.logger.Infof("Removed storage %s from the cluster config", storageID)
	return nil
}

// GetStorageConfig reads one storage's configuration, including its digest.
// API: GET /storage/{storage}
func (ops *Operations) GetStorageConfig(storageID string) (map[string]interface{}, error) {
	resp, err := ops.client.Get("/storage/"+storageID, nil)
	if err != nil {
		return nil, storagePermHint(fmt.Sprintf("read storage %q", storageID), err)
	}
	// api.Client unwraps a single-object "data" field, so resp is normally the
	// config itself; the nested form is handled for safety.
	if data, ok := resp["data"].(map[string]interface{}); ok {
		return data, nil
	}
	return resp, nil
}

// ListZFSStorages returns the configured zfspool storages.
// API: GET /storage?type=zfspool
func (ops *Operations) ListZFSStorages() ([]map[string]interface{}, error) {
	resp, err := ops.client.Get("/storage?type=zfspool", nil)
	if err != nil {
		return nil, storagePermHint("list ZFS storages", err)
	}
	items, ok := resp["items"].([]interface{})
	if !ok {
		return nil, nil
	}
	out := make([]map[string]interface{}, 0, len(items))
	for _, raw := range items {
		if m, ok := raw.(map[string]interface{}); ok {
			out = append(out, m)
		}
	}
	return out, nil
}

// applyZFSStorageFields sets the mutable fields shared by create and update.
func applyZFSStorageFields(data url.Values, opts *ZFSStorageOptions) {
	if opts.Content != "" {
		data.Set("content", opts.Content)
	}
	if opts.Nodes != "" {
		data.Set("nodes", opts.Nodes)
	}
	if opts.Blocksize != "" {
		data.Set("blocksize", opts.Blocksize)
	}
	if opts.Mountpoint != "" {
		data.Set("mountpoint", opts.Mountpoint)
	}
	if opts.BWLimit != "" {
		data.Set("bwlimit", opts.BWLimit)
	}
	if opts.Sparse {
		data.Set("sparse", "1")
	}
	if opts.Disable {
		data.Set("disable", "1")
	}
}
