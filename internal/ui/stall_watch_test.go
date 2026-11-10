package ui

import (
	"bytes"
	"log"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
)

func TestGoroutineRunning(t *testing.T) {
	dump := strings.Join([]string{
		"goroutine 7 [select]:\nchatot/internal/ui.watchMainLoop.func1()\n\t/src/stall_watch.go:40",
		"goroutine 1 [syscall, locked to thread]:\nchatot/internal/store.(*Store).Chats(...)\ngithub.com/diamondburned/gotk4/pkg/gio/v2.(*Application).Run(0x1, {0x2})\nmain.main()",
		"goroutine 9 [IO wait]:\nnet.(*conn).Read()",
	}, "\n\n")
	got := string(goroutineRunning([]byte(dump), []byte("Application).Run(")))
	if !strings.HasPrefix(got, "goroutine 1 ") || !strings.Contains(got, "(*Store).Chats") {
		t.Fatalf("picked %q, want the goroutine running the application", got)
	}
	if g := goroutineRunning([]byte("goroutine 2 [running]:\nmain.other()"), []byte("Application).Run(")); g != nil {
		t.Fatalf("picked %q from a dump without the main loop", g)
	}
}

// A callback that holds the main loop past the threshold is reported while
// it runs, with its own frame in the stack, and again once the loop answers.
func TestWatchMainLoopReportsAStall(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var out syncBuffer
	log.SetOutput(&out)
	defer log.SetOutput(os.Stderr)

	WatchMainLoop()
	ctx := glib.MainContextDefault()
	pump := func(d time.Duration) {
		for end := time.Now().Add(d); time.Now().Before(end); {
			ctx.Iteration(false)
			time.Sleep(5 * time.Millisecond)
		}
	}
	pump(600 * time.Millisecond)
	glib.IdleAdd(func() { holdMainLoopForTest(1500 * time.Millisecond) })
	pump(2 * time.Second)

	got := out.String()
	for _, want := range []string{"main loop stuck for", "holdMainLoopForTest", "main loop answered again after"} {
		if !strings.Contains(got, want) {
			t.Errorf("log lacks %q:\n%s", want, got)
		}
	}
}

//go:noinline
func holdMainLoopForTest(d time.Duration) { time.Sleep(d) }

// syncBuffer is a bytes.Buffer the watcher's goroutine and the test can
// share.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
