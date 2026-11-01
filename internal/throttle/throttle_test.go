package throttle

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"
)

func TestLimiterSharesItsRate(t *testing.T) {
	l := &Limiter{}
	l.SetRate(200 << 10) // 200 KB/s
	start := time.Now()
	var wg sync.WaitGroup
	// Two downloads of 200 KB together take about two seconds, not one.
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			io.Copy(io.Discard, l.Reader(context.Background(), bytes.NewReader(make([]byte, 200<<10))))
		}()
	}
	wg.Wait()
	if took := time.Since(start); took < 1500*time.Millisecond || took > 3*time.Second {
		t.Errorf("400 KB at 200 KB/s took %v", took)
	}

	l.SetRate(0)
	start = time.Now()
	io.Copy(io.Discard, l.Reader(context.Background(), bytes.NewReader(make([]byte, 10<<20))))
	if took := time.Since(start); took > 500*time.Millisecond {
		t.Errorf("without a limit 10 MB took %v", took)
	}

	l.SetRate(1 << 10)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := io.Copy(io.Discard, l.Reader(ctx, bytes.NewReader(make([]byte, 1<<20)))); err == nil {
		t.Error("a cancelled download went on")
	}
}
