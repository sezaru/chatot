package client

import (
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types/events"
)

// offlineProgressEvery spaces out the progress events of a backlog: one per
// message would flood the event bus, whose subscribers drop what overflows.
const offlineProgressEvery = 250 * time.Millisecond

// offlineProgress counts the backlog the server replays on connect. The
// server announces it (offline_preview), streams it through the same queue
// as live traffic, then marks its end (offline); whatsmeow hands the three
// over in that order.
type offlineProgress struct {
	mu       sync.Mutex
	active   bool
	state    OfflineSync
	lastPush time.Time
}

// preview starts a backlog of the announced size.
func (p *offlineProgress) preview(v *events.OfflineSyncPreview, now time.Time) OfflineSync {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.active = true
	p.state = OfflineSync{Total: v.Total, Messages: v.Messages}
	p.lastPush = now
	return p.state
}

// message counts one message filed while a backlog is running, and reports
// whether enough time has passed since the last report to send another.
func (p *offlineProgress) message(now time.Time) (OfflineSync, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.active {
		return OfflineSync{}, false
	}
	p.state.Received++
	if now.Sub(p.lastPush) < offlineProgressEvery {
		return OfflineSync{}, false
	}
	p.lastPush = now
	return p.state, true
}

// completed ends the backlog.
func (p *offlineProgress) completed() OfflineSync {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.active = false
	p.state.Done = true
	p.state.Received = p.state.Messages
	return p.state
}

// trackOfflineSync feeds evt to the backlog counter and publishes what the
// sync banner needs to know.
func (w *Whatsmeow) trackOfflineSync(evt interface{}) {
	var st OfflineSync
	switch v := evt.(type) {
	case *events.OfflineSyncPreview:
		st = w.catchUp.preview(v, time.Now())
	case *events.OfflineSyncCompleted:
		st = w.catchUp.completed()
	case *events.Message, *events.UndecryptableMessage:
		var ok bool
		if st, ok = w.catchUp.message(time.Now()); !ok {
			return
		}
	default:
		return
	}
	w.pushEvent(Event{Kind: EventOfflineSync, OfflineSync: &st})
}
