package monitor

import (
	"fmt"
	"os"
	"path/filepath"
)

type canonicalCacheEntry struct {
	info   os.FileInfo
	claims SessionClaims
}

// readCanonical caches only canonical-derived fields. Spawn policy is always fresh.
// The caller holds r.mu; unchanged logs are statted but never opened or scanned.
func (r *CanonicalClaimsReader) readCanonical(sessionID string) (SessionClaims, bool, error) {
	path := filepath.Join(r.runtimeDir, "sessions", sessionID, "canonical.ndjson")
	current, err := os.Stat(path)
	if err != nil {
		delete(r.cache, sessionID)
		if os.IsNotExist(err) {
			return SessionClaims{SessionID: sessionID}, false, nil
		}
		return SessionClaims{}, false, fmt.Errorf("stat canonical events: %w", err)
	}
	if entry, ok := r.cache[sessionID]; ok && sameCanonicalGeneration(entry.info, current) {
		return entry.claims, true, nil
	}
	delete(r.cache, sessionID)
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return SessionClaims{SessionID: sessionID}, false, nil
		}
		return SessionClaims{}, false, fmt.Errorf("open canonical events: %w", err)
	}
	defer file.Close()
	before, err := file.Stat()
	if err != nil {
		return SessionClaims{}, false, fmt.Errorf("stat opened canonical events: %w", err)
	}
	claims, err := r.scanCanonical(file, sessionID)
	if err != nil {
		return SessionClaims{}, false, err
	}
	after, fileErr := file.Stat()
	latest, pathErr := os.Stat(path)
	// A concurrent mutation may yield a transient snapshot, never a reusable cache entry.
	if fileErr == nil && pathErr == nil && sameCanonicalGeneration(before, after) && sameCanonicalGeneration(after, latest) {
		r.cache[sessionID] = canonicalCacheEntry{info: after, claims: claims}
	}
	return claims, true, nil
}

func sameCanonicalGeneration(a, b os.FileInfo) bool {
	aTime, aSupported := fileChangeTime(a)
	bTime, bSupported := fileChangeTime(b)
	return aSupported && bSupported && os.SameFile(a, b) && a.Size() == b.Size() &&
		a.Mode() == b.Mode() && a.ModTime().Equal(b.ModTime()) && aTime.Equal(bTime)
}

func (r *CanonicalClaimsReader) pruneCache(sessionIDs []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	present := make(map[string]struct{}, len(sessionIDs))
	for _, id := range sessionIDs {
		present[id] = struct{}{}
	}
	for id := range r.cache {
		if _, ok := present[id]; !ok {
			delete(r.cache, id)
		}
	}
}
