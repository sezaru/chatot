package transcribe

import (
	"context"
	"sync"
)

// Job is one transcription waiting its turn in a Queue.
type Job struct {
	// Key names the note (its message id); a second Enqueue of the same
	// key joins the first rather than running twice.
	Key      string
	Path     string
	CacheDir string
	// Requested marks a run someone clicked for: it goes ahead of every
	// automatic one, and is never dropped.
	Requested bool
	// OnStart runs when the engine takes the job; OnDone with its result.
	// Both are called on the queue's goroutine.
	OnStart func()
	OnDone  func(text string, err error)
}

// Queue runs transcriptions one at a time: requested ones first, in the
// order asked, then automatic ones newest first, since the note that just
// arrived matters more than the one from Tuesday. A chat full of voice
// notes therefore never starts a dozen engines at once, and a click
// never waits behind a backlog. Automatic jobs beyond MaxAuto are dropped
// (the bubble asks again the next time it is built), so scrolling through
// a week of notes queues a screenful, not the week.
type Queue struct {
	// MaxAuto caps the automatic jobs waiting; 0 means DefaultMaxAuto.
	MaxAuto int
	// run does the work; Transcribe by default, replaceable in tests.
	run func(ctx context.Context, cacheDir, path string) (string, error)

	mu        sync.Mutex
	requested []*Job
	auto      []*Job // newest last; popped from the end
	keys      map[string]*Job
	running   bool
	current   string
	// cancel stops the job running right now; nil between jobs.
	cancel context.CancelFunc
}

// DefaultMaxAuto is how many automatic jobs a Queue holds before dropping
// new ones.
const DefaultMaxAuto = 8

// Default is the app's queue.
var Default = NewQueue(Transcribe)

// NewQueue makes a queue whose jobs run through run.
func NewQueue(run func(ctx context.Context, cacheDir, path string) (string, error)) *Queue {
	return &Queue{run: run, keys: map[string]*Job{}}
}

// Enqueue adds job and reports whether it was taken: false for an
// automatic job when the automatic backlog is full. A job for a key that
// is already waiting is not added again; if the new one is requested, the
// waiting one is moved ahead of the automatic jobs. A key that is running
// right now is also reported as taken.
func (q *Queue) Enqueue(job Job) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.current == job.Key {
		return true
	}
	if old, ok := q.keys[job.Key]; ok {
		if job.Requested && !old.Requested {
			q.auto = remove(q.auto, old)
			old.Requested = true
			q.requested = append(q.requested, old)
		}
		return true
	}
	j := job
	if j.Requested {
		q.requested = append(q.requested, &j)
	} else {
		if len(q.auto) >= q.maxAuto() {
			return false
		}
		q.auto = append(q.auto, &j)
	}
	q.keys[j.Key] = &j
	if !q.running {
		q.running = true
		go q.drain()
	}
	return true
}

// Cancel stops key's transcription and reports whether there was one to
// stop. A job the engine is on is torn down where it runs, and its OnDone
// lands with context.Canceled; one that is only waiting is dropped without
// a call, since it never started. Either way the key is free again the
// moment this returns, so asking for the same note straight after is a new
// run rather than a join with the one being taken down.
func (q *Queue) Cancel(key string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.current == key {
		if q.cancel != nil {
			q.cancel()
		}
		q.current = ""
		return true
	}
	j, ok := q.keys[key]
	if !ok {
		return false
	}
	delete(q.keys, key)
	q.requested = remove(q.requested, j)
	q.auto = remove(q.auto, j)
	return true
}

// Pending is how many jobs are waiting, not counting the one running.
func (q *Queue) Pending() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.requested) + len(q.auto)
}

func (q *Queue) maxAuto() int {
	if q.MaxAuto > 0 {
		return q.MaxAuto
	}
	return DefaultMaxAuto
}

// next takes the job to run now, or nil when the queue is empty (the
// worker then stops).
func (q *Queue) next() *Job {
	q.mu.Lock()
	defer q.mu.Unlock()
	var j *Job
	switch {
	case len(q.requested) > 0:
		j = q.requested[0]
		q.requested = q.requested[1:]
	case len(q.auto) > 0:
		j = q.auto[len(q.auto)-1]
		q.auto = q.auto[:len(q.auto)-1]
	default:
		q.running = false
		q.current = ""
		return nil
	}
	delete(q.keys, j.Key)
	q.current = j.Key
	return j
}

// drain is the worker: one job after another until none is left. Each job
// runs under its own context so Cancel can stop that one alone, and a
// cancelled run reports context.Canceled however the engine happened to die
// (a killed whisper.cpp exits with a signal, which is not the caller's
// business).
func (q *Queue) drain() {
	for j := q.next(); j != nil; j = q.next() {
		if j.OnStart != nil {
			j.OnStart()
		}
		ctx, cancel := context.WithCancel(context.Background())
		q.mu.Lock()
		q.cancel = cancel
		q.mu.Unlock()
		text, err := q.run(ctx, j.CacheDir, j.Path)
		q.mu.Lock()
		q.cancel = nil
		q.mu.Unlock()
		if ctx.Err() != nil {
			text, err = "", context.Canceled
		}
		cancel()
		if j.OnDone != nil {
			j.OnDone(text, err)
		}
	}
}

func remove(jobs []*Job, j *Job) []*Job {
	for i, x := range jobs {
		if x == j {
			return append(jobs[:i:i], jobs[i+1:]...)
		}
	}
	return jobs
}
