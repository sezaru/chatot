package ui

import "sync"

// thumbQueue runs the media grid's tile decodes and fetches off the main
// loop. A chat can hold thousands of pictures, so only tiles on screen ask
// for a face, the newest request goes first (the tiles just scrolled to),
// and a request whose tile scrolled away before its turn is dropped: the
// tile asks again if it comes back.
type thumbQueue struct {
	mu      sync.Mutex
	jobs    []thumbJob
	queued  map[string]bool
	wanted  map[string]int // bound tiles per key
	running int
	workers int
	// post hands a finished job's result to the main loop.
	post func(func())
}

// thumbJob is one tile face to make. run works off the main loop and
// returns what to do with the result back on it (nil for nothing).
type thumbJob struct {
	key string
	run func() func()
}

func newThumbQueue(workers int, post func(func())) *thumbQueue {
	return &thumbQueue{workers: workers, post: post, queued: map[string]bool{}, wanted: map[string]int{}}
}

// want records a tile bound to key; unwant its unbinding.
func (q *thumbQueue) want(key string) {
	q.mu.Lock()
	q.wanted[key]++
	q.mu.Unlock()
}

func (q *thumbQueue) unwant(key string) {
	q.mu.Lock()
	if q.wanted[key]--; q.wanted[key] <= 0 {
		delete(q.wanted, key)
	}
	q.mu.Unlock()
}

// push queues j unless a job for its key is already waiting.
func (q *thumbQueue) push(j thumbJob) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.queued[j.key] {
		return
	}
	q.queued[j.key] = true
	q.jobs = append(q.jobs, j)
	if q.running < q.workers {
		q.running++
		go q.work()
	}
}

// clear drops every waiting job (the page moved to another chat).
func (q *thumbQueue) clear() {
	q.mu.Lock()
	q.jobs = nil
	q.queued = map[string]bool{}
	q.mu.Unlock()
}

// next pops the newest job still wanted, dropping the unwanted ones it
// passes. ok is false once nothing is left.
func (q *thumbQueue) next() (j thumbJob, ok bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.jobs) > 0 {
		j = q.jobs[len(q.jobs)-1]
		q.jobs = q.jobs[:len(q.jobs)-1]
		delete(q.queued, j.key)
		if q.wanted[j.key] > 0 {
			return j, true
		}
	}
	return thumbJob{}, false
}

func (q *thumbQueue) work() {
	for {
		j, ok := q.next()
		if !ok {
			q.mu.Lock()
			// A push between next and here saw running at the cap and
			// started no worker: look once more before leaving.
			if len(q.jobs) > 0 {
				q.mu.Unlock()
				continue
			}
			q.running--
			q.mu.Unlock()
			return
		}
		if done := j.run(); done != nil {
			q.post(done)
		}
	}
}
