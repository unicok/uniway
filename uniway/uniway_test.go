package main

import (
	"sync"
	"testing"

	uw "github.com/unicok/uniway"
)

func Test_DialServer(t *testing.T) {
	config := uw.DefaultEPConfig()
	config.ServerID = 1
	// config.AuthKey = "test"

	gw, err := uw.DialServer("tcp", "127.0.0.1:25595", config)
	if err != nil {
		t.Error(err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		for {
			wg.Done()
			conn, connID, clientID, err := gw.Accept()
			if err != nil {
				t.Error(err)
			}
			println("connID: ", connID, " clientID: ", clientID)
			buf, err := conn.Receive()
			if err != nil {
				return
			}

			msg := *(buf.(*[]byte))
			conn.Send(msg)
		}
	}()
	wg.Wait()
}
