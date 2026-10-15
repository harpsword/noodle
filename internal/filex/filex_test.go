package filex

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteFileAtomicCreatesParentAndFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "a", "b", "payload.json")
	if err := WriteFileAtomic(path, []byte(`{"ok":true}`)); err != nil {
		t.Fatalf("write atomic file: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(got) != `{"ok":true}` {
		t.Fatalf("file contents mismatch: got %q", string(got))
	}
}

func TestWriteFileAtomicReplacesExistingContents(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "orders.json")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	if err := WriteFileAtomic(path, []byte("new")); err != nil {
		t.Fatalf("rewrite file: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rewritten file: %v", err)
	}
	if string(got) != "new" {
		t.Fatalf("file contents mismatch: got %q", string(got))
	}
}

func TestWriteFileAtomicIfChangedPreservesAndReplaces(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "nested", "output")
	if err := WriteFileAtomicIfChanged(path, []byte("same")); err != nil {
		t.Fatal(err)
	}
	stamp := time.Unix(1_000_000_000, 0)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomicIfChanged(path, []byte("same")); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) || !after.ModTime().Equal(stamp) {
		t.Fatal("identical output was replaced")
	}
	if err := WriteFileAtomicIfChanged(path, []byte("changed")); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "changed" {
		t.Fatalf("changed bytes not written: %q %v", got, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomicIfChanged(path, []byte("changed")); err != nil {
		t.Fatal(err)
	}
	got, err = os.ReadFile(path)
	if err != nil || string(got) != "changed" {
		t.Fatalf("deleted output not restored: %q %v", got, err)
	}
}

func TestWriteFileAtomicIfChangedPropagatesReplacementFailure(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "directory")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomicIfChanged(path, []byte("data")); err == nil {
		t.Fatal("replacement of directory succeeded")
	}
	files, err := filepath.Glob(path + ".tmp.*")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("failed replacement left temp files: %v", files)
	}
}
