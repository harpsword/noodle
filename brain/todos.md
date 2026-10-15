---
priority: [115, 84, 88, 90, 118, 116, 117, 110, 96, 101, 100, 86, 111, 85, 69]
# Priority notes:
#    codebase simplification program — root-cause-first execution of full audit
#    deterministic completion detection — replace fragile completion heuristics
#   115 canonical-state convergence — finish the migration and delete duplicated loop lifecycle logic
#   84 sub-agent tracking — visibility into agent orchestration
#   88 sub-agent tracking v2 — hardening, depends on 84
#   90 interactive sessions — collaborative mode, key differentiator
#   118 unified launch surface — make Noodle a real operator cockpit, not just a monitor
#   116 projection-driven live protocol — move UI/server off filesystem-churn coupling
#   117 typed control plane — explicit operator API before more surface area lands
#   110 chat input enhancements — polish interactive sessions UX
#   96,101 split out Sprites — delete built-in runtime, plugin interface
#   100 runtime mode UI setting
#   86 diffs integration — code-change review UX
#   111 backlog-in-UI visibility — improve operator ergonomics
#   85 .noodle.toml live reload — convenience, restart works fine
#   69 cursor dispatcher — extended runtime, process+sprites cover launch
---

# Todos

<!-- next-id: 132 -->
<!-- completed todos live in archive/completed_todos.md -->
<!-- completed plans live in archive/plans/ -->

## Onboarding & Getting Started


## Remote Dispatchers

69. [ ] Cursor dispatcher — implement CursorBackend (real HTTP client replacing stub), PollingDispatcher + pollingSession (deferred from plan 27 phase 4), webhook receiver endpoint on Noodle's HTTP server for status change notifications, factory wiring in loop/defaults.go. Flow: push worktree branch → launch Cursor Cloud Agent via API → agent pushes to target branch (no PR) → detect completion via webhook or polling → pull target branch back to worktree. [[plans/69-cursor-dispatcher/overview]]

## UI

100. [ ] Add a UI setting to change the run mode (auto/supervised/manual) at runtime. #needs_plan
110. [ ] Chat input enhancements — drag-to-add images, copy/paste images, and `@` autocomplete for files (and possibly actors). #needs_plan
86. [ ] Integrate diffs.com diff-rendering component into the web UI — add a bundled JS diff component (from https://diffs.com/) that renders code changes as inline diffs. Show diffs in two places: (1) inline in the session activity feed alongside each code-change event, collapsed by default with a click-to-expand interaction (avoid noise in the feed), and (2) in a dedicated diff tab/panel that collects all code changes from a session (expanded by default). Ship-ready: fully integrated, styled, and tested. [[plans/86-diffs-integration/overview]]
111. [ ] Allow viewing backlog items in the UI. #needs_plan
118. [ ] Unify Noodle's launch surface so operators can start both interactive sessions and structured work from one place with explicit provider/model/runtime selection, instead of scattering spawn controls across scheduler-only UI affordances. #needs_plan

## Backend

120. [ ] Fix scheduler self-retriggering after empty orders — separate meaningful scheduling input changes from scheduler lifecycle/history changes, preserve empty-result cooldown, and wait for active execution outcomes without repeatedly spawning schedule sessions. Verify backlog and execution changes still wake scheduling. #bug [[codebase/scheduler-self-retrigger-after-empty-orders]] #needs_plan — Audit R7: apply the same meaningful-input/cooldown eligibility to bootstrap and mise-change injection; preserve capacity and explicit-operator triggers. [[audits/2026-10-15-runtime-redundant-work]]

84. [ ] Sub-agent tracking — parse Claude/Codex sub-agent lifecycle into canonical events, build agent tree in snapshots, stream activity to UI, and enable user steering. [[plans/84-subagent-tracking/overview]] — define canonical backend failure classes (hard invariant, recoverable backend, scheduler/cook agent mistake, agent-start unrecoverable vs retryable), map loop/start/dispatcher boundaries, and surface typed recoverability metadata for operators. [[archive/plans/83-error-recoverability-taxonomy/overview]]
85. [ ] Add `.noodle.toml` fsnotify live reload in the running loop with safe apply semantics (debounce, parse/validation gate, partial-apply vs restart-required classification, and observability for rejected reloads). #needs_plan
88. [ ] Sub-agent tracking v2 — add out-of-band ingestion (Codex child sessions + Claude team inbox), harden canonical identity reconciliation, lifecycle-safe bounded pollers, robust `steer-agent` control behavior, and expanded hardening tests. [[plans/88-subagent-tracking-v2/overview]]
115. [ ] Make canonical state + projection the authoritative execution path — route dispatch planning, completion handling, merge advancement, and UI snapshots through `internal/{ingest,reducer,dispatch,projection,snapshot}`; then delete duplicated lifecycle logic that still lives in `loop/`. #needs_plan [[plans/115-canonical-state-convergence/overview]]
116. [ ] Replace filesystem-watch-driven snapshot rebroadcasting with a typed projection/delta stream from the canonical backend so the UI consumes domain state changes rather than raw `.noodle/` file churn. #needs_plan — Audit D2 and remaining D1: avoid full active-session event-history reads for unrelated changes; define change-driven snapshot/delta ownership and append/truncate/replace/reconnect correctness. Build on task 128 consumer gating without duplicating it; coordinate with task 115. [[audits/2026-10-15-runtime-redundant-work]]
117. [ ] Introduce a typed operator control plane for HTTP/WebSocket actions (launch, steer, review, mode changes, runtime selection) so server and UI stop depending on generic stringly control payloads. #needs_plan

## Extensibility

96. [ ] Custom dispatcher/runtime plugins — allow users to write their own dispatcher or runtime plugin so they can add a custom VM by implementing Noodle's interface. [[plans/96-101-runtime-plugins/overview]]
101. [ ] Split out Sprites runtime into its own runtime plugin (`noodle-runtime-sprites`). Sprites should not be a built-in default, it should be an installable plugin that uses the custom runtime interface (#96). [[plans/96-101-runtime-plugins/overview]]

## Brainmaxxing

119. [ ] Brainmaxxing autoresearch improvements — curator gate for meditate, structured findings in reflect, new optimize skill for GEPA-inspired skill hill-climbing. [[plans/119-brainmaxxing-autoresearch/overview]]

## Design Explorations


## Features

90. [ ] Interactive agent sessions — spawn a human-in-the-loop agent session outside the order system for collaborative work (planning, exploration, design decisions) where full automation is wrong. Separate from orders: no order ID, no stage pipeline, no quality/reflect lifecycle. Backend: new control action (not `enqueue`) that spawns a provider session with user-chosen provider/model, streams canonical events, and accepts user messages mid-flight via the existing steering mechanism. Runs on primary checkout by default, opt-in worktree for code-change tasks. UI: web chat panel in Noodle UI — real-time streamed agent output with a user input field for back-and-forth conversation. Launcher placement (replace scheduler feed input vs. new top-level action vs. separate panel) is undecided — design should emerge during planning. Long-term: unify the spawn surface so both structured orders and interactive sessions launch from the same place. [[plans/90-interactive-sessions/overview]]

## Runtime Efficiency

124. [ ] Refresh session metadata only when source state changes (R2) — separate active PID/heartbeat/deadline checks from historical canonical-log scans and time-only terminal metadata rewrites. After task 123, prove stable history causes no repeated full reads or meta writes; appended, truncated, replaced and corrected inputs refresh correctly, and process death/heartbeat expiry is detected without new logs. #needs_plan [[audits/2026-10-15-runtime-redundant-work]]
125. [ ] Avoid full ticket replay on unchanged session logs (R3) — after task 123, design change-driven ticket materialization and skip identical tickets.json writes. Preserve claims/releases from terminal sessions, cross-session event ordering, restart reconstruction and append/truncate/replace recovery. Accept: repeated unchanged maintenance does not reread full history or replace tickets; changed inputs match full replay. #needs_plan [[audits/2026-10-15-runtime-redundant-work]]
126. [ ] Decouple backlog adapter refresh from loop wakeups (R5) — after task 123, define bounded freshness and invalidation for local and remote adapters so idle/paused ticks and unrelated file changes do not spawn a sync subprocess each time. Accept: unchanged cycles reuse backlog data, genuine edits and refresh expiry become visible within the defined bound, and failures/recovery preserve correct scheduling. Measure adapter subprocess cost separately from Noodle CPU. #needs_plan [[audits/2026-10-15-runtime-redundant-work]]
127. [ ] Reuse final cycle projection and state publication (R6) — after task 123, remove duplicate projection/hash/sort/publish work and artificial idle-running-idle transitions when nothing changed. Coordinate with task 115 canonical-state convergence; preserve externally observable lifecycle transitions, snapshot isolation and recovery checkpoints. Accept: a stable cycle does not repeatedly rebuild or republish equivalent state; genuine transitions remain ordered and visible. #needs_plan [[audits/2026-10-15-runtime-redundant-work]]
128. [ ] Skip presentation work without consumers (D1/D3) — after task 123, prevent zero-client WebSocket snapshot construction and zero-subscriber live-event formatting/encoding; narrow invalidation to actual snapshot dependencies. This bounded fix precedes task 116 protocol redesign. Accept: unrelated writes with no clients cause no snapshot builds, no subscribers cause no live payload encoding, first subscription/reconnect backfills fully, and durable event persistence is unchanged. #needs_plan [[audits/2026-10-15-runtime-redundant-work]]
129. [ ] Emit backlog sync degradation only on meaningful warning transitions (R8) — repeated identical missing/failed-adapter warnings must not append a fresh sync.degraded event each cycle. Accept: first failure is visible, repeated same failure is quiet, changed failure and recovery followed by failure are observable, and repeated warnings do not create new scheduling input. Coordinate scheduling eligibility with task 120. #bug [[audits/2026-10-15-runtime-redundant-work]]
130. [ ] Reduce duplicate session history loading and frontend event merging (D4/D5) — after task 116 defines snapshot/delta ownership, profile first load/reconnect overlap between HTTP history and WebSocket backfill, and repeated full-history deduplication/copy/sort/regroup in the browser. Plan only changes supported by measurements; accept exactly-once displayed events under overlapping backfill, duplicate/out-of-order arrival and reconnect, preserved delta/virtual-list behavior, and separately reported server/browser costs. #needs_plan [[audits/2026-10-15-runtime-redundant-work]]
131. [ ] Measure active-output conversion and log-writer overhead before optimizing (D6/D7/D8) — after tasks 123-128, profile duplicate canonical-to-event conversion, stamp JSON roundtrips, per-event mkdir/open/close, and append/scan/rewrite of the bounded lifecycle log. Deliver reproducible representative measurements and benefit/cost decisions; retain distinct log consumers, event order, durability and retention contracts. File measured implementation follow-ups with plan review where needed; do not add caches or persistent handles solely from static suspicion. #research [[audits/2026-10-15-runtime-redundant-work]]
