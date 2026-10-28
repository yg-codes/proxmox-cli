package access

// User represents a Proxmox API user entry from /access/users.
type User struct {
	Userid   string
	Comment  string
	Enabled  bool
	Email    string
	Expire   int64 // seconds since epoch; 0 means no expiration
	Groups   string
	FullName string
}

// APIToken represents an API token owned by a user. The Value field is only
// populated at creation time — the secret is returned once and cannot be
// retrieved again over the API.
type APIToken struct {
	Userid  string
	Tokenid string // token name (without the user@realm! prefix)
	Comment string
	Privsep bool   // true = privilege separation (default); false = full privileges
	Expire  int64  // seconds since epoch; 0 means no expiration
	Value   string // only set on creation; empty on read-back
}

// FullID returns the canonical API token identifier "user@realm!tokenid".
func (t *APIToken) FullID() string {
	return t.Userid + "!" + t.Tokenid
}

// ACLEntry represents one row of /access/acl: a set of subjects (users/tokens/
// groups) granted a set of roles on a path.
type ACLEntry struct {
	Path      string
	Roles     string // comma-separated, as returned by the API
	Users     string
	Tokens    string
	Groups    string
	Propagate bool
}
