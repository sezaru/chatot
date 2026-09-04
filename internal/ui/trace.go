package ui

import (
	"log"
	"os"
)

// traceLevel is CHATOT_TRACE: 0 off, 1 scroll/paging events, 2 also every
// row bind. Dev diagnostics; nothing in production paths reads it beyond
// the cheap check.
var traceLevel = func() int {
	switch os.Getenv("CHATOT_TRACE") {
	case "", "0":
		return 0
	case "2":
		return 2
	}
	return 1
}()

func trace(level int, format string, args ...any) {
	if traceLevel >= level {
		log.Printf("trace "+format, args...)
	}
}
