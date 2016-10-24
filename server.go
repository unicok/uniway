package main

import (
	"io"
	"net"
	"strings"
	"time"
)

type server struct {
	manager      *sessionManager
	listener     net.Listener
	protocol     Protocol
	sendChanSize int
}

func newServer(l net.Listener, p Protocol, sendChanSize int) *server {
	return &server{
		manager:      newSessionManager(),
		listener:     l,
		protocol:     p,
		sendChanSize: sendChanSize,
	}
}

func (s *server) serve(handler func(*session)) error {
	for {
		conn, err := s.accept()
		if err != nil {
			return err
		}
		go func() {
			codec, err := s.protocol.NewCodec(conn)
			if err != nil {
				conn.Close()
				return
			}
			sess := s.manager.newSession(codec, s.sendChanSize)
			handler(sess)
		}()
	}
}

func (s *server) getSession(sessID uint64) *session {
	return s.manager.getSession(sessID)
}

func (s *server) stop() {
	s.listener.Close()
	s.manager.dispose()
}

func (s *server) accept() (net.Conn, error) {
	var tempDelay time.Duration
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}
				if max := 1 * time.Second; tempDelay > max {
					tempDelay = max
				}
				time.Sleep(tempDelay)
				continue
			}
			if strings.Contains(err.Error(), "use of closed network connection") {
				return nil, io.EOF
			}
			return nil, err
		}
		return conn, nil
	}
}
