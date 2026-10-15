# Empty Control Queue Idle CPU

- Backlog item 122. Fixed by skipping empty control queue truncation. Observed on 2026-10-15 in `noodle_playground`, macOS, Homebrew Noodle v0.1.5 (`eaa1d5c`).
- With no orders and `loop_state: idle`, PID 38521 used 56.2–62.2% CPU in three consecutive readings. The empty `control.ndjson` changed 18 times in three seconds; 57 session `meta.json` files were rewritten 798 times in two seconds.
- `processControlCommands` unconditionally truncates `control.ndjson`, even when its read returned zero bytes. On macOS, truncating an already empty file generates `NOTE_ATTRIB` (fsnotify `CHMOD`). The runtime watcher matches the file name regardless of operation and runs another cycle.
- Each cycle calls `Monitor.RunOnce`, rereads historical session events, rewrites session metadata, materializes tickets, and invokes backlog sync. A symbolized sample of the installed binary confirmed the watcher-triggered `Loop.Run -> Cycle -> runCycleMaintenance -> runMonitorPass` path, event parsing, filesystem operations, and GC.
- A temporary-file kqueue probe produced five attribute notifications for five empty truncations, and none when empty writes were skipped. No live runtime files were changed for this probe.
- Fix at the writer: after reading under the existing control lock, return when `len(data) == 0`. Preserve processing, acknowledgements, and truncation for nonempty input. Do not solve this by dropping legitimate control-file notifications.
- Regression coverage: preserve an empty queue's modification time across repeated processing; process and acknowledge a real command, then use a real fsnotify watcher to ensure the drained queue cannot retrigger itself.
- Both tests failed before the fix (timestamp changed; real `CHMOD` event) and passed after it. `go test -race ./loop ./monitor ./server` passed.
- Full `pnpm check` passed with Node 24, project-pinned pnpm 10.30.3, and Git 2.53 (five existing architecture size warnings). Default Git 2.34 caused two unrelated worktree tests to fail on both the patched and unpatched source; using the newer Git resolved them.
- Isolated before/after measurement used the same source revision with only this guard toggled through a Go overlay. Both temporary projects had the same 57 historical canonical logs, paused dispatch, and the Web server disabled; no process IDs were copied. Over five seconds, CPU fell from 102.1% to 3.0%, control rewrites from 219 to zero, and observed meta rewrites from 12,482 to 285 (57 sessions × five normal polls). This measures the maintenance path, not the live instance's post-upgrade CPU.
- Remaining cost: the regular one-second poll still scans historical sessions. Incremental monitoring is a separate optimization; this fix removes the unbounded feedback loop.
- Distinct from scheduler self-retriggering after empty orders: that issue repeatedly spawns scheduler sessions; this one burns CPU after all sessions finish.

See also [[schedule-bootstrap-hot-loop]], [[scheduler-self-retrigger-after-empty-orders]], [[../principles/make-operations-idempotent]].
