package projection

import (
	"github.com/poteto/noodle/internal/mode"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteProjectionFilesSkipsUnchangedOutputs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	bundle := mustProject(t, fixtureStateForProjection(), mode.ModeState{})
	if err := WriteProjectionFiles(dir, bundle); err != nil {
		t.Fatal(err)
	}
	stamp := time.Unix(1_000_000_000, 0)
	before := map[string]os.FileInfo{}
	for _, name := range []string{"orders.json", "state.json"} {
		path := filepath.Join(dir, name)
		if err := os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		before[name] = info
	}
	if err := WriteProjectionFiles(dir, bundle); err != nil {
		t.Fatal(err)
	}
	for name, old := range before {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if !os.SameFile(old, info) || !info.ModTime().Equal(stamp) {
			t.Errorf("unchanged %s replaced", name)
		}
	}
	bundle.OrdersProjection[0].Title = "changed title"
	if err := WriteProjectionFiles(dir, bundle); err != nil {
		t.Fatal(err)
	}
	ordersInfo, err := os.Stat(filepath.Join(dir, "orders.json"))
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(before["orders.json"], ordersInfo) {
		t.Error("changed order not persisted")
	}
	stateInfo, err := os.Stat(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before["state.json"], stateInfo) {
		t.Error("unrelated state marker rewritten")
	}
	for _, damage := range []string{"delete", "corrupt"} {
		path := filepath.Join(dir, "orders.json")
		expected, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if damage == "delete" {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		} else if err := os.WriteFile(path, []byte("bad"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := WriteProjectionFiles(dir, bundle); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != string(expected) {
			t.Fatalf("orders not repaired after %s: %q %v", damage, got, err)
		}
		info, err := os.Stat(filepath.Join(dir, "state.json"))
		if err != nil {
			t.Fatal(err)
		}
		if !os.SameFile(stateInfo, info) {
			t.Error("repair rewrote unrelated state marker")
		}
	}
}
