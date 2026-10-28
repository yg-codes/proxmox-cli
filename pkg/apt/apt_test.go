package apt

import (
	"encoding/json"
	"strings"
	"testing"
)

// fixture mirrors the live GET /nodes/{node}/apt/repositories response from
// pve-node3 (2026-08-26): deb822 .sources files, 0/1 Enabled integers,
// mirror URIs, status only on configured standard repos.
const fixture = `{
  "digest": "4d587a0f58b55c010e66149576a353d90314347df9c97086133137315480d3a4",
  "errors": [],
  "infos": [
    {"kind": "origin", "path": "/etc/apt/sources.list.d/proxmox.sources", "index": 0, "message": "Proxmox"},
    {"kind": "warning", "path": "/etc/apt/sources.list.d/puppet7-release.sources", "index": 0, "property": "Suites", "message": "old suite 'bookworm' configured!"}
  ],
  "standard-repos": [
    {"handle": "enterprise", "name": "Enterprise", "description": "default stable repo", "status": 0},
    {"handle": "no-subscription", "name": "No-Subscription", "description": "testing and non-production", "status": 1},
    {"handle": "test", "name": "Test", "description": "latest packages"},
    {"handle": "ceph-tentacle-no-subscription", "name": "Ceph Tentacle No-Subscription", "description": "non-production ceph"}
  ],
  "files": [
    {"path": "/etc/apt/sources.list", "file-type": "list", "repositories": []},
    {"path": "/etc/apt/sources.list.d/proxmox.sources", "file-type": "sources", "repositories": [
      {"Enabled": 1, "Components": ["pve-no-subscription"], "Suites": ["trixie"], "URIs": ["http://pve.example.com/apt/sit/download.proxmox.com/debian/pve/"], "Types": ["deb"], "FileType": "sources"}
    ]},
    {"path": "/etc/apt/sources.list.d/pve-enterprise.sources", "file-type": "sources", "repositories": [
      {"Enabled": 0, "Components": ["pve-enterprise"], "Suites": ["trixie"], "URIs": ["https://enterprise.proxmox.com/debian/pve"], "Types": ["deb"], "FileType": "sources"}
    ]},
    {"path": "/etc/apt/sources.list.d/debian.sources", "file-type": "sources", "repositories": [
      {"Enabled": 1, "Components": ["main", "contrib"], "Suites": ["trixie", "trixie-updates"], "URIs": ["http://repo/debian/"], "FileType": "sources"},
      {"Enabled": 1, "Components": ["main"], "Suites": ["trixie-security"], "URIs": ["http://repo/security/"], "FileType": "sources"}
    ]},
    {"path": "/etc/apt/sources.list.d/ceph.sources", "file-type": "sources", "repositories": [
      {"Enabled": 0, "Components": ["no-subscription"], "Suites": ["trixie"], "URIs": ["http://pve.example.com/apt/sit/download.proxmox.com/debian/ceph-tentacle"], "FileType": "sources"}
    ]},
    {"path": "/etc/apt/sources.list.d/pvetest.sources", "file-type": "sources", "repositories": [
      {"Enabled": 1, "Components": ["pvetest"], "Suites": ["trixie"], "URIs": ["http://download.proxmox.com/debian/pve"], "FileType": "sources"}
    ]}
  ]
}`

func parseFixture(t *testing.T) *Repositories {
	t.Helper()
	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(fixture), &resp); err != nil {
		t.Fatalf("failed to parse fixture: %v", err)
	}
	return parseRepositories("test-node", resp)
}

func TestParseRepositories(t *testing.T) {
	repos := parseFixture(t)

	if repos.Digest != "4d587a0f58b55c010e66149576a353d90314347df9c97086133137315480d3a4" {
		t.Errorf("digest = %q", repos.Digest)
	}
	if len(repos.Files) != 6 {
		t.Fatalf("files = %d, want 6", len(repos.Files))
	}

	// Index must be the position within the file — debian.sources has two
	// entries at indexes 0 and 1 (the PUT index refers to this position).
	var debian *RepoFile
	for _, f := range repos.Files {
		if f.Path == "/etc/apt/sources.list.d/debian.sources" {
			debian = f
		}
	}
	if debian == nil {
		t.Fatal("debian.sources not found")
	}
	if len(debian.Repos) != 2 {
		t.Fatalf("debian repos = %d, want 2", len(debian.Repos))
	}
	for i, r := range debian.Repos {
		if r.Index != i {
			t.Errorf("debian entry %d has index %d", i, r.Index)
		}
		if !r.Enabled {
			t.Errorf("debian entry %d should be enabled", i)
		}
	}

	// Enabled 0 parses as disabled (the enterprise repo on this fixture)
	ent := mustFind(t, repos, "/etc/apt/sources.list.d/pve-enterprise.sources", 0)
	if ent.Enabled {
		t.Error("enterprise entry should be disabled")
	}
	if len(ent.Components) != 1 || ent.Components[0] != "pve-enterprise" {
		t.Errorf("enterprise components = %v", ent.Components)
	}

	// Standard repos: status present ⇒ configured; absent ⇒ not configured.
	// The ceph mirror URI means the server reports ceph-tentacle-no-subscription
	// as unconfigured even though an entry exists — the parser must reflect
	// exactly what the server said, not what the files look like.
	byHandle := map[string]*StandardRepo{}
	for _, s := range repos.StandardRepos {
		byHandle[s.Handle] = s
	}
	if s := byHandle["enterprise"]; !s.Configured || s.Enabled {
		t.Errorf("enterprise = configured:%v enabled:%v, want true/false", s.Configured, s.Enabled)
	}
	if s := byHandle["no-subscription"]; !s.Configured || !s.Enabled {
		t.Errorf("no-subscription = configured:%v enabled:%v, want true/true", s.Configured, s.Enabled)
	}
	if s := byHandle["test"]; s.Configured {
		t.Errorf("test should not be configured")
	}
	if s := byHandle["ceph-tentacle-no-subscription"]; s.Configured {
		t.Errorf("mirror-URI ceph repo should read as not configured (server status absent)")
	}

	// Diagnostics: one warning
	warnings := 0
	for _, info := range repos.Infos {
		if info.Kind == "warning" {
			warnings++
		}
	}
	if warnings != 1 {
		t.Errorf("warnings = %d, want 1", warnings)
	}
}

func mustFind(t *testing.T, repos *Repositories, path string, index int) *Repository {
	t.Helper()
	for _, r := range ResolveAll(repos) {
		if r.FilePath == path && r.Index == index {
			return r
		}
	}
	t.Fatalf("entry %s#%d not found", path, index)
	return nil
}

// ResolveAll flattens all entries (test helper).
func ResolveAll(repos *Repositories) []*Repository {
	var out []*Repository
	for _, f := range repos.Files {
		out = append(out, f.Repos...)
	}
	return out
}

func TestResolveHandle(t *testing.T) {
	repos := parseFixture(t)

	cases := []struct {
		handle  string
		path    string
		index   int
		wantErr bool
		errPart string
	}{
		{"enterprise", "/etc/apt/sources.list.d/pve-enterprise.sources", 0, false, ""},
		{"no-subscription", "/etc/apt/sources.list.d/proxmox.sources", 0, false, ""},
		// The mirror-URI ceph entry resolves by handle even though the
		// server's own standard-repos status calls it unconfigured.
		{"ceph-tentacle-no-subscription", "/etc/apt/sources.list.d/ceph.sources", 0, false, ""},
		// PVE 8 hosts carry the legacy unhyphenated "pvetest" component for
		// the Test channel; both it and the canonical "pve-test" must resolve
		// (upstream proxmox-apt accepts the legacy spelling on detection).
		{"test", "/etc/apt/sources.list.d/pvetest.sources", 0, false, ""},
		{"ceph-squid-no-subscription", "", 0, true, "no configured repository entry"},
	}
	for _, tc := range cases {
		r, err := ResolveHandle(repos, tc.handle)
		if tc.wantErr {
			if err == nil {
				t.Errorf("%s: expected error, got entry %s#%d", tc.handle, r.FilePath, r.Index)
			} else if !strings.Contains(err.Error(), tc.errPart) {
				t.Errorf("%s: error %q does not contain %q", tc.handle, err.Error(), tc.errPart)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: unexpected error: %v", tc.handle, err)
			continue
		}
		if r.FilePath != tc.path || r.Index != tc.index {
			t.Errorf("%s: resolved to %s#%d, want %s#%d", tc.handle, r.FilePath, r.Index, tc.path, tc.index)
		}
	}
}

func TestResolveHandleAmbiguous(t *testing.T) {
	repos := parseFixture(t)

	// Two entries with a pve-test component must be an error listing both,
	// not a silent pick of the first.
	dup := &RepoFile{
		Path:     "/etc/apt/sources.list.d/pve-test-a.sources",
		FileType: "sources",
		Repos: []*Repository{
			{FilePath: "/etc/apt/sources.list.d/pve-test-a.sources", Index: 0, Enabled: false, Components: []string{"pve-test"}},
		},
	}
	existing := &Repository{
		FilePath:   "/etc/apt/sources.list.d/pve-test-b.list",
		Index:      0,
		Enabled:    true,
		Components: []string{"test"},
	}
	repos.Files = append(repos.Files,
		dup,
		&RepoFile{Path: "/etc/apt/sources.list.d/pve-test-b.list", FileType: "list", Repos: []*Repository{existing}},
	)

	_, err := ResolveHandle(repos, "test")
	if err == nil {
		t.Fatal("expected ambiguity error")
	}
	if !strings.Contains(err.Error(), "pve-test-a.sources") || !strings.Contains(err.Error(), "pve-test-b.list") {
		t.Errorf("ambiguity error should list both candidates, got: %v", err)
	}
	if !strings.Contains(err.Error(), "--path/--index") {
		t.Errorf("error should point at --path/--index escape hatch, got: %v", err)
	}
}
