# Implementation issues outside the current review scope

## R26 runner race failures remain outside R14

- **Issue:** The full `go test -race ./internal/runner` suite still reports races in watcher/restarter tests and fails those tests, although the focused R14 ownership race tests pass.
- **Evidence / affected paths:** Race reports identify test-global function overrides and asynchronous `ConfigWatcher`/`ProcessRestarter` work in `internal/runner/watcher_test.go`, `internal/runner/watcher.go`, and `internal/runner/runner.go`. The focused `TestMergeEnvMultiKeyResultsDoNotAlias` and `TestMergeEnvConcurrentResultsRemainOwned` tests pass under `-race`.
- **Why outside scope:** R14 changes only shared environment merge ownership. The watcher/restarter fixture and lifecycle races are the separately scoped R26 test-assurance findings and are not required to establish caller-owned `MergeEnv` results.
- **Impact / risk:** The runner package cannot honestly claim full race-clean status; unrelated asynchronous tests may hide or produce failures in other runner behaviors.
- **Follow-up:** Resolve the R26 fixture cleanup, per-instance seams, goroutine joins, and lifecycle assertions before making a broader runner race-clean claim.

## CTX root configuration map contains no files

- **Issue:** `.context/config_files.md` reports `No files found.` even though the source root contains README/module files, documentation, and CI configuration.
- **Evidence / affected paths:** `context.yaml` uses a comma-separated `filePattern: "*.md,*.yaml,*.yml,*.mod,*.sum"` for a tree source rooted at `.`; regeneration with `ctx generate --no-interaction` still produced an empty root/configuration map. The exact pattern/depth behavior of the installed generator has not been investigated.
- **Why outside scope:** The requested deep review concerns implemented agent behavior and its delivery/tests, not repair of the CTX generator's root-file selection. The prescribed context refresh and an agent-module navigation entry were completed, but the unrelated root-source selection was not changed.
- **Impact / risk:** Future navigation through CTX can miss root documentation and build/CI configuration; this does not alter agent runtime behavior.
- **Follow-up:** Check the installed CTX schema/glob and depth rules, adjust only that source's selection, regenerate, and verify README, module, documentation, and workflow coverage.
