package app

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/Unfield/Tern/kit"
)

// Petrel talks to mail servers on users' behalf, so it must not become a
// way to guess mailbox passwords or to probe the internet. Tern limits
// every POST to 120 a minute per client; these limits are on top, per
// mailbox address, client and user.

// window counts events per key over a sliding window.
type window struct {
	mu    sync.Mutex
	max   int
	span  time.Duration
	hits  map[string][]time.Time
	swept time.Time
}

func newWindow(max int, span time.Duration) *window {
	return &window{max: max, span: span, hits: map[string][]time.Time{}}
}

// recent drops the old events of key and returns the rest. Callers hold mu.
func (w *window) recent(key string, now time.Time) []time.Time {
	if now.Sub(w.swept) > w.span { // forget keys that went quiet
		for k, list := range w.hits {
			if len(list) == 0 || now.Sub(list[len(list)-1]) > w.span {
				delete(w.hits, k)
			}
		}
		w.swept = now
	}
	list := w.hits[key]
	i := 0
	for i < len(list) && now.Sub(list[i]) > w.span {
		i++
	}
	list = list[i:]
	w.hits[key] = list
	return list
}

// Blocked reports how long until key may try again, or 0.
func (w *window) Blocked(key string) time.Duration {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := time.Now()
	list := w.recent(key, now)
	if len(list) < w.max {
		return 0
	}
	return w.span - now.Sub(list[len(list)-w.max])
}

// Add counts an event for key.
func (w *window) Add(key string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := time.Now()
	w.hits[key] = append(w.recent(key, now), now)
}

// Take counts an event for key if it isn't blocked.
func (w *window) Take(key string) time.Duration {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := time.Now()
	list := w.recent(key, now)
	if len(list) >= w.max {
		return w.span - now.Sub(list[len(list)-w.max])
	}
	w.hits[key] = append(list, now)
	return 0
}

// Reset forgets key, after a successful sign-in.
func (w *window) Reset(key string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.hits, key)
}

var limits = struct {
	signInAddress, signInClient *window // failed mailbox sign-ins
	connect                     *window // mailbox tests when connecting, per user
	discover                    *window // settings lookups, per user
}{
	signInAddress: newWindow(5, 15*time.Minute),
	signInClient:  newWindow(20, 15*time.Minute),
	connect:       newWindow(20, 15*time.Minute),
	discover:      newWindow(30, 10*time.Minute),
}

// tooMany is the error for a limit, with when to try again.
func tooMany(ctx *kit.Ctx, wait time.Duration) error {
	ctx.SetHeader("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
	return &kit.HTTPError{Status: http.StatusTooManyRequests, Message: ctx.TB("tern.accounts.error.tooMany", nil)}
}

// LimitDiscover allows a user 30 settings lookups in 10 minutes: each one
// asks DNS and web servers of a domain the user chose.
func LimitDiscover(ctx *kit.Ctx) error {
	if wait := limits.discover.Take(strconv.FormatInt(UserID(ctx), 10)); wait > 0 {
		return tooMany(ctx, wait)
	}
	return nil
}

// LimitConnect allows a user 20 tries at connecting a mailbox in 15 minutes.
func LimitConnect(ctx *kit.Ctx) error {
	if wait := limits.connect.Take(strconv.FormatInt(UserID(ctx), 10)); wait > 0 {
		ctx.SetHeader("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		return kit.FormError(ctx.TB("tern.accounts.error.tooMany", nil))
	}
	return nil
}
