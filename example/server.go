package main

import (
	"log"
	"time"

	"github.com/unicok/slab"
	"github.com/unicok/uniway"
)

func main() {

	pool := slab.NewAtomPool(64, 64*1024, 2, 1024*1024)

	server, err := uniway.DialServer("tcp", "127.0.0.1:10020",
		uniway.EndPointCfg{
			ServerID:     10086,
			MemPool:      pool,
			MaxPacket:    512 * 1024,
			BufferSize:   1024,
			SendChanSize: 10000,
			RecvChanSize: 10000,
			PingInterval: time.Second,
		},
	)
	if err != nil {
		log.Fatal(err)
	}
	defer server.Close()

	for {
		conn, err := server.Accept()
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("new connection: %d, %d", conn.ConnID(), conn.RemoteID())

		go func() {
			defer conn.Close()

			for {
				msg, err := conn.Receive()
				if err != nil {
					log.Printf("receive failed: %v", err)
					return
				}

				err = conn.Send(msg)
				if err != nil {
					log.Printf("receive failed: %v", err)
					return
				}
			}
		}()
	}
}
