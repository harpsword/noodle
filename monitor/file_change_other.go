//go:build !darwin && !linux

package monitor

import (
	"os"
	"time"
)

// Without a trustworthy change time, preserve correctness by reading each time.
func fileChangeTime(os.FileInfo) (time.Time, bool) { return time.Time{}, false }
