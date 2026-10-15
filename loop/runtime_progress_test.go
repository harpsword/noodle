package loop

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/poteto/noodle/config"
	"github.com/poteto/noodle/internal/ingest"
	"github.com/poteto/noodle/mise"
	loopruntime "github.com/poteto/noodle/runtime"
)

// These exercise Run's ticker, not manually invoked cycles. Once startup has
// settled the test only queues an in-memory result: neither orders-next.json
// nor control.ndjson is written to wake the loop.
func TestRunProcessesResultsWithoutInputWrites(t *testing.T) {
	for _, resultKind := range []string{"completion", "merge"} {
		t.Run(resultKind, func(t *testing.T) {
			projectDir := t.TempDir()
			runtimeDir := filepath.Join(projectDir, ".noodle")
			if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
				t.Fatal(err)
			}
			cfg := config.DefaultConfig()
			cfg.Mode = "auto"
			cfg.Skills.Paths = []string{filepath.Join(projectDir, "skills")}
			if err := os.MkdirAll(cfg.Skills.Paths[0], 0o755); err != nil {
				t.Fatal(err)
			}
			order := testOrder("progress", "execute", "execute", "claude", "claude-sonnet-4-6")
			schedule := scheduleOrder(cfg, "")
			schedule.Status = OrderStatusFailed
			schedule.Stages[0].Status = StageStatusFailed
			writeTestOrders(t, runtimeDir, OrdersFile{Orders: []Order{order, schedule}})

			session := &progressSession{mockSession: &mockSession{
				id: "progress-session", status: "running", done: make(chan struct{}),
			}}
			cook := &cookHandle{
				cookIdentity: cookIdentity{orderID: order.ID, stage: order.Stages[0]},
				session:      session, worktreeName: "progress-worktree", generation: 1,
			}
			brief := &progressMise{}
			l := New(projectDir, "noodle", cfg, Dependencies{
				Runtimes: map[string]loopruntime.Runtime{"process": newMockRuntime()},
				Worktree: &fakeWorktree{hasUnmergedCommits: map[string]bool{cook.worktreeName: false}},
				Adapter:  &fakeAdapterRunner{}, Mise: brief, Monitor: fakeMonitor{},
				Registry: testLoopRegistry(), Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			})
			// Install the fixture on Run's goroutine after startup reconciliation.
			// Emit the normal lifecycle events so the canonical state remains valid.
			brief.setup = func() error {
				payload := map[string]any{
					"order_id": order.ID, "stage_index": 0, "attempt_id": "progress-attempt",
					"session_id": session.ID(), "worktree_name": cook.worktreeName,
				}
				for _, kind := range []ingest.EventType{ingest.EventDispatchRequested, ingest.EventDispatchCompleted} {
					if err := l.emitEventChecked(kind, payload); err != nil {
						return err
					}
				}
				if resultKind == "merge" {
					if err := l.emitEventChecked(ingest.EventStageCompleted, l.mergeLifecyclePayload(cook, true)); err != nil {
						return err
					}
				} else {
					l.trackCookStarted(cook)
					l.cooks.activeCooksByOrder[order.ID] = cook
				}
				return l.mirrorLegacyOrderFromCanonical(order.ID)
			}
			// The merge worker needs no filesystem work; the test concerns delivery
			// of its result to Run, not Git merging itself.
			l.mergeQueue.Close()
			l.mergeQueue = NewMergeQueue(context.Background(), func(context.Context, MergeRequest) error { return nil })
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			go func() { done <- l.Run(ctx) }()
			t.Cleanup(func() {
				cancel()
				select {
				case err := <-done:
					l.Shutdown()
					if err != nil {
						t.Errorf("Run: %v", err)
					}
				case <-time.After(10 * time.Second):
					t.Error("Run did not shut down")
				}
			})

			waitProgress(t, func() bool { return brief.calls.Load() >= 2 })
			// Allow the already-started cycle to finish before delivering a result.
			time.Sleep(100 * time.Millisecond)
			before := l.State()
			if before.Status != string(StateRunning) {
				t.Fatalf("loop status = %s, want running", before.Status)
			}
			if progressOrderStatus(before, order.ID) == "" {
				t.Fatal("fixture order disappeared before result delivery")
			}
			if resultKind == "completion" {
				l.enqueueCompletion(ctx, StageResult{
					OrderID: order.ID, SessionID: session.ID(), Generation: cook.generation,
					Status: StageResultCompleted, CompletedAt: time.Now(),
				})
			} else {
				l.mergeQueue.Enqueue(MergeRequest{Cook: cook})
			}
			// A generous deadline includes the default one-second poll and cycle
			// work; this deliberately makes no strict one-second latency claim.
			waitProgress(t, func() bool { return progressOrderStatus(l.State(), order.ID) == "completed" })
			if _, err := os.Stat(filepath.Join(runtimeDir, "orders-next.json")); !os.IsNotExist(err) {
				t.Fatalf("unexpected orders-next input: %v", err)
			}
		})
	}
}

func progressOrderStatus(snapshot LoopState, id string) string {
	for _, order := range snapshot.Projection.Orders {
		if order.ID == id {
			return string(order.Status)
		}
	}
	return ""
}

func waitProgress(t *testing.T, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for !ready() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for loop progress")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

type progressMise struct {
	setup func() error
	calls atomic.Int64
}

func (m *progressMise) Build(context.Context, mise.ActiveSummary, []mise.HistoryItem) (mise.Brief, []string, bool, error) {
	if m.calls.Load() == 0 {
		if err := m.setup(); err != nil {
			return mise.Brief{}, nil, false, err
		}
	}
	m.calls.Add(1)
	return mise.Brief{}, nil, false, nil
}

// Outcome is immutable; shutdown only closes Done, avoiding mockSession's
// unsynchronized status writes while the loop publishes snapshots.
type progressSession struct {
	*mockSession
	once sync.Once
}

func (s *progressSession) Terminate() error {
	s.once.Do(func() { close(s.done) })
	return nil
}

func (s *progressSession) ForceKill() error { return s.Terminate() }
