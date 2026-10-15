package monitor

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"
)

const cacheSession = "history"
const cacheAction = `{"type":"action","message":"first","timestamp":"2026-10-15T12:00:00+05:30"}` + "\n"
const cacheResult = `{"type":"result","cost_usd":1.5,"tokens_in":20,"timestamp":"2026-10-15T12:00:01+05:30"}` + "\n"

func cacheFixture(t *testing.T, data string) (string, string) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "sessions", cacheSession, "canonical.ndjson")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	cacheWrite(t, path, data)
	return root, path
}
func cacheWrite(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
}
func cacheRead(t *testing.T, r *CanonicalClaimsReader) SessionClaims {
	t.Helper()
	c, err := r.ReadSession(cacheSession)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func requireCachePlatform(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("platform uses uncached correctness fallback")
	}
}
func countCanonicalOpens(r *CanonicalClaimsReader) *int {
	count := new(int)
	r.openCanonical = func(path string) (*os.File, error) { *count++; return os.Open(path) }
	return count
}

func TestCanonicalStableGenerationDoesNotReadAgain(t *testing.T) {
	requireCachePlatform(t)
	root, path := cacheFixture(t, cacheAction+cacheResult)
	r := NewCanonicalClaimsReader(root)
	reads := countCanonicalOpens(r)
	first := cacheRead(t, r)
	for range 4 {
		if got := cacheRead(t, r); !reflect.DeepEqual(got, first) {
			t.Fatalf("claims changed: %+v", got)
		}
	}
	if *reads != 1 {
		t.Fatalf("canonical opened %d times for unchanged source, want 1", *reads)
	}
	cacheWrite(t, filepath.Join(filepath.Dir(path), "spawn.json"), `{"model":"corrected","skill":"schedule","runtime":"sprites"}`)
	got := cacheRead(t, r)
	if got.Model != "corrected" || got.Skill != "schedule" || got.Runtime != "sprites" {
		t.Fatalf("spawn correction hidden: %+v", got)
	}
	if *reads != 1 {
		t.Fatalf("spawn correction reparsed canonical: %d", *reads)
	}
}

func TestCanonicalSourceChangesRecomputeClaims(t *testing.T) {
	for _, change := range []string{"append", "truncate", "replace", "correct", "remove-create"} {
		t.Run(change, func(t *testing.T) {
			root, path := cacheFixture(t, cacheAction)
			r := NewCanonicalClaimsReader(root)
			cacheRead(t, r)
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			wantAction, wantCost := "first", 0.0
			switch change {
			case "append":
				cacheWrite(t, path, cacheAction+cacheResult)
				wantCost = 1.5
			case "truncate":
				cacheWrite(t, path, "")
				wantAction = ""
			case "replace", "correct":
				replacement := `{"type":"action","message":"other","timestamp":"2026-10-15T12:00:00+05:30"}` + "\n"
				target := path
				if change == "replace" {
					target = path + ".next"
				}
				cacheWrite(t, target, replacement)
				if err := os.Chtimes(target, info.ModTime(), info.ModTime()); err != nil {
					t.Fatal(err)
				}
				if target != path {
					if err := os.Rename(target, path); err != nil {
						t.Fatal(err)
					}
				}
				wantAction = "other"
			case "remove-create":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if got := cacheRead(t, r); got.HasEvents {
					t.Fatal("removed log retained claims")
				}
				cacheWrite(t, path, cacheResult)
				wantAction = ""
				wantCost = 1.5
			}
			for range 2 {
				got := cacheRead(t, r)
				if got.LastAction != wantAction || got.TotalCostUSD != wantCost {
					t.Fatalf("stale claims: %+v", got)
				}
			}
		})
	}
}

func TestCanonicalParseFailureRetries(t *testing.T) {
	root, path := cacheFixture(t, cacheAction)
	r := NewCanonicalClaimsReader(root)
	cacheRead(t, r)
	cacheWrite(t, path, "{partial")
	for range 2 {
		if _, err := r.ReadSession(cacheSession); err == nil {
			t.Fatal("malformed source hidden")
		}
	}
	cacheWrite(t, path, cacheResult)
	if got := cacheRead(t, r); got.TotalCostUSD != 1.5 || got.LastAction != "" {
		t.Fatalf("failed recovery: %+v", got)
	}
}

func TestCanonicalReplacementDuringReadIsNotCached(t *testing.T) {
	root, path := cacheFixture(t, cacheAction)
	r := NewCanonicalClaimsReader(root)
	r.openCanonical = func(name string) (*os.File, error) {
		f, err := os.Open(name)
		if err != nil {
			return nil, err
		}
		cacheWrite(t, path+".next", cacheResult)
		if err := os.Rename(path+".next", path); err != nil {
			f.Close()
			t.Fatal(err)
		}
		return f, nil
	}
	cacheRead(t, r)
	r.openCanonical = os.Open
	if got := cacheRead(t, r); got.TotalCostUSD != 1.5 || got.LastAction != "" {
		t.Fatalf("reused replaced generation: %+v", got)
	}
}

// Use a recognizable old mtime for filesystem write assertions without sleeps.
func cacheOldTime() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
