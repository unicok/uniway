package uiway

import (
	"io"
	"net"
	"strings"
	"time"
)

type Server struct {
	manager      *SessionManager
	listener     net.Listener
	protocol     Protocol
	sendChanSize int
}

type Handler interface {
	HandlerSession(*Session)
}

var _ Handler = HandlerFunc(nil)

type HandlerFunc func(*Session)

func (f HandlerFunc) HandlerSession(sess *Session) {
	f(sess)
}

func NewServer(l net.Listener, p Protocol, sendChanSize int) *Server {
	return &Server{
		manager:      NewSessionManager(),
		listener:     l,
		protocol:     p,
		sendChanSize: sendChanSize,
	}
}

func (s *Server) Listener() net.Listener {
	return s.listener
}

func (s *Server) Serve(handler Handler) error {
	for {
		conn, err := s.Accept()
		if err != nil {
			return err
		}
		go func() {
			codec, err := s.protocol.NewCodec(conn)
			if err != nil {
				conn.Close()
				return
			}
			sess := s.manager.NewSession(codec, s.sendChanSize)
			handler.HandlerSession(sess)
		}()
	}
}

func (s *Server) GetSession(sessID uint64) *Session {
	return s.manager.GetSession(sessID)
}

func (s *Server) Stop() {
	s.listener.Close()
	s.manager.Dispose()
}

func (s *Server) Accept() (net.Conn, error) {
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
