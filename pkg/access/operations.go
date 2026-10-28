package access

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/yg-codes/proxmox-cli/pkg/api"
)

// Operations handles Proxmox API user/token/ACL management via the /access/*
// REST endpoints. These are the same operations the `pveum` CLI performs;
// pveum is itself a thin wrapper over this API.
//
// Mutations require User.Modify (on /access) and Permissions.Modify (on the
// ACL target path). A normal VM-scoped API token cannot perform these — the
// caller must authenticate as root@pam (or a PVEUserAdmin service account),
// typically via PVE_PASSWORD.
type Operations struct {
	client *api.Client
	logger *logrus.Logger
}

// NewOperations creates a new access-control operations handler.
func NewOperations(client *api.Client, logger *logrus.Logger) *Operations {
	if logger == nil {
		logger = logrus.New()
	}
	return &Operations{client: client, logger: logger}
}

// permHint wraps a permission error (HTTP 403) from an /access/* mutation with
// a clear message naming the required privileges. Non-403 errors pass through.
func permHint(action string, err error) error {
	var apiErr *api.ProxmoxAPIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusForbidden {
		return fmt.Errorf(
			"insufficient permission to %s: this requires User.Modify on /access and "+
				"Permissions.Modify on the target path. Authenticate as root@pve (via "+
				"PVE_PASSWORD) or a PVEUserAdmin service account, not a VM-scoped token",
			action,
		)
	}
	return err
}

// UserExists reports whether a user with the given user@realm id exists.
// API: GET /access/users
func (ops *Operations) UserExists(userid string) (bool, error) {
	resp, err := ops.client.Get("/access/users", nil)
	if err != nil {
		return false, fmt.Errorf("failed to list users: %w", err)
	}
	items, ok := resp["items"].([]interface{})
	if !ok {
		return false, nil
	}
	for _, u := range items {
		if m, ok := u.(map[string]interface{}); ok {
			if id, ok := m["userid"].(string); ok && id == userid {
				return true, nil
			}
		}
	}
	return false, nil
}

// CreateUser creates a user if it does not already exist. Returns whether the
// user was newly created.
// API: POST /access/users
func (ops *Operations) CreateUser(userid, comment string) (created bool, err error) {
	exists, err := ops.UserExists(userid)
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	form := url.Values{}
	form.Set("userid", userid)
	if comment != "" {
		form.Set("comment", comment)
	}
	if _, err := ops.client.Post("/access/users", form); err != nil {
		return false, permHint("create user "+userid, err)
	}
	return true, nil
}

// DeleteUser deletes a user and all its tokens.
// API: DELETE /access/users/{userid}
func (ops *Operations) DeleteUser(userid string) error {
	if _, err := ops.client.Delete("/access/users/" + userid); err != nil {
		return permHint("delete user "+userid, err)
	}
	return nil
}

// CreateToken adds an API token to a user with the given privilege-separation
// setting, returning the token secret. The secret is only ever returned once;
// callers must store it immediately.
// API: POST /access/users/{userid}/token/{tokenid}
func (ops *Operations) CreateToken(userid, tokenid string, privsep bool) (string, error) {
	form := url.Values{}
	if privsep {
		form.Set("privsep", "1")
	} else {
		form.Set("privsep", "0")
	}
	path := fmt.Sprintf("/access/users/%s/token/%s", userid, tokenid)
	resp, err := ops.client.Post(path, form)
	if err != nil {
		return "", permHint("create token "+userid+"!"+tokenid, err)
	}
	value, _ := resp["value"].(string)
	if value == "" {
		return "", fmt.Errorf("token created but no value returned (already exists?)")
	}
	return value, nil
}

// DeleteToken revokes a single token, preserving the user and other tokens.
// API: DELETE /access/users/{userid}/token/{tokenid}
func (ops *Operations) DeleteToken(userid, tokenid string) error {
	path := fmt.Sprintf("/access/users/%s/token/%s", userid, tokenid)
	if _, err := ops.client.Delete(path); err != nil {
		return permHint("revoke token "+userid+"!"+tokenid, err)
	}
	return nil
}

// ListTokens returns the API tokens owned by a user.
// API: GET /access/users/{userid}/token  (index endpoint; no tokenid)
func (ops *Operations) ListTokens(userid string) ([]*APIToken, error) {
	resp, err := ops.client.Get(fmt.Sprintf("/access/users/%s/token", userid), nil)
	if err != nil {
		return nil, permHint("list tokens for "+userid, err)
	}
	items, ok := resp["items"].([]interface{})
	if !ok {
		return nil, nil
	}
	tokens := make([]*APIToken, 0, len(items))
	for _, t := range items {
		m, ok := t.(map[string]interface{})
		if !ok {
			continue
		}
		tok := &APIToken{Userid: userid}
		if v, ok := m["tokenid"].(string); ok {
			tok.Tokenid = v
		}
		if v, ok := m["comment"].(string); ok {
			tok.Comment = v
		}
		if v, ok := m["privsep"].(float64); ok {
			tok.Privsep = v != 0
		} else if v, ok := m["privsep"].(bool); ok {
			tok.Privsep = v
		}
		if v, ok := m["expire"].(float64); ok {
			tok.Expire = int64(v)
		}
		tokens = append(tokens, tok)
	}
	return tokens, nil
}

// UpdateACL grants a role to subjects (tokens/users/groups) on a path. This is
// the REST equivalent of `pveum aclmod <path>`.
// API: PUT /access/acl
func (ops *Operations) UpdateACL(path, tokens, users, roles string, propagate bool) error {
	form := url.Values{}
	form.Set("path", path)
	if roles != "" {
		form.Set("roles", roles)
	}
	if tokens != "" {
		form.Set("tokens", tokens)
	}
	if users != "" {
		form.Set("users", users)
	}
	if propagate {
		form.Set("propagate", "1")
	} else {
		form.Set("propagate", "0")
	}
	if _, err := ops.client.Put("/access/acl", form); err != nil {
		return permHint("update ACL on "+path, err)
	}
	return nil
}

// ListACL returns the access control list.
// API: GET /access/acl
func (ops *Operations) ListACL() ([]*ACLEntry, error) {
	resp, err := ops.client.Get("/access/acl", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list ACL: %w", err)
	}
	items, ok := resp["items"].([]interface{})
	if !ok {
		return nil, nil
	}
	entries := make([]*ACLEntry, 0, len(items))
	for _, e := range items {
		m, ok := e.(map[string]interface{})
		if !ok {
			continue
		}
		entry := &ACLEntry{}
		if v, ok := m["path"].(string); ok {
			entry.Path = v
		}
		if v, ok := m["roleid"].(string); ok {
			entry.Roles = v
		}
		if v, ok := m["ugid"].(string); ok {
			// API returns the subject id; it may be a user, token, or group.
			if strings.Contains(v, "!") {
				entry.Tokens = v
			} else {
				entry.Users = v
			}
		}
		if v, ok := m["propagate"].(float64); ok {
			entry.Propagate = v != 0
		} else if v, ok := m["propagate"].(bool); ok {
			entry.Propagate = v
		}
		entries = append(entries, entry)
	}
	return entries, nil
}
