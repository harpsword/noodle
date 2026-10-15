package monitor

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"

	"github.com/poteto/noodle/internal/jsonx"
	"github.com/poteto/noodle/parse"
)

type spawnPayload struct {
	Runtime  string `json:"runtime"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Skill    string `json:"skill"`
}

const defaultMaxEventLineBytes = 64 << 20

type CanonicalClaimsReader struct {
	mu            sync.Mutex
	cache         map[string]canonicalCacheEntry
	runtimeDir    string
	maxLineBytes  int
	scanCanonical func(io.Reader, string) (SessionClaims, error)
}

func NewCanonicalClaimsReader(runtimeDir string) *CanonicalClaimsReader {
	reader := &CanonicalClaimsReader{
		cache:        make(map[string]canonicalCacheEntry),
		runtimeDir:   strings.TrimSpace(runtimeDir),
		maxLineBytes: defaultMaxEventLineBytes,
	}
	reader.scanCanonical = reader.parseCanonical
	return reader
}

func (r *CanonicalClaimsReader) ReadSession(sessionID string) (SessionClaims, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return SessionClaims{}, fmt.Errorf("session ID is required")
	}
	if r.runtimeDir == "" {
		return SessionClaims{}, fmt.Errorf("runtime directory is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	claims, exists, err := r.readCanonical(sessionID)
	if err != nil || !exists {
		return claims, err
	}
	if metadata, err := r.readSpawnMetadata(sessionID); err == nil {
		if claims.Skill == "" {
			claims.Skill = metadata.Skill
		}
		if claims.Runtime == "" {
			claims.Runtime = metadata.Runtime
		}
		if claims.Provider == "" {
			claims.Provider = metadata.Provider
		}
		if claims.Model == "" {
			claims.Model = metadata.Model
		}
	}

	return claims, nil
}

func (r *CanonicalClaimsReader) parseCanonical(file io.Reader, sessionID string) (SessionClaims, error) {
	claims := SessionClaims{SessionID: sessionID}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), r.maxLineBytes)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event parse.CanonicalEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return SessionClaims{}, fmt.Errorf("parse canonical event: %w", err)
		}

		claims.HasEvents = true
		if claims.FirstEventAt.IsZero() || event.Timestamp.Before(claims.FirstEventAt) {
			claims.FirstEventAt = event.Timestamp
		}
		if event.Timestamp.After(claims.LastEventAt) {
			claims.LastEventAt = event.Timestamp
		}
		if strings.TrimSpace(event.Provider) != "" {
			claims.Provider = strings.TrimSpace(event.Provider)
		}

		switch event.Type {
		case parse.EventAction:
			if message := strings.TrimSpace(event.Message); message != "" {
				claims.LastAction = message
			}
		case parse.EventResult:
			claims.TotalCostUSD += event.CostUSD
			claims.TokensIn += event.TokensIn
			claims.TokensOut += event.TokensOut
			claims.Completed = true
		case parse.EventComplete:
			claims.Completed = true
		}
	}
	if err := scanner.Err(); err != nil {
		return SessionClaims{}, fmt.Errorf("read canonical events: %w", err)
	}
	return claims, nil
}

func (r *CanonicalClaimsReader) readSpawnMetadata(sessionID string) (SessionClaims, error) {
	path := filepath.Join(r.runtimeDir, "sessions", sessionID, "spawn.json")
	payload, err := jsonx.ReadJSON[spawnPayload](path)
	if err != nil {
		return SessionClaims{}, err
	}
	return SessionClaims{
		Skill:    strings.TrimSpace(payload.Skill),
		Runtime:  strings.TrimSpace(payload.Runtime),
		Provider: strings.TrimSpace(payload.Provider),
		Model:    strings.TrimSpace(payload.Model),
	}, nil
}
