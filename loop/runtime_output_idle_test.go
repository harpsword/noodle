package loop

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/poteto/noodle/config"
	"github.com/poteto/noodle/internal/filex"
	"github.com/poteto/noodle/internal/state"
	loopruntime "github.com/poteto/noodle/runtime"
)

func newOutputTestLoop(t *testing.T, scenario string) *Loop {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, ".noodle")
	orders := OrdersFile{}
	if scenario != "idle" {
		orders.Orders = []Order{{ID: "schedule", Status: OrderStatusFailed, Stages: []Stage{{TaskKey: "schedule", Skill: "schedule", Provider: "claude", Model: "claude-opus-4-6", Runtime: "process", Status: StageStatusFailed}}}}
	}
	if scenario == "active" || scenario == "review" {
		orders.Orders = []Order{{ID: "42", Status: OrderStatusActive, Stages: []Stage{{TaskKey: "execute", Skill: "execute", Provider: "claude", Model: "claude-opus-4-6", Runtime: "process", Status: StageStatusActive}}}}
	}
	path := writeTestOrders(t, dir, orders)
	l := New(root, "noodle", config.DefaultConfig(), Dependencies{Runtimes: map[string]loopruntime.Runtime{"process": newMockRuntime()}, Worktree: &fakeWorktree{}, Adapter: &fakeAdapterRunner{}, Mise: &fakeMise{}, Monitor: fakeMonitor{}, Registry: testLoopRegistry(), Now: time.Now, OrdersFile: path})
	if err := l.loadOrdersState(); err != nil {
		t.Fatal(err)
	}
	if scenario == "review" {
		l.cooks.pendingReview["42"] = &pendingReviewCook{cookIdentity: cookIdentity{orderID: "42", stageIndex: 0, stage: orders.Orders[0].Stages[0]}, worktreeName: "42-0-execute", sessionID: "sess-42"}
	}
	if err := l.ensureCanonicalLoadedFromBridge(); err != nil {
		t.Fatal(err)
	}
	if scenario == "active" {
		l.cooks.activeCooksByOrder["42"] = &cookHandle{cookIdentity: cookIdentity{orderID: "42", stageIndex: 0, stage: orders.Orders[0].Stages[0]}, session: &mockSession{id: "sess-42", status: "running", done: make(chan struct{})}}
	}
	t.Cleanup(func() {
		for _, cook := range l.cooks.activeCooksByOrder {
			if cook.session != nil {
				_ = cook.session.ForceKill()
			}
		}
		l.Shutdown()
	})
	if scenario == "paused" {
		l.state = StatePaused
	}
	return l
}

func TestRuntimeOutputsDoNotRetriggerCycles(t *testing.T) {
	for _, scenario := range []string{"idle", "paused", "failed", "review", "active"} {
		t.Run(scenario, func(t *testing.T) {
			l := newOutputTestLoop(t, scenario)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			// Watch the actual runtime directory, using the same forwarder and ticker as Run.
			w, err := fsnotify.NewWatcher()
			if err != nil {
				t.Fatal(err)
			}
			defer w.Close()
			if err = w.Add(l.runtimeDir); err != nil {
				t.Fatal(err)
			}
			trigger := make(chan struct{}, 1)
			errs := make(chan error, 1)
			done := make(chan struct{})
			go func() { defer close(done); l.forwardRuntimeWatcher(ctx, w.Events, w.Errors, trigger, errs) }()
			defer func() { cancel(); <-done }()
			ticker := time.NewTicker(l.pollInterval())
			defer ticker.Stop()
			timer := time.NewTimer(1200 * time.Millisecond)
			defer timer.Stop()
			enqueueRuntimeCycle(trigger)
			cycles := 0
			for {
				select {
				case <-timer.C:
					if cycles < 2 {
						t.Fatalf("ticker made no progress: %d cycles", cycles)
					}
					return
				case err := <-errs:
					t.Fatal(err)
				case <-ticker.C:
					enqueueRuntimeCycle(trigger)
				case <-trigger:
					if err := l.runOnce(ctx); err != nil {
						t.Fatal(err)
					}
					cycles++
					if cycles > 8 {
						t.Fatalf("unchanged %s state self-triggered %d cycles", scenario, cycles)
					}
				}
			}
		})
	}
}

// No ticker: a timer cannot hide a broken real-input wakeup path.
func TestRuntimeInputWritesWakeAndApply(t *testing.T) {
	for _, name := range []string{"control.ndjson", "orders-next.json"} {
		t.Run(name, func(t *testing.T) {
			l := newOutputTestLoop(t, "failed")
			w, err := fsnotify.NewWatcher()
			if err != nil {
				t.Fatal(err)
			}
			defer w.Close()
			if err = w.Add(l.runtimeDir); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			trigger := make(chan struct{}, 1)
			errs := make(chan error, 1)
			done := make(chan struct{})
			go func() { defer close(done); l.forwardRuntimeWatcher(ctx, w.Events, w.Errors, trigger, errs) }()
			defer func() { cancel(); <-done }()
			payload := []byte("{\"orders\":[]}\n")
			if name == "control.ndjson" {
				payload = []byte("{\"id\":\"wake-test\",\"action\":\"pause\"}\n")
			}
			// A sibling-temp rename mirrors real atomic input publication.
			if err := filex.WriteFileAtomic(filepath.Join(l.runtimeDir, name), payload); err != nil {
				t.Fatal(err)
			}
			select {
			case <-trigger:
			case err := <-errs:
				t.Fatal(err)
			case <-time.After(2 * time.Second):
				t.Fatal("real input did not wake cycle")
			}
			if err := l.runOnce(ctx); err != nil {
				t.Fatal(err)
			}
			if name == "control.ndjson" {
				if l.state != StatePaused {
					t.Fatalf("control not applied: %s", l.state)
				}
				_, ack, _ := l.controlPaths()
				data, err := os.ReadFile(ack)
				if err != nil || len(data) == 0 {
					t.Fatalf("missing ack: %v", err)
				}
			} else {
				if !l.schedulePromoted {
					t.Fatal("orders-next not promoted")
				}
				if _, err := os.Stat(filepath.Join(l.runtimeDir, name)); !os.IsNotExist(err) {
					t.Fatalf("promoted input not removed: %v", err)
				}
			}
		})
	}
}

func TestFlushStableOutputsPreserveFiles(t *testing.T) {
	l := newOutputTestLoop(t, "failed")
	l.cmds.lastAppliedSeq = 7
	if err := l.flushState(); err != nil {
		t.Fatal(err)
	}
	names := []string{"orders.json", "state.json", "pending-review.json", "last-applied-seq"}
	stamp := time.Unix(1_000_000_000, 0)
	before := map[string]os.FileInfo{}
	for _, name := range names {
		path := filepath.Join(l.runtimeDir, name)
		if err := os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		before[name] = info
	}
	for i := 0; i < 3; i++ {
		if err := l.flushState(); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range names {
		info, err := os.Stat(filepath.Join(l.runtimeDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if !os.SameFile(before[name], info) || !info.ModTime().Equal(stamp) {
			t.Errorf("stable %s was rewritten", name)
		}
	}
}

func TestPendingReviewOutputChangesAndRepairs(t *testing.T) {
	l := newOutputTestLoop(t, "failed")
	l.canonical.PendingReviews = map[string]state.PendingReviewNode{"42": {OrderID: "42", WorktreeName: "42-work"}}
	if err := l.writePendingReview(); err != nil {
		t.Fatal(err)
	}
	items, err := ReadPendingReview(l.runtimeDir)
	if err != nil || len(items) != 1 || items[0].OrderID != "42" {
		t.Fatalf("review missing: %+v %v", items, err)
	}
	path := pendingReviewFilePath(l.runtimeDir)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	stamp := time.Unix(1_000_000_000, 0)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	if err := l.writePendingReview(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(stamp) {
		t.Error("populated review rewritten")
	}
	for _, damage := range []string{"delete", "corrupt"} {
		if damage == "delete" {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		} else if err := os.WriteFile(path, []byte("bad"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := l.writePendingReview(); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != string(data) {
			t.Fatalf("review not repaired after %s: %q %v", damage, got, err)
		}
	}
	delete(l.canonical.PendingReviews, "42")
	if err := l.writePendingReview(); err != nil {
		t.Fatal(err)
	}
	items, err = ReadPendingReview(l.runtimeDir)
	if err != nil || len(items) != 0 {
		t.Fatalf("review removal not persisted: %+v %v", items, err)
	}
}
