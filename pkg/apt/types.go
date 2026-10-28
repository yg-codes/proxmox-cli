package apt

// Repository represents a single configured APT repository entry (one stanza
// of a .sources file or one line of a .list file on the node).
type Repository struct {
	FilePath   string // absolute path of the containing file
	Index      int    // position within the file, 0-based — the PUT `index` value
	Enabled    bool
	Components []string
	Suites     []string
	URIs       []string
	Types      []string
	Comment    string
}

// RepoFile is one APT source file and its repository entries.
type RepoFile struct {
	Path     string
	FileType string // "list" or "sources"
	Repos    []*Repository
}

// StandardRepo is a repository handle Proxmox knows about and can add for you
// (POST /apt/repositories only accepts these handles — arbitrary URLs cannot
// be added through the API).
type StandardRepo struct {
	Handle      string
	Name        string
	Description string
	Configured  bool // status key present in the API response
	Enabled     bool
}

// Info is a diagnostic message the API attaches to a file or entry.
// kind "warning" entries flag problems (e.g. an outdated suite); "origin"
// entries merely label the repo's vendor.
type Info struct {
	Kind     string
	Path     string
	Index    int
	Property string
	Message  string
}

// Repositories is the full GET /nodes/{node}/apt/repositories payload.
// Digest guards add/enable/disable against concurrent modifications.
type Repositories struct {
	Node          string
	Files         []*RepoFile
	StandardRepos []*StandardRepo
	Infos         []*Info
	Errors        []string
	Digest        string
}
