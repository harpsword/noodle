package monitor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestMonitorLivenessChild(t *testing.T) {
	if os.Getenv("NOODLE_TEST_MONITOR_CHILD") != "1" {
		return
	}
	time.Sleep(time.Hour)
}

func TestMonitorDetectsProcessDeathWithoutNewLogs(t *testing.T) {
	root, path := cacheFixture(t, cacheAction)
	child := exec.Command(os.Args[0], "-test.run=^TestMonitorLivenessChild$")
	child.Env = append(os.Environ(), "NOODLE_TEST_MONITOR_CHILD=1")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = child.Process.Kill() })
	cacheWrite(t, filepath.Join(filepath.Dir(path), "process.json"), fmt.Sprintf(`{"pid":%d}`, child.Process.Pid))
	m := NewMonitor(root)
	m.tickets = &stubTicketMaterializer{}
	reads := countCanonicalScans(m.claims.(*CanonicalClaimsReader))
	first, err := m.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !first[0].Alive || first[0].Status != SessionStatusRunning {
		t.Fatalf("child not detected: %+v", first)
	}
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = child.Wait()
	second, err := m.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if second[0].Alive || second[0].Status != SessionStatusExited {
		t.Fatalf("death hidden: %+v", second)
	}
	requireCachePlatform(t)
	if *reads != 1 {
		t.Fatalf("liveness check reread canonical %d times", *reads)
	}
}

func TestMonitorHeartbeatExpiresWithoutSourceChange(t *testing.T) {
	root, path := cacheFixture(t, cacheAction)
	now := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	cacheWrite(t, filepath.Join(filepath.Dir(path), "spawn.json"), `{"runtime":"sprites"}`)
	cacheWrite(t, filepath.Join(filepath.Dir(path), "heartbeat.json"), fmt.Sprintf(`{"timestamp":%q,"ttl_seconds":30}`, now.Format(time.RFC3339)))
	m := NewMonitor(root)
	m.tickets = &stubTicketMaterializer{}
	m.now = func() time.Time { return now }
	heartbeat := NewHeartbeatObserver(root)
	heartbeat.now = m.now
	m.observer = NewCompositeObserver(root, NewPidObserver(root), heartbeat)
	reads := countCanonicalScans(m.claims.(*CanonicalClaimsReader))
	first, err := m.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !first[0].Alive {
		t.Fatal("fresh heartbeat not alive")
	}
	now = now.Add(61 * time.Second)
	second, err := m.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if second[0].Alive || second[0].Status != SessionStatusExited {
		t.Fatalf("expiry hidden: %+v", second)
	}
	requireCachePlatform(t)
	if *reads != 1 {
		t.Fatalf("expiry reread canonical %d times", *reads)
	}
}

func TestMonitorSpawnCorrectionChangesPolicy(t *testing.T) {
	root, path := cacheFixture(t, cacheAction+cacheResult)
	spawn := filepath.Join(filepath.Dir(path), "spawn.json")
	cacheWrite(t, spawn, `{"runtime":"process","skill":"schedule","model":"before"}`)
	m := NewMonitor(root)
	m.tickets = &stubTicketMaterializer{}
	alive := stubObserver{observations: map[string]Observation{cacheSession: {SessionID: cacheSession, Alive: true}}}
	m.observer = NewCompositeObserver(root, alive, stubObserver{})
	first, err := m.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !first[0].Alive {
		t.Fatal("persistent schedule marked terminal")
	}
	cacheWrite(t, spawn, `{"runtime":"process","skill":"execute","model":"after"}`)
	second, err := m.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if second[0].Alive || second[0].Model != "after" {
		t.Fatalf("policy correction hidden: %+v", second)
	}
	// Restore schedule but switch runtime to the dead remote observer.
	cacheWrite(t, spawn, `{"runtime":"sprites","skill":"schedule","model":"remote"}`)
	third, err := m.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if third[0].Alive || third[0].Runtime != "sprites" {
		t.Fatalf("observer switch hidden: %+v", third)
	}
}

func TestMonitorConcurrentRunsAndCachePruning(t *testing.T) {
	root, path := cacheFixture(t, cacheAction+cacheResult)
	m := NewMonitor(root)
	m.tickets = &stubTicketMaterializer{}
	var group sync.WaitGroup
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			if _, err := m.RunOnce(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	reader := m.claims.(*CanonicalClaimsReader)
	if err := os.RemoveAll(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	if _, err := m.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(reader.cache) != 0 {
		t.Fatal("removed session retained in cache")
	}
}
