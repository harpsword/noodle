package monitor

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestMonitorStableHistoryDoesNotWrite(t *testing.T) {
	root, canonical := cacheFixture(t, cacheAction+cacheResult)
	if err := os.Chtimes(canonical, cacheOldTime(), cacheOldTime()); err != nil {
		t.Fatal(err)
	}
	m := NewMonitor(root)
	m.tickets = &stubTicketMaterializer{}
	now := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return now }
	first, err := m.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	path := sessionMetaPath(root, cacheSession)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, cacheOldTime(), cacheOldTime()); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		now = now.Add(time.Minute)
		got, err := m.RunOnce(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || !reflect.DeepEqual(got[0], first[0]) {
			t.Errorf("unchanged terminal metadata changed: %+v", got)
		}
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) || !os.SameFile(info, stat) || !info.ModTime().Equal(stat.ModTime()) {
		t.Fatal("stable historical metadata rewritten")
	}
	// A fresh monitor reconstructs claims but must leave stable metadata untouched.
	restarted := NewMonitor(root)
	restarted.tickets = &stubTicketMaterializer{}
	restarted.now = m.now
	if _, err := restarted.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	stat, err = os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(info, stat) || !info.ModTime().Equal(stat.ModTime()) {
		t.Fatal("restart rewrote unchanged metadata")
	}
}

func TestMonitorReadsExternalMetadataChanges(t *testing.T) {
	root, path := cacheFixture(t, cacheAction+cacheResult)
	m := NewMonitor(root)
	m.tickets = &stubTicketMaterializer{}
	if _, err := m.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	metaPath := filepath.Join(filepath.Dir(path), "meta.json")
	previous, err := readSessionMeta(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	previous.RetryCount = 7
	if err := writeSessionMeta(metaPath, previous); err != nil {
		t.Fatal(err)
	}
	got, err := m.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got[0].RetryCount != 7 {
		t.Fatal("external retry count lost")
	}
	if err := os.Remove(metaPath); err != nil {
		t.Fatal(err)
	}
	if _, err := m.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(metaPath); err != nil {
		t.Fatal("removed meta not repaired", err)
	}
	cacheWrite(t, metaPath, "{broken")
	if _, err := m.RunOnce(context.Background()); err == nil {
		t.Fatal("corrupt metadata hidden")
	}
}

func TestTerminalClocksStopWithLiveProcess(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	claims := SessionClaims{HasEvents: true, Completed: true, FirstEventAt: now.Add(-time.Minute), LastEventAt: now.Add(-30 * time.Second)}
	for _, skill := range []string{"execute", "schedule"} {
		claims.Skill = skill
		a := DeriveSessionMeta("session", Observation{Alive: true}, claims, SessionMeta{}, now)
		b := DeriveSessionMeta("session", Observation{Alive: true}, claims, a, now.Add(time.Minute))
		if skill == "schedule" {
			if b.DurationSeconds <= a.DurationSeconds || !b.Alive {
				t.Fatal("scheduler clocks stopped")
			}
			continue
		}
		if a.DurationSeconds != 30 || b.DurationSeconds != 30 || a.IdleSeconds != 0 || b.IdleSeconds != 0 {
			t.Fatalf("terminal clocks kept ticking: %+v %+v", a, b)
		}
	}
}
