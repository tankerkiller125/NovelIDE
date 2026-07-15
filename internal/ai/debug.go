package ai

import (
	"log"
	"os"
)

// wireDebug dumps raw SSE events to stderr when NOVELIDE_AI_DEBUG is set, so we
// can see exactly what a provider sends (which may differ from what its own
// dashboard logs show, e.g. a proxy that transforms the response).
var wireDebug = os.Getenv("NOVELIDE_AI_DEBUG") != ""

func dbgf(format string, args ...any) {
	if wireDebug {
		log.Printf("[ai/wire] "+format, args...)
	}
}
