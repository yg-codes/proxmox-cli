package vm

import (
	"fmt"
	"net/url"
	"strings"
)

// Firewall enums, each verified against a live PVE 9.2 cluster by sending a
// deliberately invalid value and reading the enumeration back out of the error.
// Validating locally means a typo is named here rather than returning the
// server's raw enum message.
var (
	firewallRuleTypes = []string{"in", "out", "forward", "group"}
	firewallPolicies  = []string{"ACCEPT", "REJECT", "DROP"}
	firewallLogLevels = []string{"emerg", "alert", "crit", "err", "warning",
		"notice", "info", "debug", "nolog"}
	firewallRefTypes = []string{"alias", "ipset"}
)

// FirewallRule is one rule in a VM's firewall chain.
//
// Pos is the rule's index in the chain and doubles as its identity: rules have
// no stable id, so inserting or deleting one renumbers everything after it.
// That makes "delete rule 3" a positional operation, not a durable reference.
type FirewallRule struct {
	Pos      int
	Type     string
	Action   string
	Enable   int
	Macro    string
	Iface    string
	Source   string
	Dest     string
	Proto    string
	SPort    string
	DPort    string
	Comment  string
	Log      string
	ICMPType string
	Digest   string
}

// FirewallOptions is a VM's firewall configuration.
type FirewallOptions struct {
	Enable      *bool
	DHCP        *bool
	IPFilter    *bool
	MACFilter   *bool
	NDP         *bool
	RADV        *bool
	PolicyIn    string
	PolicyOut   string
	LogLevelIn  string
	LogLevelOut string
	Digest      string
}

// FirewallAlias is a named CIDR, usable in a rule's source or dest.
type FirewallAlias struct {
	Name    string
	CIDR    string
	Comment string
	Digest  string
}

// FirewallIPSet is a named collection of CIDRs.
type FirewallIPSet struct {
	Name    string
	Comment string
	Digest  string
}

// FirewallIPSetEntry is one CIDR inside an IP set.
type FirewallIPSetEntry struct {
	CIDR    string
	Comment string
	// NoMatch inverts this entry: the set matches everything except it.
	NoMatch bool
	Digest  string
}

// FirewallRef is an alias or IP set that rules may reference.
type FirewallRef struct {
	Type    string
	Name    string
	Ref     string
	Scope   string
	Comment string
}

func (ops *Operations) firewallPath(node, vmid, sub string) string {
	base := fmt.Sprintf("/nodes/%s/qemu/%s/firewall", node, vmid)
	if sub == "" {
		return base
	}
	return base + "/" + sub
}

// ----- rules ---------------------------------------------------------------

// ListFirewallRules returns a VM's firewall rules in chain order.
// API: GET /nodes/{node}/qemu/{vmid}/firewall/rules
func (ops *Operations) ListFirewallRules(vmid string) ([]*FirewallRule, error) {
	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return nil, err
	}

	resp, err := ops.client.Get(ops.firewallPath(node, vmid, "rules"), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list firewall rules for VM %s: %w", vmid, err)
	}

	items, ok := resp["items"].([]interface{})
	if !ok {
		return nil, nil
	}
	out := make([]*FirewallRule, 0, len(items))
	for _, raw := range items {
		if m, ok := raw.(map[string]interface{}); ok {
			out = append(out, firewallRuleFromMap(m))
		}
	}
	return out, nil
}

// GetFirewallRule reads one rule by position.
// API: GET /nodes/{node}/qemu/{vmid}/firewall/rules/{pos}
func (ops *Operations) GetFirewallRule(vmid string, pos int) (*FirewallRule, error) {
	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return nil, err
	}

	path := ops.firewallPath(node, vmid, fmt.Sprintf("rules/%d", pos))
	resp, err := ops.client.Get(path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to read firewall rule %d on VM %s: %w", pos, vmid, err)
	}
	// api.Client already unwraps a single-object "data" field and returns the
	// inner map, so resp IS the object. The nested form is still handled in
	// case a caller passes a raw envelope.
	if data, ok := resp["data"].(map[string]interface{}); ok {
		return firewallRuleFromMap(data), nil
	}
	if len(resp) > 0 {
		return firewallRuleFromMap(resp), nil
	}
	return nil, fmt.Errorf("firewall rule %d not found on VM %s", pos, vmid)
}

// CreateFirewallRule inserts a rule. Without an explicit Pos it goes to the top
// of the chain, which is the API's behaviour, not a choice made here — worth
// knowing because a new rule therefore takes precedence over existing ones.
// API: POST /nodes/{node}/qemu/{vmid}/firewall/rules
func (ops *Operations) CreateFirewallRule(vmid string, rule *FirewallRule) error {
	if err := ValidateFirewallRule(rule); err != nil {
		return err
	}
	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return err
	}

	data := url.Values{}
	data.Set("type", rule.Type)
	data.Set("action", rule.Action)
	applyFirewallRuleFields(data, rule, true)

	if _, err := ops.client.Post(ops.firewallPath(node, vmid, "rules"), data); err != nil {
		return fmt.Errorf("failed to create firewall rule on VM %s: %w", vmid, err)
	}
	ops.logger.Infof("Created %s/%s firewall rule on VM %s", rule.Type, rule.Action, vmid)
	return nil
}

// UpdateFirewallRule changes the rule at a position.
// API: PUT /nodes/{node}/qemu/{vmid}/firewall/rules/{pos}
func (ops *Operations) UpdateFirewallRule(vmid string, pos int, rule *FirewallRule, deleteKeys []string, moveTo int) error {
	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return err
	}

	// The API refuses to delete type or action ("unable to delete required
	// property"), so catch it here with an explanation instead.
	for _, k := range deleteKeys {
		if k == "type" || k == "action" {
			return fmt.Errorf("%q cannot be deleted from a rule — it is required. "+
				"Change it with --%s instead, or delete the whole rule", k, k)
		}
	}

	data := url.Values{}
	// moveto reorders the rule; the API ignores every other argument when it is
	// present, so sending both silently drops the edits.
	if moveTo >= 0 {
		if len(deleteKeys) > 0 || hasRuleEdits(rule) {
			return fmt.Errorf("--move-to cannot be combined with other changes: " +
				"the API ignores all other arguments when moving a rule. Move it, then edit it")
		}
		data.Set("moveto", fmt.Sprintf("%d", moveTo))
		if rule != nil && rule.Digest != "" {
			data.Set("digest", rule.Digest)
		}
		path := ops.firewallPath(node, vmid, fmt.Sprintf("rules/%d", pos))
		if _, err := ops.client.Put(path, data); err != nil {
			return fmt.Errorf("failed to move firewall rule %d on VM %s: %w", pos, vmid, err)
		}
		ops.logger.Infof("Moved firewall rule %d to position %d on VM %s", pos, moveTo, vmid)
		return nil
	}
	if rule.Type != "" {
		if err := validateEnum("type", rule.Type, firewallRuleTypes); err != nil {
			return err
		}
		data.Set("type", rule.Type)
	}
	if rule.Action != "" {
		data.Set("action", rule.Action)
	}
	applyFirewallRuleFields(data, rule, false)
	if len(deleteKeys) > 0 {
		data.Set("delete", strings.Join(deleteKeys, ","))
	}
	if rule.Digest != "" {
		data.Set("digest", rule.Digest)
	}
	if len(data) == 0 {
		return fmt.Errorf("nothing to update: pass at least one field to change or delete")
	}

	path := ops.firewallPath(node, vmid, fmt.Sprintf("rules/%d", pos))
	if _, err := ops.client.Put(path, data); err != nil {
		return fmt.Errorf("failed to update firewall rule %d on VM %s: %w", pos, vmid, err)
	}
	ops.logger.Infof("Updated firewall rule %d on VM %s", pos, vmid)
	return nil
}

// DeleteFirewallRule removes the rule at a position.
//
// Positions shift: deleting rule 1 renumbers everything below it. Deleting
// several rules therefore has to go from the highest position downwards, or the
// second delete hits a rule that has moved.
// API: DELETE /nodes/{node}/qemu/{vmid}/firewall/rules/{pos}
func (ops *Operations) DeleteFirewallRule(vmid string, pos int, digest string) error {
	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return err
	}

	path := ops.firewallPath(node, vmid, fmt.Sprintf("rules/%d", pos))
	if digest != "" {
		path += "?digest=" + url.QueryEscape(digest)
	}
	if _, err := ops.client.Delete(path); err != nil {
		return fmt.Errorf("failed to delete firewall rule %d on VM %s: %w", pos, vmid, err)
	}
	ops.logger.Infof("Deleted firewall rule %d on VM %s", pos, vmid)
	return nil
}

// ----- options -------------------------------------------------------------

// GetFirewallOptions reads a VM's firewall settings.
// API: GET /nodes/{node}/qemu/{vmid}/firewall/options
func (ops *Operations) GetFirewallOptions(vmid string) (*FirewallOptions, error) {
	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return nil, err
	}

	resp, err := ops.client.Get(ops.firewallPath(node, vmid, "options"), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to read firewall options for VM %s: %w", vmid, err)
	}

	// api.Client unwraps a single-object "data" field, so resp is the object
	// itself. Reading resp["data"] here silently produced all-defaults output,
	// which looks like a correctly-read "firewall off" and is worse than an
	// error — that bug is what this fallback prevents.
	data := resp
	if nested, ok := resp["data"].(map[string]interface{}); ok {
		data = nested
	}

	o := &FirewallOptions{}
	o.Digest, _ = data["digest"].(string)
	o.PolicyIn, _ = data["policy_in"].(string)
	o.PolicyOut, _ = data["policy_out"].(string)
	o.LogLevelIn, _ = data["log_level_in"].(string)
	o.LogLevelOut, _ = data["log_level_out"].(string)
	o.Enable = boolPtrFrom(data["enable"])
	o.DHCP = boolPtrFrom(data["dhcp"])
	o.IPFilter = boolPtrFrom(data["ipfilter"])
	o.MACFilter = boolPtrFrom(data["macfilter"])
	o.NDP = boolPtrFrom(data["ndp"])
	o.RADV = boolPtrFrom(data["radv"])
	return o, nil
}

// SetFirewallOptions changes a VM's firewall settings.
// API: PUT /nodes/{node}/qemu/{vmid}/firewall/options
func (ops *Operations) SetFirewallOptions(vmid string, o *FirewallOptions, deleteKeys []string) error {
	if o == nil {
		return fmt.Errorf("options are required")
	}
	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return err
	}

	for name, value := range map[string]string{
		"policy_in":  o.PolicyIn,
		"policy_out": o.PolicyOut,
	} {
		if value != "" {
			if err := validateEnum(name, value, firewallPolicies); err != nil {
				return err
			}
		}
	}
	for name, value := range map[string]string{
		"log_level_in":  o.LogLevelIn,
		"log_level_out": o.LogLevelOut,
	} {
		if value != "" {
			if err := validateEnum(name, value, firewallLogLevels); err != nil {
				return err
			}
		}
	}

	data := url.Values{}
	setStr(data, "policy_in", o.PolicyIn)
	setStr(data, "policy_out", o.PolicyOut)
	setStr(data, "log_level_in", o.LogLevelIn)
	setStr(data, "log_level_out", o.LogLevelOut)
	setBoolPtr(data, "enable", o.Enable)
	setBoolPtr(data, "dhcp", o.DHCP)
	setBoolPtr(data, "ipfilter", o.IPFilter)
	setBoolPtr(data, "macfilter", o.MACFilter)
	setBoolPtr(data, "ndp", o.NDP)
	setBoolPtr(data, "radv", o.RADV)
	if len(deleteKeys) > 0 {
		data.Set("delete", strings.Join(deleteKeys, ","))
	}
	if o.Digest != "" {
		data.Set("digest", o.Digest)
	}
	if len(data) == 0 {
		return fmt.Errorf("nothing to update: pass at least one option to change or delete")
	}

	if _, err := ops.client.Put(ops.firewallPath(node, vmid, "options"), data); err != nil {
		return fmt.Errorf("failed to update firewall options for VM %s: %w", vmid, err)
	}
	ops.logger.Infof("Updated firewall options for VM %s", vmid)
	return nil
}

// ----- aliases -------------------------------------------------------------

// ListFirewallAliases returns a VM's firewall aliases.
func (ops *Operations) ListFirewallAliases(vmid string) ([]*FirewallAlias, error) {
	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return nil, err
	}
	resp, err := ops.client.Get(ops.firewallPath(node, vmid, "aliases"), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list firewall aliases for VM %s: %w", vmid, err)
	}
	items, ok := resp["items"].([]interface{})
	if !ok {
		return nil, nil
	}
	out := make([]*FirewallAlias, 0, len(items))
	for _, raw := range items {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		a := &FirewallAlias{}
		a.Name, _ = m["name"].(string)
		a.CIDR, _ = m["cidr"].(string)
		a.Comment, _ = m["comment"].(string)
		a.Digest, _ = m["digest"].(string)
		out = append(out, a)
	}
	return out, nil
}

// CreateFirewallAlias adds a named CIDR.
func (ops *Operations) CreateFirewallAlias(vmid string, alias *FirewallAlias) error {
	if alias == nil || alias.Name == "" {
		return fmt.Errorf("an alias name is required")
	}
	if alias.CIDR == "" {
		return fmt.Errorf("a CIDR is required, e.g. 192.0.2.0/8 or 198.51.100.5")
	}
	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return err
	}

	data := url.Values{}
	data.Set("name", alias.Name)
	data.Set("cidr", alias.CIDR)
	setStr(data, "comment", alias.Comment)

	if _, err := ops.client.Post(ops.firewallPath(node, vmid, "aliases"), data); err != nil {
		return fmt.Errorf("failed to create firewall alias %q on VM %s: %w", alias.Name, vmid, err)
	}
	ops.logger.Infof("Created firewall alias %s on VM %s", alias.Name, vmid)
	return nil
}

// UpdateFirewallAlias changes an alias's CIDR, comment, or name.
func (ops *Operations) UpdateFirewallAlias(vmid, name string, alias *FirewallAlias, rename string) error {
	if name == "" {
		return fmt.Errorf("the alias name to update is required")
	}
	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return err
	}

	data := url.Values{}
	if alias != nil {
		setStr(data, "cidr", alias.CIDR)
		setStr(data, "comment", alias.Comment)
		if alias.Digest != "" {
			data.Set("digest", alias.Digest)
		}
	}
	setStr(data, "rename", rename)
	if len(data) == 0 {
		return fmt.Errorf("nothing to update: pass --cidr, --comment or --rename")
	}

	path := ops.firewallPath(node, vmid, "aliases/"+name)
	if _, err := ops.client.Put(path, data); err != nil {
		return fmt.Errorf("failed to update firewall alias %q on VM %s: %w", name, vmid, err)
	}
	ops.logger.Infof("Updated firewall alias %s on VM %s", name, vmid)
	return nil
}

// DeleteFirewallAlias removes an alias.
func (ops *Operations) DeleteFirewallAlias(vmid, name string) error {
	if name == "" {
		return fmt.Errorf("an alias name is required")
	}
	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return err
	}
	if _, err := ops.client.Delete(ops.firewallPath(node, vmid, "aliases/"+name)); err != nil {
		return fmt.Errorf("failed to delete firewall alias %q on VM %s: %w", name, vmid, err)
	}
	ops.logger.Infof("Deleted firewall alias %s on VM %s", name, vmid)
	return nil
}

// ----- ipset ---------------------------------------------------------------

// ListFirewallIPSets returns a VM's IP sets.
func (ops *Operations) ListFirewallIPSets(vmid string) ([]*FirewallIPSet, error) {
	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return nil, err
	}
	resp, err := ops.client.Get(ops.firewallPath(node, vmid, "ipset"), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list IP sets for VM %s: %w", vmid, err)
	}
	items, ok := resp["items"].([]interface{})
	if !ok {
		return nil, nil
	}
	out := make([]*FirewallIPSet, 0, len(items))
	for _, raw := range items {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		s := &FirewallIPSet{}
		s.Name, _ = m["name"].(string)
		s.Comment, _ = m["comment"].(string)
		s.Digest, _ = m["digest"].(string)
		out = append(out, s)
	}
	return out, nil
}

// CreateFirewallIPSet creates an empty IP set.
//
// rename doubles as the update path: the API has no PUT on the set itself, so
// renaming — or changing only a comment, by passing rename equal to name — goes
// through this same POST.
func (ops *Operations) CreateFirewallIPSet(vmid, name, comment, rename string) error {
	if name == "" {
		return fmt.Errorf("an IP set name is required")
	}
	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return err
	}

	data := url.Values{}
	data.Set("name", name)
	setStr(data, "comment", comment)
	setStr(data, "rename", rename)

	if _, err := ops.client.Post(ops.firewallPath(node, vmid, "ipset"), data); err != nil {
		return fmt.Errorf("failed to create IP set %q on VM %s: %w", name, vmid, err)
	}
	ops.logger.Infof("Created IP set %s on VM %s", name, vmid)
	return nil
}

// UpdateFirewallIPSetEntry changes an existing entry's nomatch flag or comment.
// API: PUT /nodes/{node}/qemu/{vmid}/firewall/ipset/{name}/{cidr}
func (ops *Operations) UpdateFirewallIPSetEntry(vmid, name string, entry *FirewallIPSetEntry) error {
	if name == "" || entry == nil || entry.CIDR == "" {
		return fmt.Errorf("both an IP set name and a CIDR are required")
	}
	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return err
	}

	data := url.Values{}
	setStr(data, "comment", entry.Comment)
	if entry.NoMatch {
		data.Set("nomatch", "1")
	} else {
		data.Set("nomatch", "0")
	}
	if entry.Digest != "" {
		data.Set("digest", entry.Digest)
	}

	path := ops.firewallPath(node, vmid, "ipset/"+name+"/"+url.PathEscape(entry.CIDR))
	if _, err := ops.client.Put(path, data); err != nil {
		return fmt.Errorf("failed to update %s in IP set %q on VM %s: %w", entry.CIDR, name, vmid, err)
	}
	ops.logger.Infof("Updated %s in IP set %s on VM %s", entry.CIDR, name, vmid)
	return nil
}

// DeleteFirewallIPSet removes an IP set. The set must be empty; Proxmox refuses
// otherwise, so its entries have to be removed first.
func (ops *Operations) DeleteFirewallIPSet(vmid, name string, force bool) error {
	if name == "" {
		return fmt.Errorf("an IP set name is required")
	}
	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return err
	}
	path := ops.firewallPath(node, vmid, "ipset/"+name)
	if force {
		path += "?force=1"
	}
	if _, err := ops.client.Delete(path); err != nil {
		return fmt.Errorf("failed to delete IP set %q on VM %s: %w", name, vmid, err)
	}
	ops.logger.Infof("Deleted IP set %s on VM %s", name, vmid)
	return nil
}

// ListFirewallIPSetEntries returns the CIDRs in an IP set.
func (ops *Operations) ListFirewallIPSetEntries(vmid, name string) ([]*FirewallIPSetEntry, error) {
	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return nil, err
	}
	resp, err := ops.client.Get(ops.firewallPath(node, vmid, "ipset/"+name), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list entries in IP set %q on VM %s: %w", name, vmid, err)
	}
	items, ok := resp["items"].([]interface{})
	if !ok {
		return nil, nil
	}
	out := make([]*FirewallIPSetEntry, 0, len(items))
	for _, raw := range items {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		e := &FirewallIPSetEntry{}
		e.CIDR, _ = m["cidr"].(string)
		e.Comment, _ = m["comment"].(string)
		e.Digest, _ = m["digest"].(string)
		if p := boolPtrFrom(m["nomatch"]); p != nil {
			e.NoMatch = *p
		}
		out = append(out, e)
	}
	return out, nil
}

// AddFirewallIPSetEntry adds a CIDR to an IP set.
func (ops *Operations) AddFirewallIPSetEntry(vmid, name string, entry *FirewallIPSetEntry) error {
	if name == "" || entry == nil || entry.CIDR == "" {
		return fmt.Errorf("both an IP set name and a CIDR are required")
	}
	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return err
	}

	data := url.Values{}
	data.Set("cidr", entry.CIDR)
	setStr(data, "comment", entry.Comment)
	if entry.NoMatch {
		data.Set("nomatch", "1")
	}

	if _, err := ops.client.Post(ops.firewallPath(node, vmid, "ipset/"+name), data); err != nil {
		return fmt.Errorf("failed to add %s to IP set %q on VM %s: %w", entry.CIDR, name, vmid, err)
	}
	ops.logger.Infof("Added %s to IP set %s on VM %s", entry.CIDR, name, vmid)
	return nil
}

// DeleteFirewallIPSetEntry removes a CIDR from an IP set.
func (ops *Operations) DeleteFirewallIPSetEntry(vmid, name, cidr string) error {
	if name == "" || cidr == "" {
		return fmt.Errorf("both an IP set name and a CIDR are required")
	}
	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return err
	}
	path := ops.firewallPath(node, vmid, "ipset/"+name+"/"+url.PathEscape(cidr))
	if _, err := ops.client.Delete(path); err != nil {
		return fmt.Errorf("failed to remove %s from IP set %q on VM %s: %w", cidr, name, vmid, err)
	}
	ops.logger.Infof("Removed %s from IP set %s on VM %s", cidr, name, vmid)
	return nil
}

// ----- refs ----------------------------------------------------------------

// ListFirewallRefs returns the aliases and IP sets a rule may reference,
// including cluster-level ones. refType may be "", "alias" or "ipset".
func (ops *Operations) ListFirewallRefs(vmid, refType string) ([]*FirewallRef, error) {
	if refType != "" {
		if err := validateEnum("type", refType, firewallRefTypes); err != nil {
			return nil, err
		}
	}
	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return nil, err
	}

	path := ops.firewallPath(node, vmid, "refs")
	if refType != "" {
		path += "?type=" + url.QueryEscape(refType)
	}
	resp, err := ops.client.Get(path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list firewall refs for VM %s: %w", vmid, err)
	}
	items, ok := resp["items"].([]interface{})
	if !ok {
		return nil, nil
	}
	out := make([]*FirewallRef, 0, len(items))
	for _, raw := range items {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		r := &FirewallRef{}
		r.Type, _ = m["type"].(string)
		r.Name, _ = m["name"].(string)
		r.Ref, _ = m["ref"].(string)
		r.Scope, _ = m["scope"].(string)
		r.Comment, _ = m["comment"].(string)
		out = append(out, r)
	}
	return out, nil
}

// ----- validation and helpers ---------------------------------------------

// hasRuleEdits reports whether a rule carries any field change, so combining an
// edit with --move-to can be refused rather than silently discarded.
func hasRuleEdits(rule *FirewallRule) bool {
	if rule == nil {
		return false
	}
	return rule.Type != "" || rule.Action != "" || rule.Macro != "" ||
		rule.Iface != "" || rule.Source != "" || rule.Dest != "" ||
		rule.Proto != "" || rule.SPort != "" || rule.DPort != "" ||
		rule.Comment != "" || rule.Log != "" || rule.ICMPType != "" ||
		rule.Enable != 0
}

// ValidateFirewallRule checks a rule before it is sent.
func ValidateFirewallRule(rule *FirewallRule) error {
	if rule == nil {
		return fmt.Errorf("a rule is required")
	}
	if err := validateEnum("type", rule.Type, firewallRuleTypes); err != nil {
		return err
	}
	if rule.Action == "" {
		return fmt.Errorf("an action is required: ACCEPT, REJECT, DROP, or a security group name")
	}
	// action is NOT an enum in the API — it is pattern-validated as
	// [A-Za-z][A-Za-z0-9\-\_]+ (min 2, max 20), because a security group name is
	// as valid as ACCEPT/DROP/REJECT. Enforcing the three policies here would
	// reject legitimate rules, so only the pattern is checked.
	if err := validateFirewallAction(rule.Action); err != nil {
		return err
	}
	if rule.Log != "" {
		if err := validateEnum("log", rule.Log, firewallLogLevels); err != nil {
			return err
		}
	}
	if rule.Enable != 0 && rule.Enable != 1 {
		return fmt.Errorf("enable must be 0 or 1, got %d", rule.Enable)
	}
	return nil
}

// FirewallRuleTypes, FirewallPolicies and FirewallLogLevels expose the enums for
// help text and completion.
func FirewallRuleTypes() []string { return copyOf(firewallRuleTypes) }
func FirewallPolicies() []string  { return copyOf(firewallPolicies) }
func FirewallLogLevels() []string { return copyOf(firewallLogLevels) }

func copyOf(in []string) []string {
	out := make([]string, len(in))
	copy(out, in)
	return out
}

// validateFirewallAction checks the API's pattern for a rule action:
// [A-Za-z][A-Za-z0-9\-\_]+ with length 2-20. ACCEPT/DROP/REJECT and security
// group names all satisfy it, which is why the API declares no enum.
func validateFirewallAction(action string) error {
	if len(action) < 2 || len(action) > 20 {
		return fmt.Errorf("action %q must be 2-20 characters", action)
	}
	first := action[0]
	if (first < 'A' || first > 'Z') && (first < 'a' || first > 'z') {
		return fmt.Errorf("action %q must start with a letter", action)
	}
	for i := 1; i < len(action); i++ {
		c := action[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9',
			c == '-', c == '_':
		default:
			return fmt.Errorf("action %q may contain only letters, digits, '-' and '_'", action)
		}
	}
	return nil
}

func validateEnum(field, value string, allowed []string) error {
	for _, a := range allowed {
		if value == a {
			return nil
		}
	}
	if value == "" {
		return fmt.Errorf("%s is required; valid values are: %s", field, strings.Join(allowed, ", "))
	}
	return fmt.Errorf("unknown %s %q; valid values are: %s", field, value, strings.Join(allowed, ", "))
}

func applyFirewallRuleFields(data url.Values, rule *FirewallRule, isCreate bool) {
	setStr(data, "macro", rule.Macro)
	setStr(data, "iface", rule.Iface)
	setStr(data, "source", rule.Source)
	setStr(data, "dest", rule.Dest)
	setStr(data, "proto", rule.Proto)
	setStr(data, "sport", rule.SPort)
	setStr(data, "dport", rule.DPort)
	setStr(data, "comment", rule.Comment)
	setStr(data, "log", rule.Log)
	setStr(data, "icmp-type", rule.ICMPType)
	if rule.Pos > 0 {
		data.Set("pos", fmt.Sprintf("%d", rule.Pos))
	}
	// enable=0 is meaningful (a disabled rule), so it must be sent on create
	// rather than skipped as a zero value.
	if isCreate || rule.Enable != 0 {
		data.Set("enable", fmt.Sprintf("%d", rule.Enable))
	}
}

func firewallRuleFromMap(m map[string]interface{}) *FirewallRule {
	r := &FirewallRule{}
	if v, ok := m["pos"].(float64); ok {
		r.Pos = int(v)
	}
	if v, ok := m["enable"].(float64); ok {
		r.Enable = int(v)
	}
	r.Type, _ = m["type"].(string)
	r.Action, _ = m["action"].(string)
	r.Macro, _ = m["macro"].(string)
	r.Iface, _ = m["iface"].(string)
	r.Source, _ = m["source"].(string)
	r.Dest, _ = m["dest"].(string)
	r.Proto, _ = m["proto"].(string)
	r.SPort, _ = m["sport"].(string)
	r.DPort, _ = m["dport"].(string)
	r.Comment, _ = m["comment"].(string)
	r.Log, _ = m["log"].(string)
	r.ICMPType, _ = m["icmp-type"].(string)
	r.Digest, _ = m["digest"].(string)
	return r
}

func setStr(data url.Values, key, value string) {
	if value != "" {
		data.Set(key, value)
	}
}

func setBoolPtr(data url.Values, key string, value *bool) {
	if value == nil {
		return
	}
	if *value {
		data.Set(key, "1")
	} else {
		data.Set(key, "0")
	}
}

// boolPtrFrom converts an API boolean, which arrives as a number, a bool or a
// string depending on how it was set. Returns nil when the field is absent, so
// "unset" stays distinguishable from "explicitly false".
func boolPtrFrom(v interface{}) *bool {
	var b bool
	switch t := v.(type) {
	case float64:
		b = t != 0
	case bool:
		b = t
	case string:
		b = t == "1" || strings.EqualFold(t, "true")
	default:
		return nil
	}
	return &b
}
