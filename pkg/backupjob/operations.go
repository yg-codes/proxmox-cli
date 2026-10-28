// Package backupjob manages Proxmox vzdump backup jobs.
//
// Backup jobs are the scheduled, cluster-wide vzdump definitions stored in
// jobs.cfg — distinct from the one-shot backups and the backup *files* handled
// by pkg/backup. The API surface is /cluster/backup (CRUD) plus
// /cluster/backup-info/not-backed-up (guests covered by no job).
//
// Two properties of the API that shape this package:
//
//   - There is no digest parameter on any endpoint. jobs.cfg is protected by
//     cfs file locking server-side, not optimistic locking, so update races
//     are the server's problem and fabricating a digest here would be wrong.
//   - Guest selection is the classic all / vmid / pool trio (plus exclude),
//     with no include/exclude filter mode on the create/update endpoints.
package backupjob

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/yg-codes/proxmox-cli/pkg/api"
)

// Job is one vzdump backup job from /cluster/backup.
//
// Fields left at their zero value are omitted from create/update requests, so
// a Job doubles as a partial-update payload. Enabled is a pointer for the same
// reason: nil means "leave unchanged", which a plain bool cannot express.
type Job struct {
	ID   string
	Type string // read-only, always "vzdump"

	// Guest selection: exactly one of All/VMID/Pool (enforced by
	// ValidateSelection). Exclude is only meaningful alongside All.
	All     bool
	VMID    string // pve-vmid-list, e.g. "4001,4002" — IDs only, no names
	Pool    string
	Exclude string // pve-vmid-list; assumes All
	// ExcludePath is a JSON array of shell globs, sent as repeated form keys.
	ExcludePath []string

	Storage  string
	Schedule string // pve-calendar-event, e.g. "sat 02:30"
	// Enabled is tri-state: nil = leave unchanged / server default.
	Enabled    *bool
	Mode       string // snapshot|suspend|stop
	Compress   string // 0|1|gzip|lzo|zstd
	Comment    string
	NotesTempl string // notes-template

	// Notification settings. mailto/mailnotification are deprecated in favor
	// of the notification system, but remain valid API input.
	Mailto           string
	MailNotification string // always|failure
	NotificationMode string // auto|legacy-sendmail|notification-system
	PruneBackups     string // property string, e.g. "keep-last=7" — passthrough
	Performance      string // property string — passthrough
	Fleecing         string // property string — passthrough
	PBSChangeDet     string // pbs-change-detection-mode: legacy|data|metadata
	Protected        bool   // mark resulting backups protected
	Remove           bool   // prune older backups per prune-backups
	RepeatMissed     bool   // run ASAP if a scheduled run was missed
	Node             string // only run when executed on this node
	BWLimit          int    // KiB/s; >0 sends
	IOnice           int    // BFQ IO priority; >0 sends
	Zstd             int    // zstd threads; >0 sends

	// NextRun is the computed next-run epoch from a list response (read-only;
	// 0 means unknown/disabled).
	NextRun int64
}

// Guest is one entry of the not-backed-up report.
type Guest struct {
	VMID int
	Name string
	Type string // "qemu" | "lxc"
}

// Operations handles backup job management.
type Operations struct {
	client *api.Client
	logger *logrus.Logger
}

// NewOperations creates a backup job operations handler.
func NewOperations(client *api.Client, logger *logrus.Logger) *Operations {
	if logger == nil {
		logger = logrus.New()
	}
	return &Operations{client: client, logger: logger}
}

// permHint annotates a 403. Reads need Sys.Audit on / and writes need
// Sys.Modify on / — neither is included in PVEVMAdmin or PVEAdmin, so a
// VM-scoped token cannot manage jobs at all. bwlimit/ionice/performance are
// additionally restricted to Sys.Modify holders even where the endpoint
// itself allowed the call, and tmpdir/dumpdir/script to root@pam (the CLI
// does not expose those three).
func permHint(action string, err error) error {
	var apiErr *api.ProxmoxAPIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusForbidden {
		return fmt.Errorf(
			"insufficient permission to %s: backup job reads need Sys.Audit on / and "+
				"changes need Sys.Modify on /.\n"+
				"  PVEVMAdmin and PVEAdmin include neither; grant PVEAuditor for reads or\n"+
				"  PVESysAdmin for changes: pveum aclmod / -token '<user>@<realm>!<token>' -role <role>\n"+
				"  (bwlimit/ionice/performance also require Sys.Modify; tmpdir/dumpdir/script are root@pam only)",
			action)
	}
	return err
}

// ListJobs returns the configured backup jobs.
// API: GET /cluster/backup
func (ops *Operations) ListJobs() ([]*Job, error) {
	resp, err := ops.client.Get("/cluster/backup", nil)
	if err != nil {
		return nil, permHint("list backup jobs", err)
	}
	return jobsFromList(resp)
}

// GetJob reads one backup job.
// API: GET /cluster/backup/{id}
func (ops *Operations) GetJob(id string) (*Job, error) {
	if id == "" {
		return nil, fmt.Errorf("a job ID is required")
	}
	resp, err := ops.client.Get("/cluster/backup/"+id, nil)
	if err != nil {
		return nil, permHint("read backup job "+id, err)
	}
	// api.Client unwraps a single-object "data" field, so resp is the object.
	if len(resp) > 0 {
		return jobFromMap(resp), nil
	}
	return nil, fmt.Errorf("backup job %s not found", id)
}

// CreateJob creates a backup job.
//
// The ID is optional on the server side (it generates one from a UUID when
// omitted); a caller-supplied ID must be a pve-configid.
// API: POST /cluster/backup
func (ops *Operations) CreateJob(job *Job) error {
	if job == nil {
		return fmt.Errorf("a job definition is required")
	}
	if job.Schedule == "" {
		return fmt.Errorf("--schedule must be specified")
	}
	if job.Storage == "" {
		return fmt.Errorf("--storage must be specified")
	}
	if err := ValidateSelection(job); err != nil {
		return err
	}
	if job.ID != "" {
		if err := ValidateJobID(job.ID); err != nil {
			return err
		}
	}

	data := url.Values{}
	applyJobFields(data, job)

	if _, err := ops.client.Post("/cluster/backup", data); err != nil {
		return permHint("create backup job "+job.ID, err)
	}
	id := job.ID
	if id == "" {
		id = "(server-generated)"
	}
	ops.logger.Infof("Created backup job %s", id)
	return nil
}

// UpdateJob changes an existing backup job.
//
// deleteKeys resets settings to their server default, which is the only way
// to clear a boolean beyond sending its zero value explicitly.
// API: PUT /cluster/backup/{id}
func (ops *Operations) UpdateJob(id string, job *Job, deleteKeys []string) error {
	if id == "" {
		return fmt.Errorf("a job ID is required")
	}
	if job == nil {
		job = &Job{}
	}

	data := url.Values{}
	applyJobFields(data, job)
	if len(deleteKeys) > 0 {
		for _, key := range deleteKeys {
			if err := validateDeletableKey(key); err != nil {
				return err
			}
		}
		data.Set("delete", strings.Join(deleteKeys, ","))
	}
	if len(data) == 0 {
		return fmt.Errorf("nothing to update: pass at least one field to change or delete")
	}

	if _, err := ops.client.Put("/cluster/backup/"+id, data); err != nil {
		return permHint("update backup job "+id, err)
	}
	ops.logger.Infof("Updated backup job %s", id)
	return nil
}

// DeleteJob removes a backup job definition. Existing backup files are
// unaffected; only the schedule is deleted.
// API: DELETE /cluster/backup/{id}
func (ops *Operations) DeleteJob(id string) error {
	if id == "" {
		return fmt.Errorf("a job ID is required")
	}
	if _, err := ops.client.Delete("/cluster/backup/" + id); err != nil {
		return permHint("delete backup job "+id, err)
	}
	ops.logger.Infof("Deleted backup job %s", id)
	return nil
}

// NotCoveredGuests returns guests not covered by any backup job.
// API: GET /cluster/backup-info/not-backed-up
func (ops *Operations) NotCoveredGuests() ([]*Guest, error) {
	resp, err := ops.client.Get("/cluster/backup-info/not-backed-up", nil)
	if err != nil {
		return nil, permHint("list guests not covered by a backup job", err)
	}
	items, ok := resp["items"].([]interface{})
	if !ok {
		return nil, nil
	}
	guests := make([]*Guest, 0, len(items))
	for _, raw := range items {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		g := &Guest{}
		if v, ok := m["vmid"].(float64); ok {
			g.VMID = int(v)
		}
		g.Name, _ = m["name"].(string)
		g.Type, _ = m["type"].(string)
		guests = append(guests, g)
	}
	return guests, nil
}

// ValidateJobID checks the pve-configid form the API requires for a
// caller-supplied job ID. Server-generated IDs are UUID-derived, so anything
// short and readable that a human types must still pass this pattern.
func ValidateJobID(id string) error {
	if id == "" {
		return fmt.Errorf("a job ID is required, e.g. weekly-vms")
	}
	if len(id) > 40 {
		return fmt.Errorf("job ID %q: too long (max 40 characters)", id)
	}
	// pve-configid: starts with a letter, then letters/digits/underscore/hyphen.
	for i, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			// always fine
		case i > 0 && (r >= '0' && r <= '9' || r == '_' || r == '-'):
			// fine after the first character
		default:
			return fmt.Errorf("job ID %q: must start with a letter and contain only letters, digits, '_' and '-'", id)
		}
	}
	return nil
}

// validateDeletableKey guards the --delete list against keys the API would
// reject or that would corrupt the job (id cannot be deleted; unknown keys
// fail with a pattern error that names no useful remedy).
func validateDeletableKey(key string) error {
	if key == "" {
		return fmt.Errorf("--delete contains an empty setting name")
	}
	if key == "id" {
		return fmt.Errorf("the job ID cannot be deleted; delete the job itself instead")
	}
	if key == "schedule" {
		return fmt.Errorf("setting %q cannot be deleted (a job without a schedule is invalid); set a new --schedule instead", key)
	}
	for _, r := range key {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return fmt.Errorf("setting %q is not a valid backup job property name", key)
		}
	}
	return nil
}

// ValidateVMIDList enforces the pve-vmid-list form: non-empty, comma-separated
// positive integers. VM *names* and ranges are not valid here — unlike the CLI
// selectors, this value is stored server-side and expanded at job runtime, so
// a hyphenated name would silently select nothing.
func ValidateVMIDList(s string) error {
	if s == "" {
		return fmt.Errorf("a VMID list is required, e.g. 4001,4002")
	}
	for _, part := range strings.Split(s, ",") {
		if part == "" {
			return fmt.Errorf("VMID list %q: empty element (check for a trailing comma)", s)
		}
		n, err := strconv.Atoi(part)
		if err != nil || n <= 0 {
			return fmt.Errorf("VMID list %q: %q is not a VMID — backup jobs take numeric guest IDs, not names",
				s, part)
		}
	}
	return nil
}

// ValidateSelection enforces exactly one guest selection among all/vmid/pool,
// and that exclude only appears with all.
func ValidateSelection(job *Job) error {
	chosen := 0
	if job.All {
		chosen++
	}
	if job.VMID != "" {
		chosen++
		if err := ValidateVMIDList(job.VMID); err != nil {
			return err
		}
	}
	if job.Pool != "" {
		chosen++
	}
	if chosen != 1 {
		return fmt.Errorf("exactly one guest selection is required: --all, --vmid or --pool")
	}
	if job.Exclude != "" && !job.All {
		return fmt.Errorf("--exclude requires --all (it excludes guests from an all-guests backup)")
	}
	if job.Exclude != "" {
		if err := ValidateVMIDList(job.Exclude); err != nil {
			return fmt.Errorf("--exclude: %v", err)
		}
	}
	return nil
}

func applyJobFields(data url.Values, job *Job) {
	if job.ID != "" {
		data.Set("id", job.ID)
	}
	if job.Schedule != "" {
		data.Set("schedule", job.Schedule)
	}
	if job.Storage != "" {
		data.Set("storage", job.Storage)
	}
	if job.Enabled != nil && *job.Enabled {
		data.Set("enabled", "1")
	} else if job.Enabled != nil {
		data.Set("enabled", "0")
	}
	if job.All {
		data.Set("all", "1")
	}
	if job.VMID != "" {
		data.Set("vmid", job.VMID)
	}
	if job.Pool != "" {
		data.Set("pool", job.Pool)
	}
	if job.Exclude != "" {
		data.Set("exclude", job.Exclude)
	}
	for _, p := range job.ExcludePath {
		data.Add("exclude-path", p)
	}
	if job.Mode != "" {
		data.Set("mode", job.Mode)
	}
	if job.Compress != "" {
		data.Set("compress", job.Compress)
	}
	if job.Comment != "" {
		data.Set("comment", job.Comment)
	}
	if job.NotesTempl != "" {
		data.Set("notes-template", job.NotesTempl)
	}
	if job.Mailto != "" {
		data.Set("mailto", job.Mailto)
	}
	if job.MailNotification != "" {
		data.Set("mailnotification", job.MailNotification)
	}
	if job.NotificationMode != "" {
		data.Set("notification-mode", job.NotificationMode)
	}
	if job.PruneBackups != "" {
		data.Set("prune-backups", job.PruneBackups)
	}
	if job.Performance != "" {
		data.Set("performance", job.Performance)
	}
	if job.Fleecing != "" {
		data.Set("fleecing", job.Fleecing)
	}
	if job.PBSChangeDet != "" {
		data.Set("pbs-change-detection-mode", job.PBSChangeDet)
	}
	if job.Protected {
		data.Set("protected", "1")
	}
	if job.Remove {
		data.Set("remove", "1")
	}
	if job.RepeatMissed {
		data.Set("repeat-missed", "1")
	}
	if job.Node != "" {
		data.Set("node", job.Node)
	}
	if job.BWLimit > 0 {
		data.Set("bwlimit", strconv.Itoa(job.BWLimit))
	}
	if job.IOnice > 0 {
		data.Set("ionice", strconv.Itoa(job.IOnice))
	}
	if job.Zstd > 0 {
		data.Set("zstd", strconv.Itoa(job.Zstd))
	}
	// Deliberately no digest: no backup-job endpoint accepts one (jobs.cfg is
	// guarded by cfs locking, not optimistic locking).
}

func jobsFromList(resp map[string]interface{}) ([]*Job, error) {
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

func jobFromMap(m map[string]interface{}) *Job {
	j := &Job{}
	j.ID, _ = m["id"].(string)
	j.Type, _ = m["type"].(string)
	j.Schedule, _ = m["schedule"].(string)
	j.VMID, _ = m["vmid"].(string)
	j.Pool, _ = m["pool"].(string)
	j.Exclude, _ = m["exclude"].(string)
	if paths, ok := m["exclude-path"].([]interface{}); ok {
		for _, p := range paths {
			if s, ok := p.(string); ok {
				j.ExcludePath = append(j.ExcludePath, s)
			}
		}
	}
	j.Storage, _ = m["storage"].(string)
	j.Mode, _ = m["mode"].(string)
	j.Compress, _ = m["compress"].(string)
	j.Comment, _ = m["comment"].(string)
	j.NotesTempl, _ = m["notes-template"].(string)
	j.Mailto, _ = m["mailto"].(string)
	j.MailNotification, _ = m["mailnotification"].(string)
	j.NotificationMode, _ = m["notification-mode"].(string)
	j.PruneBackups, _ = m["prune-backups"].(string)
	j.Performance, _ = m["performance"].(string)
	j.Fleecing, _ = m["fleecing"].(string)
	j.PBSChangeDet, _ = m["pbs-change-detection-mode"].(string)
	j.Node, _ = m["node"].(string)
	j.BWLimit = intFromMap(m, "bwlimit")
	j.IOnice = intFromMap(m, "ionice")
	j.Zstd = intFromMap(m, "zstd")
	j.All = truthyFromMap(m, "all")
	j.Protected = truthyFromMap(m, "protected")
	j.Remove = truthyFromMap(m, "remove")
	j.RepeatMissed = truthyFromMap(m, "repeat-missed")
	if e := truthyFromMap(m, "enabled"); e {
		j.Enabled = &e
	} else if _, present := m["enabled"]; present {
		f := false
		j.Enabled = &f
	}
	if v := intFromMap(m, "next-run"); v > 0 {
		j.NextRun = int64(v)
	}
	return j
}

// truthyFromMap handles the three shapes Proxmox uses for booleans across
// endpoints: JSON true/false, numeric 1/0, and string "1"/"0".
func truthyFromMap(m map[string]interface{}, key string) bool {
	switch v := m[key].(type) {
	case bool:
		return v
	case float64:
		return v != 0
	case int:
		return v != 0
	case string:
		return v == "1" || v == "true"
	}
	return false
}

func intFromMap(m map[string]interface{}, key string) int {
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	return 0
}
