package vm

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// AgentInfo is the guest agent's self-report: which QMP commands it supports and
// which the guest has enabled.
//
// This matters operationally. A guest can run the agent while refusing
// individual commands — Rocky and most hardened images ship with guest-exec and
// all guest-file-* disabled — and the resulting error names the command, not the
// cause. Reading info first turns "Agent error: Command guest-exec has been
// disabled" into something explainable before it is hit.
type AgentInfo struct {
	Enabled  []string
	Disabled []string
}

// AgentExecResult is the outcome of a command run inside the guest.
type AgentExecResult struct {
	PID      int
	Exited   bool
	ExitCode int
	Stdout   string
	Stderr   string
	// Signal is set instead of ExitCode when the process was killed.
	Signal int
	// TimedOut reports that the poll loop gave up before the guest finished.
	// The command may still be running inside the guest.
	TimedOut bool
}

// agentPath builds the agent endpoint path for a VM.
func (ops *Operations) agentPath(node, vmid, endpoint string) string {
	return fmt.Sprintf("/nodes/%s/qemu/%s/agent/%s", node, vmid, endpoint)
}

// AgentGet calls a read-only agent endpoint and returns its raw result.
//
// The agent subtree returns its payload under data.result for most endpoints,
// so that is unwrapped here; endpoints returning a bare data value pass through.
func (ops *Operations) AgentGet(vmid, endpoint string) (interface{}, error) {
	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return nil, err
	}

	resp, err := ops.client.Get(ops.agentPath(node, vmid, endpoint), nil)
	if err != nil {
		return nil, agentError(vmid, endpoint, err)
	}
	return unwrapAgentResult(resp), nil
}

// AgentPost calls an action agent endpoint.
func (ops *Operations) AgentPost(vmid, endpoint string, params url.Values) (interface{}, error) {
	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return nil, err
	}

	resp, err := ops.client.Post(ops.agentPath(node, vmid, endpoint), params)
	if err != nil {
		return nil, agentError(vmid, endpoint, err)
	}
	return unwrapAgentResult(resp), nil
}

// GetAgentInfo reports which agent commands the guest permits.
func (ops *Operations) GetAgentInfo(vmid string) (*AgentInfo, error) {
	raw, err := ops.AgentGet(vmid, "info")
	if err != nil {
		return nil, err
	}

	info := &AgentInfo{}
	m, ok := raw.(map[string]interface{})
	if !ok {
		return info, nil
	}
	cmds, ok := m["supported_commands"].([]interface{})
	if !ok {
		return info, nil
	}
	for _, c := range cmds {
		cm, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := cm["name"].(string)
		if name == "" {
			continue
		}
		if enabled, _ := cm["enabled"].(bool); enabled {
			info.Enabled = append(info.Enabled, name)
		} else {
			info.Disabled = append(info.Disabled, name)
		}
	}
	return info, nil
}

// AgentPing checks that the guest agent is responding.
//
// A non-nil error here usually means the guest agent is not running — which is
// also what a booting VM reports for the first minute or two, so a single
// failure is not proof the agent is absent.
func (ops *Operations) AgentPing(vmid string) error {
	_, err := ops.AgentPost(vmid, "ping", nil)
	return err
}

// AgentExec runs a command inside the guest and waits for its output.
//
// The API returns only a PID; the caller must poll exec-status for the result.
// Without that loop the command is close to useless, so the poll is done here
// rather than left to every caller.
//
// Requires guest-exec to be enabled inside the guest. Many images (Rocky's
// default among them) ship with it disabled, in which case the guest agent
// refuses regardless of API permissions.
func (ops *Operations) AgentExec(vmid string, command []string, inputData string, timeout time.Duration) (*AgentExecResult, error) {
	if len(command) == 0 {
		return nil, fmt.Errorf("exec requires a command")
	}

	params := url.Values{}
	// The API takes the command as a single string, not a repeated parameter —
	// verified against a live cluster: command[]=… is rejected as "property is
	// not defined in schema".
	params.Set("command", strings.Join(command, " "))
	if inputData != "" {
		params.Set("input-data", inputData)
	}

	raw, err := ops.AgentPost(vmid, "exec", params)
	if err != nil {
		return nil, err
	}

	m, ok := raw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected exec response: %T", raw)
	}
	pidFloat, ok := m["pid"].(float64)
	if !ok {
		return nil, fmt.Errorf("exec did not return a pid; the guest agent may have refused the command")
	}

	result := &AgentExecResult{PID: int(pidFloat)}

	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	deadline := time.Now().Add(timeout)
	interval := 500 * time.Millisecond

	for {
		status, err := ops.AgentExecStatus(vmid, result.PID)
		if err != nil {
			return result, err
		}
		if status.Exited {
			status.PID = result.PID
			return status, nil
		}
		if time.Now().After(deadline) {
			// Report what is known rather than an error: the command is still
			// running in the guest and may yet succeed, so calling this a
			// failure would misstate the state.
			status.PID = result.PID
			status.TimedOut = true
			return status, nil
		}
		time.Sleep(interval)
	}
}

// AgentExecStatus reads the state of a command started by AgentExec.
func (ops *Operations) AgentExecStatus(vmid string, pid int) (*AgentExecResult, error) {
	node, err := ops.FindVMNode(vmid)
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("%s?pid=%d", ops.agentPath(node, vmid, "exec-status"), pid)
	resp, err := ops.client.Get(path, nil)
	if err != nil {
		return nil, agentError(vmid, "exec-status", err)
	}

	raw := unwrapAgentResult(resp)
	m, ok := raw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected exec-status response: %T", raw)
	}

	out := &AgentExecResult{PID: pid}
	if v, ok := m["exited"].(float64); ok {
		out.Exited = v != 0
	} else if v, ok := m["exited"].(bool); ok {
		out.Exited = v
	}
	if v, ok := m["exitcode"].(float64); ok {
		out.ExitCode = int(v)
	}
	if v, ok := m["signal"].(float64); ok {
		out.Signal = int(v)
	}
	if v, ok := m["out-data"].(string); ok {
		out.Stdout = v
	}
	if v, ok := m["err-data"].(string); ok {
		out.Stderr = v
	}
	return out, nil
}

// SetGuestUserPassword sets a user's password inside the guest.
func (ops *Operations) SetGuestUserPassword(vmid, username, password string, crypted bool) error {
	params := url.Values{}
	params.Set("username", username)
	params.Set("password", password)
	if crypted {
		params.Set("crypted", "1")
	}
	_, err := ops.AgentPost(vmid, "set-user-password", params)
	return err
}

// unwrapAgentResult pulls the agent payload out of the API envelope.
//
// Most agent endpoints nest their answer under data.result; a few return the
// value directly. Both shapes are handled so callers see the payload either way.
func unwrapAgentResult(resp map[string]interface{}) interface{} {
	if resp == nil {
		return nil
	}
	if data, ok := resp["data"].(map[string]interface{}); ok {
		if result, ok := data["result"]; ok {
			return result
		}
		return data
	}
	if result, ok := resp["result"]; ok {
		return result
	}
	// The client lifts array responses into "items".
	if items, ok := resp["items"]; ok {
		return items
	}
	return resp["data"]
}

// agentError annotates an agent failure with the two causes that account for
// nearly all of them, because the raw message names neither.
func agentError(vmid, endpoint string, err error) error {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "not running"):
		return fmt.Errorf("guest agent on VM %s is not responding for %q: %w\n"+
			"  The agent takes a while to come up after boot (~5 minutes was observed on\n"+
			"  Rocky 10), and reports the same error when it is not installed at all.\n"+
			"  Check 'agent: 1' in the VM config and qemu-guest-agent inside the guest",
			vmid, endpoint, err)
	case strings.Contains(msg, "has been disabled"):
		return fmt.Errorf("the guest agent refused %q on VM %s: %w\n"+
			"  This is the GUEST's own policy, not an API permission. Hardened images\n"+
			"  commonly disable guest-exec and all guest-file-* commands.\n"+
			"  Run 'pve vm agent info --vmid %s' to see what this guest permits",
			endpoint, vmid, err, vmid)
	default:
		return fmt.Errorf("agent %q failed on VM %s: %w", endpoint, vmid, err)
	}
}

// FormatAgentJSON renders an agent payload as indented JSON for display.
func FormatAgentJSON(v interface{}) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}
