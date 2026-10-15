# Task 124 verification

Completed 2026-10-16. Source fix b3c1f5c7b3a0dd706465765d45d77579208b03c6; red regressions ba8ea04; reviewed plan 1595aee. Back to [[archive/plans/124-session-metadata-refresh/overview]].

## Actual running process

The installed binary is `/opt/homebrew/libexec/noodle-local/noodle`, reached through the existing `/opt/homebrew/bin/noodle` symlink. Built on clean main with `pnpm build`, version **v0.1.5+local.b3c1f5c**; embedded vcs.revision matches the source fix and vcs.modified=false. The old executable was backed up locally before atomic replacement. PID 53523 was gracefully stopped; PID 97332 runs the new executable in `/Users/hs/code/Projects/noodle_playground`.

Both samples used the same 58 historical sessions, the same single failed schedule order and running/supervised/max_concurrency=4 state. No pause or empty-order substitution was used. Orders compare equal before and after restart. Measurement uses cumulative process CPU time divided by elapsed wall time; 100% means one CPU core. File changes are observed by 10 ms polling of inode/mtime/size, not syscall tracing.

| Measurement | Before: local.3c51ca9 | After: local.b3c1f5c |
|---|---:|---:|
| Sample duration | 15.006 s | 15.008 s |
| Process CPU time | 1.35 s | 0.33 s |
| Mean CPU | 9.0% | 2.2% |
| Metadata changes, all 58 histories | 870 | 0 |
| orders/state/pending-review/last-applied-seq/control changes, each | 0 | 0 |
| tickets.json changes | 15 | 15 |

CPU decreased approximately 76% for this workload. All metadata files retained identical bytes during the after window. Residual ticket replay/write work is task 125 and remains unchanged. This sample does not claim equivalent percentages under active provider workloads or different history sizes.

## Behavioral and repository verification

- Red tests reproduced 5 canonical opens for 5 unchanged reads, terminal metadata replacement, and growing terminal duration/idle time. The failing tests were committed separately before the fix.
- Final canonical scan counter proves one initial scan followed by zero scans for repeated unchanged reads; spawn-only correction also avoids scanning.
- Real filesystem tests cover append, truncation, same-size/mtime replacement, restored-mtime correction, removal/recreation, parse failure/recovery, fresh spawn and cold restart.
- Metadata tests assert stable bytes, inode and mtime, including non-UTC canonical timestamps across JSON round-trips. External retry counts survive; removed metadata is recreated; malformed metadata remains an error.
- Actual child process termination and fake-clock heartbeat expiry are detected without new canonical data. Spawn runtime/skill correction and persistent scheduler behavior remain covered. Concurrent RunOnce and cache pruning pass the race detector.
- Deterministic scan-time mutation tests reject unsafe publication. A deliberate temporary bypass of the descriptor-generation check makes append/truncate fail; restored implementation passes. See [[archive/plans/124-session-metadata-refresh/quality-review]].
- `go test -race ./monitor ./loop -count=1` passed; final scanner-seam adjustment was followed by a fresh monitor race run.
- Final `pnpm check` passed: build, all Go tests, vet, architecture lint, loop fixtures and fixture hashes. Architecture lint reports five existing warnings outside modified files.
- Local `go test -tags e2e -timeout 120s -count=1 ./e2e/ -run '^TestSmokeStartOnce'` passed. Model-backed smoke was not invoked; no external review CLI was used.
- Linux/amd64 and Windows/amd64 monitor test binaries cross-compiled after the final adjustment. These are compile checks, not native execution on those operating systems.

## Boundaries

Darwin and Linux cache observable generations using identity, size, mode, mtime and ctime. Filesystem timestamp collisions or otherwise unobservable rewrites are not guaranteed detectable; no content hashing or periodic full replay is added. Other platforms retain correct uncached canonical reads until a trustworthy change-time accessor is implemented. PID/heartbeat/spawn/previous-meta checks intentionally remain fresh every cycle. Canonical sources changed during a read are not cached; existing transient-snapshot/error semantics remain.

Local raw evidence is under `/tmp/noodle-124/` (before.json, after.json, check-final.log, race.log, race-monitor-final.log, smoke.log, mutation-proof.log). The durable conclusions and methodology above do not depend on retaining those temporary files.
