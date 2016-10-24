package main

import (
	"log"
	"testing"
	"time"
)

func Test_TimingWheel(t *testing.T) {
	w := newTimingWheel(100*time.Millisecond, 10)

	for {
		select {
		case <-w.after(200 * time.Millisecond):
			log.Println("1")
			return
		}
	}
}
