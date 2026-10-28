// Package replication manages Proxmox storage replication jobs.
//
// Replication is ZFS-specific in practice: zfspool is currently the only
// replication-capable storage plugin, and the engine works by zfs send/recv
// between nodes. It is therefore the closest thing the REST API offers to
// dataset-level ZFS management — the raw send/recv calls are not exposed.
package replication

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/yg-codes/proxmox-cli/pkg/api"
)

// Job is one replication job from /cluster/replication.
type Job struct {
	// ID has the form "<vmid>-<number>", e.g. "100-0". The number lets one
	// guest have several jobs to different targets.
	ID       string
	Type     string
	Source   string
	Target   string
	Schedule string
	Rate     float64
	Comment  string
	Disable  bool
	// RemoveJob is "local" or "full" when the job is being torn down.
	RemoveJob string
}

// Status is a job's runtime state from /nodes/{node}/replication.
type Status struct {
	ID            string
	VMID          string
	Target        string
	LastSync      float64
	NextSync      float64
	Duration      float64
	FailCount     int
	Error         string
	LastTry       float64
	GuestType     string
	JobNum        string
	PID           int
	IsRunning     bool
	ScheduleValid bool
}

// Operations handles replication job management.
type Operations struct {
	client *api.Client
	logger *logrus.Logger
}

// NewOperations creates a replication operations handler.
func NewOperations(client *api.Client, logger *logrus.Logger) *Operations {
	if logger == nil {
		logger = logrus.New()
	}
	return &Operations{client: client, logger: logger}
}

// permHint annotates a 403. Reads need VM.Audit; writes need VM.Replicate,
// which is a distinct privilege that VM.Audit does not imply.
func permHint(action string, err error) error {
	var apiErr *api.ProxmoxAPIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusForbidden {
		return fmt.Errorf(
			"insufficient permission to %s: replication reads need VM.Audit and "+
				"changes need VM.Replicate on /vms/<vmid>.\n"+
				"  PVEVMAdmin includes both; PVEAuditor grants only the read side",
			action)
	}
	return err
}

// ListJobs returns the configured replication jobs.
// API: GET /cluster/replication
func (ops *Operations) ListJobs() ([]*Job, error) {
	resp, err := ops.client.Get("/cluster/replication", nil)
	if err != nil {
		return nil, permHint("list replication jobs", err)
	}

	items, ok := resp["items"].([]interface{})
	if !ok {
		return nil, nil
	}
	jobs := make([]*Job, 0, len(items))
	for _, raw := range items {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		jobs = append(jobs, jobFromMap(m))
	}
	return jobs, nil
}

// GetJob reads one replication job.
// API: GET /cluster/replication/{id}
func (ops *Operations) GetJob(id string) (*Job, error) {
	resp, err := ops.client.Get("/cluster/replication/"+id, nil)
	if err != nil {
		return nil, permHint("read replication job "+id, err)
	}
	// api.Client unwraps a single-object "data" field, so resp is the object.
	if data, ok := resp["data"].(map[string]interface{}); ok {
		return jobFromMap(data), nil
	}
	if len(resp) > 0 {
		return jobFromMap(resp), nil
	}
	return nil, fmt.Errorf("replication job %s not found", id)
}

// CreateJob creates a replication job.
//
// The ID must be "<vmid>-<number>"; Proxmox derives the guest from it, so a
// malformed id fails with a pattern error rather than a missing-VM error.
// API: POST /cluster/replication
func (ops *Operations) CreateJob(job *Job) error {
	if job == nil {
		return fmt.Errorf("a job definition is required")
	}
	if err := ValidateJobID(job.ID); err != nil {
		return err
	}
	if job.Target == "" {
		return fmt.Errorf("a target node is required")
	}

	data := url.Values{}
	data.Set("id", job.ID)
	data.Set("type", "local") // the only type the API supports today
	data.Set("target", job.Target)
	applyJobFields(data, job)

	if _, err := ops.client.Post("/cluster/replication", data); err != nil {
		return permHint("create replication job "+job.ID, err)
	}
	ops.logger.Infof("Created replication job %s -> %s", job.ID, job.Target)
	return nil
}

// UpdateJob changes an existing replication job.
// API: PUT /cluster/replication/{id}
func (ops *Operations) UpdateJob(job *Job, deleteKeys []string) error {
	if job == nil || job.ID == "" {
		return fmt.Errorf("a job ID is required")
	}

	data := url.Values{}
	applyJobFields(data, job)
	// Target is mutable on update, unlike on a storage's pool.
	if job.Target != "" {
		data.Set("target", job.Target)
	}
	if len(deleteKeys) > 0 {
		data.Set("delete", strings.Join(deleteKeys, ","))
	}
	if len(data) == 0 {
		return fmt.Errorf("nothing to update: pass at least one field to change or delete")
	}

	if _, err := ops.client.Put("/cluster/replication/"+job.ID, data); err != nil {
		return permHint("update replication job "+job.ID, err)
	}
	ops.logger.Infof("Updated replication job %s", job.ID)
	return nil
}

// DeleteJob marks a replication job for removal.
//
// keep=false (the default) also removes the replicated data on the target;
// keep=true leaves it in place. force skips the removal of remote data when the
// target is unreachable, which otherwise blocks the deletion.
// API: DELETE /cluster/replication/{id}
func (ops *Operations) DeleteJob(id string, keep, force bool) error {
	if id == "" {
		return fmt.Errorf("a job ID is required")
	}

	path := "/cluster/replication/" + id
	params := url.Values{}
	if keep {
		params.Set("keep", "1")
	}
	if force {
		params.Set("force", "1")
	}
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	if _, err := ops.client.Delete(path); err != nil {
		return permHint("delete replication job "+id, err)
	}
	ops.logger.Infof("Marked replication job %s for removal (keep=%v force=%v)", id, keep, force)
	return nil
}

// GetStatus returns the runtime state of replication jobs on a node.
// API: GET /nodes/{node}/replication
func (ops *Operations) GetStatus(node string) ([]*Status, error) {
	if node == "" {
		return nil, fmt.Errorf("a node is required")
	}
	resp, err := ops.client.Get(fmt.Sprintf("/nodes/%s/replication", node), nil)
	if err != nil {
		return nil, permHint("read replication status on "+node, err)
	}

	items, ok := resp["items"].([]interface{})
	if !ok {
		return nil, nil
	}
	out := make([]*Status, 0, len(items))
	for _, raw := range items {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		s := &Status{}
		s.ID, _ = m["id"].(string)
		s.Target, _ = m["target"].(string)
		s.Error, _ = m["error"].(string)
		s.GuestType, _ = m["guest_type"].(string)
		s.JobNum, _ = m["jobnum"].(string)
		if v, ok := m["vmid"].(float64); ok {
			s.VMID = fmt.Sprintf("%.0f", v)
		} else if v, ok := m["vmid"].(string); ok {
			s.VMID = v
		}
		s.LastSync, _ = m["last_sync"].(float64)
		s.NextSync, _ = m["next_sync"].(float64)
		s.Duration, _ = m["duration"].(float64)
		s.LastTry, _ = m["last_try"].(float64)
		if v, ok := m["fail_count"].(float64); ok {
			s.FailCount = int(v)
		}
		if v, ok := m["pid"].(float64); ok {
			s.PID = int(v)
			s.IsRunning = s.PID != 0
		}
		out = append(out, s)
	}
	return out, nil
}

// ScheduleNow triggers a replication job immediately, outside its schedule.
// API: POST /nodes/{node}/replication/{id}/schedule_now
func (ops *Operations) ScheduleNow(node, id string) (string, error) {
	if node == "" || id == "" {
		return "", fmt.Errorf("both a node and a job ID are required")
	}
	resp, err := ops.client.Post(
		fmt.Sprintf("/nodes/%s/replication/%s/schedule_now", node, id), nil)
	if err != nil {
		return "", permHint("trigger replication job "+id, err)
	}
	upid, _ := resp["data"].(string)
	ops.logger.Infof("Triggered replication job %s on %s, task: %s", id, node, upid)
	return upid, nil
}

// ValidateJobID checks the "<vmid>-<number>" form the API requires.
//
// The API pattern is [1-9][0-9]{2,8}-\d{1,9}, so the VMID part must be at least
// three digits. Checking here turns a regex rejection into an explanation.
func ValidateJobID(id string) error {
	if id == "" {
		return fmt.Errorf("a job ID is required, in the form <vmid>-<number>, e.g. 100-0")
	}
	vmid, num, found := strings.Cut(id, "-")
	if !found {
		// Don't echo the input back as the example — for a non-numeric id that
		// would suggest something equally invalid.
		return fmt.Errorf("job ID %q must be <vmid>-<number>, e.g. 100-0", id)
	}
	if len(vmid) < 3 || len(vmid) > 9 {
		return fmt.Errorf("job ID %q: the VMID part must be 3-9 digits", id)
	}
	if vmid[0] == '0' {
		return fmt.Errorf("job ID %q: the VMID part cannot start with 0", id)
	}
	for _, part := range []string{vmid, num} {
		if part == "" {
			return fmt.Errorf("job ID %q must be <vmid>-<number>, e.g. 100-0", id)
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return fmt.Errorf("job ID %q must contain only digits and one '-'", id)
			}
		}
	}
	if len(num) > 9 {
		return fmt.Errorf("job ID %q: the job number is too long", id)
	}
	return nil
}

func applyJobFields(data url.Values, job *Job) {
	if job.Schedule != "" {
		data.Set("schedule", job.Schedule)
	}
	if job.Comment != "" {
		data.Set("comment", job.Comment)
	}
	if job.Rate > 0 {
		data.Set("rate", fmt.Sprintf("%g", job.Rate))
	}
	if job.Disable {
		data.Set("disable", "1")
	}
	if job.RemoveJob != "" {
		data.Set("remove_job", job.RemoveJob)
	}
}

func jobFromMap(m map[string]interface{}) *Job {
	j := &Job{}
	j.ID, _ = m["id"].(string)
	j.Type, _ = m["type"].(string)
	j.Source, _ = m["source"].(string)
	j.Target, _ = m["target"].(string)
	j.Schedule, _ = m["schedule"].(string)
	j.Comment, _ = m["comment"].(string)
	j.RemoveJob, _ = m["remove_job"].(string)
	j.Rate, _ = m["rate"].(float64)
	switch v := m["disable"].(type) {
	case float64:
		j.Disable = v != 0
	case bool:
		j.Disable = v
	case string:
		j.Disable = v == "1"
	}
	return j
}
