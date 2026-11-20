---
ticket: ""
status: implementation_complete
---

# Reporting integrity fixes

## Scope

Fix three reproduced reporting defects without changing infrastructure,
credentials, ACLs, dependencies, or unrelated command behavior:

- Reject capacity analysis when online-node hardware capacity is unknown.
- Propagate storage inventory/status API failures with node/storage context.
- Preserve and display each node/storage pair, rather than merging local pools
  with the same storage name.

## Steps

1. Add failing regression tests at the public resource/storage operation APIs,
   using loopback TLS fixtures and complete-data controls.
2. Fix missing-capacity handling and preserve CPU capacity independently of
   whether CPU-utilization data is present.
3. Fix error propagation in both VM and backup storage listings.
4. Keep node/storage identities distinct and include the node in both tables.
5. Run the complete test suite, vet, race tests, formatting/lint checks and the
   original local mock repro. Review changes independently before publishing.
6. Commit explicit paths, push the topic branch and open a PR to main. Do not
   merge, release, install the new CLI or change infrastructure.

## Acceptance

Incomplete capacity and storage reads return errors; healthy and genuinely
empty storage reads remain distinguishable. Same-named local pools retain
separate capacity readings with visible node scope. Existing tests pass.
Only generic fixtures and public product information enter the repository/PR.

## Compatibility and rollback

Storage listings/validators now fail closed on incomplete inventory, and table
consumers must account for an added node column and node-scoped rows. Shared
storage also appears once per node; rows are not additive cluster capacity.
A denied, unavailable or offline node can therefore block name-only backup
validation even when the selected storage is usable elsewhere. Review identified
this compatibility risk; silently skipping that node would reintroduce partial
success. Keep the conservative behavior and cover it with validation tests. Once the implementation
commit is the topic-branch tip, rollback is `git revert --no-edit HEAD` on this
branch; do not discard working trees or modify infrastructure to undo code.

## Verification

- Each of the three regression slices failed before its implementation change
  and passed afterwards, using loopback-only TLS fixtures.
- The original mock repro passes all five checks (including healthy controls).
- `make test`, `make vet`, `go test -race ./...`, formatting and whitespace
  checks pass with Go 1.27.4. A CLI build also passes with
  `go build -o /tmp/proxmox-cli-reporting-fixed ./pve`.
  Minimum-runtime compatibility was not tested locally.
- `golangci-lint run` v2.13.1 is blocked locally: its importer supports export
  data version 4, while the installed Go emits version 5. The same failure was
  reproduced on an archive of the unchanged base revision
  `c208d913dca5f592f90e9bcd70da7e5deaadf306`; lint is not claimed to pass.
- Independent review completed; its offline-node compatibility finding is
  recorded above. No infrastructure mutation or CLI installation was performed.
- Publication and final remote CI result are pending.
