package uniway

//
// This code written by https://github.com/siddontang
// Copy from https://github.com/siddontang/go/blob/master/timingwheel/timingwheel.go
//

import (
	"sync"
	"time"
)

type timingWheel struct {
	sync.Mutex

	interval time.Duration

	ticker *time.Ticker
	quit   chan struct{}

	maxTimeout time.Duration

	cs []chan struct{}

	pos int
}

func newTimingWheel(interval time.Duration, buckets int) *timingWheel {
	w := new(timingWheel)

	w.interval = interval

	w.quit = make(chan struct{})
	w.pos = 0

	w.maxTimeout = time.Duration(interval * (time.Duration(buckets)))

	w.cs = make([]chan struct{}, buckets)

	for i := range w.cs {
		w.cs[i] = make(chan struct{})
	}

	w.ticker = time.NewTicker(interval)
	go w.run()

	return w
}

func (w *timingWheel) stop() {
	close(w.quit)
}

func (w *timingWheel) after(timeout time.Duration) <-chan struct{} {
	if timeout >= w.maxTimeout {
		panic("timeout too much, over maxtimeout")
	}

	index := int(timeout / w.interval)
	if 0 < index {
		index--
	}

	w.Lock()

	index = (w.pos + index) % len(w.cs)

	b := w.cs[index]

	w.Unlock()

	return b
}

func (w *timingWheel) run() {
	for {
		select {
		case <-w.ticker.C:
			w.onTicker()
		case <-w.quit:
			w.ticker.Stop()
			return
		}
	}
}

func (w *timingWheel) onTicker() {
	w.Lock()

	lastC := w.cs[w.pos]
	w.cs[w.pos] = make(chan struct{})

	w.pos = (w.pos + 1) % len(w.cs)

	w.Unlock()

	close(lastC)
}
