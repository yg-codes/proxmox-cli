package apt

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/yg-codes/proxmox-cli/pkg/api"
)

// Operations handles APT repository operations
type Operations struct {
	client *api.Client
	logger *logrus.Logger
}

// NewOperations creates a new APT operations instance
func NewOperations(client *api.Client, logger *logrus.Logger) *Operations {
	if logger == nil {
		logger = logrus.New()
	}

	return &Operations{
		client: client,
		logger: logger,
	}
}

// GetRepositories lists configured repositories and standard handles
// API: GET /nodes/{node}/apt/repositories
func (ops *Operations) GetRepositories(nodeName string) (*Repositories, error) {
	ops.logger.Debugf("Fetching apt repositories for node: %s", nodeName)

	path := fmt.Sprintf("/nodes/%s/apt/repositories", nodeName)
	resp, err := ops.client.Get(path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get apt repositories for node %s: %w", nodeName, err)
	}

	repos := parseRepositories(nodeName, resp)
	ops.logger.Infof("Found %d repo files and %d standard handles on node %s",
		len(repos.Files), len(repos.StandardRepos), nodeName)
	return repos, nil
}

// AddRepository adds a standard repository by handle. If the repository is
// already configured but disabled, the API enables it instead.
// API: POST /nodes/{node}/apt/repositories
func (ops *Operations) AddRepository(nodeName, handle, digest string) error {
	ops.logger.Infof("Adding standard repository %q on node %s", handle, nodeName)

	path := fmt.Sprintf("/nodes/%s/apt/repositories", nodeName)
	data := url.Values{"handle": {handle}}
	if digest != "" {
		data.Set("digest", digest)
	}

	if _, err := ops.client.Post(path, data); err != nil {
		return fmt.Errorf("failed to add repository %q on node %s: %w", handle, nodeName, err)
	}

	ops.logger.Infof("✅ Added/enabled standard repository %q on node %s", handle, nodeName)
	return nil
}

// SetRepositoryEnabled enables or disables the repository entry at path+index.
// API: PUT /nodes/{node}/apt/repositories
func (ops *Operations) SetRepositoryEnabled(nodeName, filePath string, index int, enabled bool, digest string) error {
	action := "enable"
	if !enabled {
		action = "disable"
	}
	ops.logger.Infof("%sing repository %s index %d on node %s", action, filePath, index, nodeName)

	path := fmt.Sprintf("/nodes/%s/apt/repositories", nodeName)
	enabledValue := "0"
	if enabled {
		enabledValue = "1"
	}
	data := url.Values{
		"path":    {filePath},
		"index":   {strconv.Itoa(index)},
		"enabled": {enabledValue},
	}
	if digest != "" {
		data.Set("digest", digest)
	}

	if _, err := ops.client.Put(path, data); err != nil {
		return fmt.Errorf("failed to %s repository %s index %d on node %s: %w",
			action, filePath, index, nodeName, err)
	}

	ops.logger.Infof("✅ %sd repository %s index %d on node %s", action, filePath, index, nodeName)
	return nil
}

// ResolveHandle maps a standard repository handle to the configured repository
// entry it refers to, so enable/disable can address it without knowing the
// file layout. Derived from how Proxmox lays out its own repo files:
//
//   - PVE handles (enterprise, no-subscription, test) carry a component equal
//     to the handle or "pve-"+handle (live: pve-enterprise, pve-no-subscription).
//   - Ceph handles (ceph-<version>-<suffix>) point at an entry whose URI
//     contains "ceph-<version>" and whose component is <suffix> (live:
//     URI .../ceph-tentacle + component no-subscription).
//
// Unlike the server's own standard-repo status (which misses mirror URIs),
// this matches the actual configured entries, so a repo served through a
// mirror can still be toggled by handle.
//
// Ambiguity (more than one entry matches) is an error listing the candidates —
// guessing between them would toggle the wrong repository.
func ResolveHandle(repos *Repositories, handle string) (*Repository, error) {
	var matches []*Repository
	for _, f := range repos.Files {
		for _, r := range f.Repos {
			if handleMatches(r, handle) {
				matches = append(matches, r)
			}
		}
	}

	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return nil, fmt.Errorf("handle %q has no configured repository entry on node %s; if it is a standard handle, add it first with 'repos add --handle %s'",
			handle, repos.Node, handle)
	default:
		var b strings.Builder
		for _, m := range matches {
			fmt.Fprintf(&b, "\n  %s index %d", m.FilePath, m.Index)
		}
		return nil, fmt.Errorf("handle %q matches %d repository entries, refusing to pick one — use --path/--index instead:%s",
			handle, len(matches), b.String())
	}
}

// PVE 8 (bookworm) wrote the Test channel component without the hyphen
// ("pvetest"); trixie standardized on "pve-test". Upgraded hosts keep the
// legacy spelling until an API write rewrites the sources file, so both must
// resolve. Mirrors upstream proxmox-apt's extra_detect_components, which
// only exists for the Test channel.
var legacyPVEComponents = map[string]string{"test": "pvetest"}

func handleMatches(r *Repository, handle string) bool {
	if strings.HasPrefix(handle, "ceph-") {
		// ceph-<version>-<suffix>, e.g. ceph-tentacle-no-subscription
		rest := strings.TrimPrefix(handle, "ceph-")
		i := strings.Index(rest, "-")
		if i <= 0 {
			return false
		}
		version, suffix := rest[:i], rest[i+1:]

		uriHit := false
		for _, u := range r.URIs {
			if strings.Contains(u, "ceph-"+version) {
				uriHit = true
				break
			}
		}
		if !uriHit {
			return false
		}
		for _, c := range r.Components {
			if c == suffix || c == "ceph-"+suffix {
				return true
			}
		}
		return false
	}

	// PVE repo components are always "pve-<handle>"; a bare component equal to
	// the handle belongs to a Ceph repo (no-subscription/enterprise/test) and
	// must not match, or the pve and ceph entries would collide — exactly what
	// happens on nodes with both configured.
	isCeph := false
	for _, u := range r.URIs {
		if strings.Contains(u, "/ceph") {
			isCeph = true
			break
		}
	}
	for _, c := range r.Components {
		if c == "pve-"+handle {
			return true
		}
		if c == handle && !isCeph {
			return true
		}
		if legacy, ok := legacyPVEComponents[handle]; ok && c == legacy {
			return true
		}
	}
	return false
}

// parseRepositories converts the unwrapped GET response into Repositories.
func parseRepositories(nodeName string, resp map[string]interface{}) *Repositories {
	repos := &Repositories{Node: nodeName}

	if digest, ok := resp["digest"].(string); ok {
		repos.Digest = digest
	}

	if errs, ok := resp["errors"].([]interface{}); ok {
		for _, e := range errs {
			if eMap, ok := e.(map[string]interface{}); ok {
				path, _ := eMap["path"].(string)
				msg, _ := eMap["error"].(string)
				if path != "" || msg != "" {
					repos.Errors = append(repos.Errors, fmt.Sprintf("%s: %s", path, msg))
				}
			}
		}
	}

	if infos, ok := resp["infos"].([]interface{}); ok {
		for _, i := range infos {
			if iMap, ok := i.(map[string]interface{}); ok {
				info := &Info{}
				info.Kind, _ = iMap["kind"].(string)
				info.Path, _ = iMap["path"].(string)
				info.Property, _ = iMap["property"].(string)
				info.Message, _ = iMap["message"].(string)
				if idx, ok := iMap["index"].(float64); ok {
					info.Index = int(idx)
				}
				repos.Infos = append(repos.Infos, info)
			}
		}
	}

	if files, ok := resp["files"].([]interface{}); ok {
		for _, f := range files {
			if fMap, ok := f.(map[string]interface{}); ok {
				file := &RepoFile{}
				file.Path, _ = fMap["path"].(string)
				file.FileType, _ = fMap["file-type"].(string)

				if entries, ok := fMap["repositories"].([]interface{}); ok {
					for idx, e := range entries {
						if eMap, ok := e.(map[string]interface{}); ok {
							entry := parseRepositoryEntry(file.Path, idx, eMap)
							file.Repos = append(file.Repos, entry)
						}
					}
				}
				repos.Files = append(repos.Files, file)
			}
		}
	}

	if standards, ok := resp["standard-repos"].([]interface{}); ok {
		for _, s := range standards {
			if sMap, ok := s.(map[string]interface{}); ok {
				std := &StandardRepo{}
				std.Handle, _ = sMap["handle"].(string)
				std.Name, _ = sMap["name"].(string)
				std.Description, _ = sMap["description"].(string)
				// status is only present when the repo is configured:
				// 0 = configured but disabled, 1 = configured and enabled
				if status, ok := sMap["status"].(float64); ok {
					std.Configured = true
					std.Enabled = status == 1
				}
				repos.StandardRepos = append(repos.StandardRepos, std)
			}
		}
	}

	return repos
}

func parseRepositoryEntry(filePath string, index int, eMap map[string]interface{}) *Repository {
	entry := &Repository{
		FilePath: filePath,
		Index:    index,
	}

	// "Enabled" is 0/1 in current PVE; accept a bare boolean as well
	switch v := eMap["Enabled"].(type) {
	case bool:
		entry.Enabled = v
	case float64:
		entry.Enabled = v == 1
	}

	entry.Components = stringSlice(eMap, "Components", "Component")
	entry.Suites = stringSlice(eMap, "Suites", "Suite")
	entry.URIs = stringSlice(eMap, "URIs", "URI")
	entry.Types = stringSlice(eMap, "Types", "Types")
	entry.Comment, _ = eMap["Comment"].(string)

	return entry
}

// stringSlice reads the first present key among aliases as a []string.
func stringSlice(m map[string]interface{}, keys ...string) []string {
	for _, key := range keys {
		if raw, ok := m[key].([]interface{}); ok {
			var out []string
			for _, v := range raw {
				if s, ok := v.(string); ok {
					out = append(out, s)
				}
			}
			return out
		}
		// one-line .list style entries may use a plain string
		if s, ok := m[key].(string); ok && s != "" {
			return []string{s}
		}
	}
	return nil
}
