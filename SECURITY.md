# Security Policy

## Supported versions

The `main` branch and the latest tagged release receive security fixes.

## Reporting a vulnerability

Open a **private** report via GitHub's *"Report a vulnerability"* feature on
the Security tab, or email the maintainer via the address listed on the
profile. Please do not open a public issue for suspected vulnerabilities.

Include: the commit or release affected, a minimal reproduction, and the
impact you observe.

## Security posture

- **TLS verification is on by default** (`PVE_VERIFY_SSL=true`). Disabling it
  (`false`) permits man-in-the-middle interception and is intended only for
  lab hosts with self-signed certificates.
- **Private CAs**: set `PVE_CA_CERT=/path/to/ca-bundle.pem` to verify a host
  whose certificate chains to your own CA — preferred over disabling
  verification. Malformed or unreadable bundles fail configuration loading.
- **Credentials** are read from environment variables (optionally resolved
  from 1Password via `op://` references); the CLI never writes tokens to disk.
- **Scope**: this is an API client. It performs only the operations you
  request; confirmations with typed input guard destructive verbs
  (`destroy`, `prune`, pool `destroy`, backup-job `delete`).

## Threat model and limitations

| Area | Limitation |
|---|---|
| Transport security | With `PVE_VERIFY_SSL=false` all traffic is interceptable — the setting exists for self-signed labs, not production |
| Credential storage | Environment variables and shell history are outside this tool's control; prefer token auth with least-privilege roles over password auth |
| REST-only view | Cluster health/quorum reads come from the REST API; it is a point-in-time snapshot, not a substitute for on-node checks (`pvecm`, `corosync`, logs) |
| Guest agent | The agent may run while refusing individual commands — a guest-side configuration issue, surfaced as an error, not bypassed |
| Concurrency | Bulk operations cap concurrency (`PVE_OPERATIONS_MAX_CONCURRENT_*`) but do not coordinate with other actors; a VM locked by another session fails its operation rather than queueing |
| Audit trail | The CLI prints what it did; durable auditing is the Proxmox task log's responsibility |
