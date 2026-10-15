package loop

import (
	"os"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

func TestProcessControlCommandsEmptyQueueDoesNotWrite(t *testing.T) {
	l := newControlTestLoop(t, &fakeWorktree{}, newMockRuntime())
	controlPath, _, _ := l.controlPaths()
	if err := os.WriteFile(controlPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	// A fixed old timestamp detects a rewrite without relying on clock resolution.
	stamp := time.Unix(1_000_000_000, 0)
	if err := os.Chtimes(controlPath, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := l.processControlCommands(); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(controlPath)
		if err != nil {
			t.Fatal(err)
		}
		if !info.ModTime().Equal(stamp) {
			t.Fatalf("empty control queue was rewritten on pass %d", i+1)
		}
	}
}

func TestProcessControlCommandsDrainedQueueDoesNotRetrigger(t *testing.T) {
	l := newControlTestLoop(t, &fakeWorktree{}, newMockRuntime())
	controlPath, ackPath, _ := l.controlPaths()
	if err := os.WriteFile(controlPath, []byte("{\"id\":\"idle-probe\",\"action\":\"pause\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := l.processControlCommands(); err != nil {
		t.Fatal(err)
	}
	if l.state != StatePaused {
		t.Fatalf("state = %s, want paused", l.state)
	}
	acks, err := os.ReadFile(ackPath)
	if err != nil || len(acks) == 0 {
		t.Fatalf("missing control acknowledgement: %v", err)
	}
	data, err := os.ReadFile(controlPath)
	if err != nil || len(data) != 0 {
		t.Fatalf("control queue was not drained: %q, %v", data, err)
	}

	// Watch the same directory as Loop.Run, after the legitimate drain event.
	// Retrying that event must not generate another event for the control file.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Close()
	if err := watcher.Add(l.runtimeDir); err != nil {
		t.Fatal(err)
	}
	if err := l.processControlCommands(); err != nil {
		t.Fatal(err)
	}
	timer := time.NewTimer(200 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case ev := <-watcher.Events:
			if shouldTriggerRuntimeCycle(ev.Name) {
				t.Fatalf("drained control queue retriggered a cycle: %s", ev)
			}
		case err := <-watcher.Errors:
			t.Fatal(err)
		case <-timer.C:
			return
		}
	}
}
