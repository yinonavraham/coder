// Package clitiming contains facilities for debugging what's taking so long
// for a CLI command to complete.
package clitiming

import (
	"fmt"
	"os"
	"sync/atomic"
	"time"
)

var (
	start      = time.Now()
	lastRecord atomic.Pointer[time.Time]
)

// enabled cannot be set by a CLI option because it's used to debug
// the CLI itself.
//
// This is an internal flag and may change at any time.
var enabled = os.Getenv("CODER_DEBUG_TIMING") != ""

func Record(fmtStr string, args ...interface{}) {
	if !enabled {
		return
	}
	var (
		now           = time.Now()
		last          = lastRecord.Swap(&now)
		secSinceStart = time.Since(start).Seconds()
	)

	var secSinceLast float64
	if last != nil {
		secSinceLast = now.Sub(*last).Seconds()
	}
	_, _ = fmt.Fprintf(
		os.Stderr,
		"timing: %0.3fs %0.3fs: %s\n", secSinceStart, secSinceLast, fmt.Sprintf(fmtStr, args...),
	)
}
