// Package throttle keeps downloads under a speed limit, all of them
// together: a few downloads at full speed fill a home connection so that
// everything else on it, calls and games, stalls or drops.
package throttle

import (
	"context"
	"io"
	"sync"
	"time"
)

// Downloads is the limit every download of Hermit shares.
var Downloads = &Limiter{}

// Limiter shares a rate of bytes a second among the readers it wraps.
type Limiter struct {
	mu    sync.Mutex
	rate  int64 // bytes a second; 0 is no limit
	avail float64
	last  time.Time
}

// SetRate sets the limit, in bytes a second; zero or less lifts it.
func (l *Limiter) SetRate(bytesPerSecond int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.rate = max(0, bytesPerSecond)
	l.avail, l.last = 0, time.Now()
}

// Rate is the limit in bytes a second, 0 for none.
func (l *Limiter) Rate() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.rate
}

// Reader reads r no faster than the limit, with what else reads through l.
// Reading slower, it lets the connection's buffers fill, and the server
// sends slower in turn.
func (l *Limiter) Reader(ctx context.Context, r io.Reader) io.Reader {
	return &reader{l: l, ctx: ctx, r: r}
}

type reader struct {
	l   *Limiter
	ctx context.Context
	r   io.Reader
}

func (r *reader) Read(p []byte) (int, error) {
	// Reads of a tenth of a second at most, so that the pace is even.
	if rate := r.l.Rate(); rate > 0 {
		p = p[:min(len(p), max(1024, int(rate/10)))]
	}
	n, err := r.r.Read(p)
	if n > 0 {
		if werr := r.l.wait(r.ctx, n); werr != nil {
			return n, werr
		}
	}
	return n, err
}

// wait takes n bytes out of what the limit allows, waiting until it allows
// them.
func (l *Limiter) wait(ctx context.Context, n int) error {
	l.mu.Lock()
	if l.rate <= 0 {
		l.mu.Unlock()
		return nil
	}
	now := time.Now()
	rate := float64(l.rate)
	// At most a second's worth saved up, so a pause is not followed by a burst.
	l.avail = min(rate, l.avail+now.Sub(l.last).Seconds()*rate)
	l.last = now
	l.avail -= float64(n)
	var delay time.Duration
	if l.avail < 0 {
		delay = time.Duration(-l.avail / rate * float64(time.Second))
	}
	l.mu.Unlock()
	if delay <= 0 {
		return nil
	}
	t := time.NewTimer(delay)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
