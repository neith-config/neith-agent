# Backlog

## Product identity

- [ ] **NAME-001 — Unify the project name as Neith.** The intended name is the Egyptian goddess Neith (Russian: Нет); use `neith` as the target lowercase spelling. The approved target is organization `neith-config`, repository `neith-agent`, Go module `github.com/neith-config/neith-agent`, and binary `neith-agent`. The migration is deliberately breaking in the current tree and public configuration: no compatibility aliases or history rewrite. GitHub's old repository redirect is accepted. Implementation plan: [Neith rename and remediation plan](../.pi/plans/2026-10-15-neith-renaming-and-remediation/plan.md).
  - Requested: 2026-10-15.
  - Name reference: [World History Encyclopedia — Neith](https://www.worldhistory.org/Neith/).

## Documentation

- [ ] **DOC-001 — Align README with the sidecar-daemon contract.** The product is a persistent sidecar daemon that launches and manages the application's main process, whatever executable it is. It is not merely a one-shot configuration launcher. Reconcile CLI, lifecycle, configuration delivery, and error documentation with the agreed daemon behavior and the implementation after its defects are resolved.
  - Authority: [Sidecar contract clarification](../.history/2026-10-15-sidecar-contract-and-review.md).
  - Review evidence and unresolved behavior: [Deep implementation review](review-2026-10-15.md). Remediation is not yet an approved implementation plan.
