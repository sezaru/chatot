package client

import (
	"log"
	"sync/atomic"

	waLog "go.mau.fi/whatsmeow/util/log"
)

// verbose gates whatsmeow's info and debug lines. Warnings and errors
// always print. Preferences flips it at runtime.
var verbose atomic.Bool

// SetVerboseLogging turns whatsmeow's info and debug output on or off for
// every client in the process.
func SetVerboseLogging(on bool) { verbose.Store(on) }

// stdLogger routes whatsmeow's logger through the standard log package, so
// the lines land wherever the app sends its log (stderr and the capped log
// file) instead of whatsmeow's own stdout writer.
type stdLogger struct {
	module string
}

// newLogger returns the logger for a whatsmeow module.
func newLogger(module string) waLog.Logger { return stdLogger{module: module} }

func (l stdLogger) Errorf(msg string, args ...any) { l.printf("ERROR", msg, args...) }
func (l stdLogger) Warnf(msg string, args ...any)  { l.printf("WARN", msg, args...) }

func (l stdLogger) Infof(msg string, args ...any) {
	if verbose.Load() {
		l.printf("INFO", msg, args...)
	}
}

func (l stdLogger) Debugf(msg string, args ...any) {
	if verbose.Load() {
		l.printf("DEBUG", msg, args...)
	}
}

func (l stdLogger) Sub(module string) waLog.Logger {
	return stdLogger{module: l.module + "/" + module}
}

func (l stdLogger) printf(level, msg string, args ...any) {
	log.Printf("["+l.module+" "+level+"] "+msg, args...)
}
