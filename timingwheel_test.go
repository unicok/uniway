package uiway

import (
	"log"
	"testing"
	"time"
)

func Test_TimingWheel(t *testing.T) {
	w := NewTimingWheel(100*time.Millisecond, 10)

	for {
		select {
		case <-w.After(200 * time.Millisecond):
			log.Println("1")
			return
		}
	}
}
