package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const connBuckets = 32

// Config is gateway config
type Config struct {
	MaxConn      int
	BufferSize   int
	SendChanSize int
	IdleTimeout  time.Duration
	AuthKey      string
}

// Gateway implements gateway protocol.
type Gateway struct {
	protocol
	timer   *timingWheel
	servers [2]*server

	physicalConnID uint32
	physicalConns  [connBuckets][2]*channel

	virtualConnID      uint32
	virtualConns       [connBuckets]map[uint32][2]*session
	virtualConnMutexes [connBuckets]sync.RWMutex
}

// NewGateway create a gateway
func NewGateway(pool Pool, maxPacketSize int) *Gateway {
	var gw = &Gateway{}

	gw.pool = pool
	gw.maxPacketSize = maxPacketSize
	gw.timer = newTimingWheel(100*time.Millisecond, 18000)

	for i := 0; i < connBuckets; i++ {
		gw.virtualConns[i] = make(map[uint32][2]*session)
	}

	for i := 0; i < connBuckets; i++ {
		gw.physicalConns[i][0] = newChannel()
		gw.physicalConns[i][1] = newChannel()
	}
	return gw
}

func (g *Gateway) addVirtualConn(connID uint32, pair [2]*session) {
	bucket := connID % connBuckets
	g.virtualConnMutexes[bucket].Lock()
	defer g.virtualConnMutexes[bucket].Unlock()
	if _, exists := g.virtualConns[bucket][connID]; exists {
		panic("virtual connection already exists")
	}
	g.virtualConns[bucket][connID] = pair
}

func (g *Gateway) getVirtualConn(connID uint32) [2]*session {
	bucket := connID % connBuckets
	g.virtualConnMutexes[bucket].RLock()
	defer g.virtualConnMutexes[bucket].RUnlock()
	return g.virtualConns[bucket][connID]
}

func (g *Gateway) delVirtualConn(connID uint32) ([2]*session, bool) {
	bucket := connID % connBuckets
	g.virtualConnMutexes[bucket].Lock()
	pair, exists := g.virtualConns[bucket][connID]
	if exists {
		delete(g.virtualConns[bucket], connID)
	}
	g.virtualConnMutexes[bucket].Unlock()
	return pair, exists
}

func (g *Gateway) addPhysicalConn(connID uint32, side int, session *session) {
	g.physicalConns[connID%connBuckets][side].put(connID, session)
}

func (g *Gateway) getPhysicalConn(connID uint32, side int) *session {
	return g.physicalConns[connID%connBuckets][side].get(connID)
}

func (g *Gateway) closeVirtualConn(connID uint32) {
	pair, ok := g.delVirtualConn(connID)
	if !ok {
		return
	}

	for _, p := range pair {
		state := p.State.(*state)
		state.Lock()
		defer state.Unlock()
		if state.disposed {
			continue
		}
		delete(state.virtualConns, connID)
		g.send(p, g.encodeCloseCmd(connID))
	}
}

func (g *Gateway) acceptVirtualConn(pair [2]*session, session *session, maxConn int) bool {
	var connID uint32
	if connID == 0 {
		connID = atomic.AddUint32(&g.virtualConnID, 1)
	}

	for _, p := range pair {
		state := p.State.(*state)
		state.Lock()
		defer state.Unlock()
		if state.disposed {
			return false
		}

		if p == session && maxConn != 0 && len(state.virtualConns) >= maxConn {
			return false
		}

		if _, exists := state.virtualConns[connID]; exists {
			panic("virtual connection already exists")
		}

		state.virtualConns[connID] = struct{}{}
	}

	g.addVirtualConn(connID, pair)

	for i, p := range pair {
		remoteID := pair[(i+1)%2].State.(*state).id
		if p == session {
			g.send(p, g.encodeAcceptCmd(connID, remoteID))
		} else {
			g.send(p, g.encodeConnectCmd(connID, remoteID))
		}
	}
	return true
}

// ServeClients serve client connections.
func (g *Gateway) ServeClients(listener net.Listener, config Config) {
	g.servers[0] = newServer(listener, ProtocolFunc(func(rw io.ReadWriter) (Codec, error) {
		return newCodec(&g.protocol, atomic.AddUint32(&g.physicalConnID, 1), rw.(net.Conn), config.BufferSize), nil
	}), config.SendChanSize)

	g.servers[0].serve(func(session *session) {
		g.handleSession(session, 0, config.MaxConn, config.IdleTimeout)
	})
}

// ServeServers serve server connections
func (g *Gateway) ServeServers(listener net.Listener, config Config) {
	g.servers[1] = newServer(listener, ProtocolFunc(func(rw io.ReadWriter) (Codec, error) {
		serverID, err := g.serverAuth(rw.(net.Conn), []byte(config.AuthKey))
		if err != nil {
			log.Printf("error happends when accept server from %s: %s", rw.(net.Conn).RemoteAddr(), err)
			return nil, err
		}
		log.Printf("accept server %d from %s", serverID, rw.(net.Conn).RemoteAddr())
		return newCodec(&g.protocol, serverID, rw.(net.Conn), config.BufferSize), nil
	}), config.SendChanSize)

	g.servers[1].serve(func(session *session) {
		g.handleSession(session, 1, 0, config.IdleTimeout)
	})
}

func (g *Gateway) handleSession(session *session, side, maxConn int, idleTimeout time.Duration) {
	id := session.getCodec().(*codec).id
	state := g.newSessionState(id, session, idleTimeout)
	session.State = state
	g.addPhysicalConn(id, side, session)

	defer func() {
		state.dispose()
		printPanicStack()
	}()

	otherSide := (side + 1) % 2

	for {
		atomic.StoreInt64(&state.lastActive, time.Now().Unix())

		buf, err := session.receive()
		if err != nil {
			return
		}

		msg := *(buf.(*[]byte))
		connID := g.decodePacket(msg)
		if connID == 0 {
			g.processCmd(msg, session, state, side, otherSide, maxConn)
			continue
		}

		pair := g.getVirtualConn(connID)
		if pair[side] == nil || pair[otherSide] == nil {
			g.free(msg)
			g.send(session, g.encodeCloseCmd(connID))
			continue
		}
		if pair[side] != session {
			g.free(msg)
			panic("endpoint not match")
		}
		g.send(pair[otherSide], msg)
	}
}

func (g *Gateway) processCmd(msg []byte, sess *session, state *state, side, otherSide, maxConn int) {
	switch g.decodeCmd(msg) {
	case dialCmd:
		remoteID := g.decodeDialCmd(msg)
		g.free(msg)

		var pair [2]*session
		pair[side] = sess
		pair[otherSide] = g.getPhysicalConn(remoteID, otherSide)
		if pair[otherSide] == nil || !g.acceptVirtualConn(pair, sess, maxConn) {
			g.send(sess, g.encodeRefuseCmd(remoteID))
		}
	case closeCmd:
		connID := g.decodeCloseCmd(msg)
		g.free(msg)
		g.closeVirtualConn(connID)
	case pingCmd:
		state.pingChan <- struct{}{}
		g.free(msg)
		g.send(sess, g.encodePingCmd())
	default:
		g.free(msg)
		panic(fmt.Sprintf("unsupported gateway command: %d", g.decodeCmd(msg)))
	}
}

// Stop gateway
func (g *Gateway) Stop() {
	for _, s := range g.servers {
		s.stop()
	}
	g.timer.stop()
}

type state struct {
	sync.Mutex
	id           uint32
	gateway      *Gateway
	session      *session
	lastActive   int64
	pingChan     chan struct{}
	watchChan    chan struct{}
	disposeChan  chan struct{}
	disposeOnce  sync.Once
	disposed     bool
	virtualConns map[uint32]struct{}
}

func (g *Gateway) newSessionState(id uint32, session *session, idleTimeout time.Duration) *state {
	state := &state{
		id:           id,
		session:      session,
		gateway:      g,
		watchChan:    make(chan struct{}),
		pingChan:     make(chan struct{}),
		disposeChan:  make(chan struct{}),
		virtualConns: make(map[uint32]struct{}),
	}
	go state.watcher(session, idleTimeout)
	return state
}

func (s *state) watcher(session *session, idleTimeout time.Duration) {
L:
	for {
		select {
		case <-s.pingChan:
		case <-s.gateway.timer.after(idleTimeout):
			if time.Since(time.Unix(atomic.LoadInt64(&s.lastActive), 0)) >= idleTimeout {
				break L
			}
		case <-s.disposeChan:
			break L
		}
	}
	s.dispose()
}

func (s *state) dispose() {
	s.disposeOnce.Do(func() {
		close(s.disposeChan)
		s.session.close()

		s.Lock()
		s.disposed = true
		s.Unlock()

		// Close releated virtual connections
		for connID := range s.virtualConns {
			s.gateway.closeVirtualConn(connID)
		}
	})
}
