package client

import (
	"context"
	"sync"
)

// flight runs one call per key at a time: a caller arriving while the key's
// call is in flight waits for that call's result instead of starting its
// own. The zero value is ready to use.
type flight struct {
	mu    sync.Mutex
	calls map[string]*flightCall
}

type flightCall struct {
	done chan struct{}
	val  string
	err  error
}

// do returns fn's result for key, running fn only if no call for key is in
// flight. A waiter whose ctx ends first returns ctx's error; the call goes
// on for the others.
func (f *flight) do(ctx context.Context, key string, fn func() (string, error)) (string, error) {
	f.mu.Lock()
	if c, ok := f.calls[key]; ok {
		f.mu.Unlock()
		select {
		case <-c.done:
			return c.val, c.err
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if f.calls == nil {
		f.calls = map[string]*flightCall{}
	}
	c := &flightCall{done: make(chan struct{})}
	f.calls[key] = c
	f.mu.Unlock()

	c.val, c.err = fn()

	f.mu.Lock()
	delete(f.calls, key)
	f.mu.Unlock()
	close(c.done)
	return c.val, c.err
}
