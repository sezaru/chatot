package client

import (
	"context"
	"fmt"
	"time"

	"go.mau.fi/whatsmeow"
)

// wakeTick is how often the wake watcher reads the wall clock; wakeJump is
// the gap between two reads that means the machine slept in between (a
// busy main loop never holds a goroutine's timer for anything close to
// it).
const (
	wakeTick = 5 * time.Second
	wakeJump = 30 * time.Second
)

// reconnectRetryMin and reconnectRetryMax bound the pause between two
// connection attempts after a wake or a dead socket: the network is
// usually still coming up when the first one runs.
const (
	reconnectRetryMin = 2 * time.Second
	reconnectRetryMax = 30 * time.Second
)

// watchWake reconnects the session after the machine slept. whatsmeow
// notices a socket that died during a suspend only through its keepalive
// pings, and forces a reconnect after three minutes of failed ones; the
// timers behind those pings run on the monotonic clock, which stops while
// the machine is suspended, so from their point of view nothing happened.
// The wall clock does jump, and that jump is the signal. Runs until ctx
// ends.
func (w *Whatsmeow) watchWake(ctx context.Context) {
	last := time.Now()
	ticker := time.NewTicker(wakeTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if jumped, gap := clockJumped(last, now); jumped {
				go w.reconnect(ctx, w.currentWA(), fmt.Sprintf("the clock jumped %s, the machine slept", gap.Round(time.Second)))
			}
			last = now
		}
	}
}

// clockJumped reports whether the wall clock moved from prev to now by
// more than a tick could take (wakeJump), and by how much. The comparison
// strips the monotonic readings: they are the clock that does not count
// a suspend.
func clockJumped(prev, now time.Time) (bool, time.Duration) {
	gap := now.Round(0).Sub(prev.Round(0))
	return gap > wakeJump, gap
}

// currentWA is the session in use, under the relink lock.
func (w *Whatsmeow) currentWA() *whatsmeow.Client {
	w.relinkMu.Lock()
	defer w.relinkMu.Unlock()
	return w.wa
}

// reconnect drops wa's socket and opens a new one, retrying with a growing
// pause while ctx lives, the session is still wa and the attempt fails
// (right after a wake the network is often still down). One reconnect
// runs at a time: a second trigger while one is under way (a keepalive
// timing out during the retries) is dropped. Nothing happens for a
// session that is not linked or is handing out pairing codes.
func (w *Whatsmeow) reconnect(ctx context.Context, wa *whatsmeow.Client, reason string) {
	if wa == nil || wa.Store.ID == nil || w.pairing.Load() || ctx.Err() != nil {
		return
	}
	if !w.reconnecting.CompareAndSwap(false, true) {
		return
	}
	defer w.reconnecting.Store(false)
	w.log.Infof("chatot/client: reconnecting: %s", reason)
	delay := reconnectRetryMin
	for {
		if w.currentWA() != wa {
			return
		}
		wa.Disconnect()
		err := wa.Connect()
		if err == nil {
			w.log.Infof("chatot/client: reconnected")
			return
		}
		w.log.Warnf("chatot/client: reconnect: %v (retrying in %s)", err, delay)
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		delay = nextReconnectDelay(delay)
	}
}

// nextReconnectDelay doubles the pause between attempts up to
// reconnectRetryMax.
func nextReconnectDelay(d time.Duration) time.Duration {
	return min(d*2, reconnectRetryMax)
}
