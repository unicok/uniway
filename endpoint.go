package uniway

import (
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var endpointTimer = newTimingWheel(time.Second, 1800)

// ErrRefused happens when virtual connection couldn't dial to remote EndPoint.
var ErrRefused = errors.New("virtual connection refused")

// EndpointConfig used to config EndPoint.
type EndpointConfig struct {
	MemPool         Pool
	MaxPacket       int
	BufferSize      int
	SendChanSize    int
	RecvChanSize    int
	PingInterval    time.Duration
	PingTimeout     time.Duration
	TimeoutCallback func()
	ServerID        uint32
	AuthKey         string
}

func DefaultEPConfig() EndpointConfig {
	return EndpointConfig{
		MemPool:      NewPool(2, 64, 64*1024, 1024*1024),
		MaxPacket:    512 * 1024,
		BufferSize:   64 * 1024,
		SendChanSize: 102400,
		RecvChanSize: 102400,
		PingInterval: 30 * time.Second,
		PingTimeout:  60 * time.Second,
	}
}

// DialClient dial to gateway and return a client EndPoint.
// addr is the gateway address.
func DialClient(network, addr string, config EndpointConfig) (*Endpoint, error) {
	conn, err := net.Dial(network, addr)
	if err != nil {
		return nil, err
	}
	return NewEPClient(conn, config)
}

// DialServer dial to gateway and return a server EndPoint.
// addr is the gateway address.
func DialServer(network, addr string, config EndpointConfig) (*Endpoint, error) {
	conn, err := net.Dial(network, addr)
	if err != nil {
		return nil, err
	}
	return NewEPServer(conn, config)
}

// NewEPClient dial to gateway and return a client EndPoint.
// conn is the physical connection.
func NewEPClient(conn net.Conn, config EndpointConfig) (*Endpoint, error) {
	ep := newEndpoint(config.MemPool, config.MaxPacket, config.RecvChanSize)
	ep.session = newSession(newCodec(&ep.protocol, 0, conn, config.BufferSize), config.SendChanSize)
	go ep.loop()
	go ep.keepalive(config.PingInterval, config.PingTimeout, config.TimeoutCallback)
	return ep, nil
}

// NewEPServer dial to gateway and return a server EndPoint.
// conn is the physical connection.
func NewEPServer(conn net.Conn, config EndpointConfig) (*Endpoint, error) {
	ep := newEndpoint(config.MemPool, config.MaxPacket, config.RecvChanSize)
	if err := ep.serverInit(conn, config.ServerID, []byte(config.AuthKey)); err != nil {
		return nil, err
	}
	ep.session = newSession(newCodec(&ep.protocol, 0, conn, config.BufferSize), config.SendChanSize)
	go ep.loop()
	go ep.keepalive(config.PingInterval, config.PingTimeout, config.TimeoutCallback)
	return ep, nil
}

type vconn struct {
	session  *Session
	ConnID   uint32
	RemoteID uint32
}

// Endpoint is can be a client or a server.
type Endpoint struct {
	protocol
	manager      *sessionManager
	recvChanSize int
	session      *Session
	lastActive   int64
	newConnMutex sync.Mutex
	newConnChan  chan uint32
	dialMutex    sync.Mutex
	acceptChan   chan vconn
	connectChan  chan vconn
	virtualConns *channel
	closeChan    chan struct{}
	closeFlag    int32
}

func newEndpoint(pool Pool, maxPacketSize, recvChanSize int) *Endpoint {
	return &Endpoint{
		protocol:     protocol{pool, maxPacketSize},
		manager:      newSessionManager(),
		recvChanSize: recvChanSize,
		newConnChan:  make(chan uint32),
		acceptChan:   make(chan vconn, 1),
		connectChan:  make(chan vconn, 1000),
		virtualConns: newChannel(),
		closeChan:    make(chan struct{}),
	}
}

// Accept is accept a virutal connection
func (ep *Endpoint) Accept() (session *Session, connID, remoteID uint32, err error) {
	select {
	case conn := <-ep.connectChan:
		return conn.session, conn.ConnID, conn.RemoteID, nil
	case <-ep.closeChan:
		return nil, 0, 0, io.EOF
	}
}

// Dial create a virtual connection and dial to a remote Endpoint
func (ep *Endpoint) Dial(remoteID uint32) (*Session, uint32, error) {
	ep.dialMutex.Lock()
	defer ep.dialMutex.Unlock()

	if err := ep.send(ep.session, ep.encodeDialCmd(remoteID)); err != nil {
		return nil, 0, err
	}

	select {
	case conn := <-ep.acceptChan:
		if conn.session == nil {
			return nil, 0, ErrRefused
		}
		return conn.session, conn.ConnID, nil
	case <-ep.closeChan:
		return nil, 0, io.EOF
	}
}

// GetSession get a virtual connection session by session ID.
func (ep *Endpoint) GetSession(sessionID uint64) *Session {
	return ep.manager.getSession(sessionID)
}

// Close Endpoint
func (ep *Endpoint) Close() {
	if atomic.CompareAndSwapInt32(&ep.closeFlag, 0, 1) {
		ep.manager.dispose()
		ep.session.Close()
		close(ep.closeChan)
	}
}

func (ep *Endpoint) addVirtualConn(connID, remoteID uint32, c chan vconn) {
	codec := newVirtualCodec(&ep.protocol, ep.session, connID, ep.recvChanSize, &ep.lastActive)
	session := ep.manager.newSession(codec, 0)
	ep.virtualConns.put(connID, session)
	select {
	case c <- vconn{session, connID, remoteID}:
	case <-ep.closeChan:
	default:
		ep.send(ep.session, ep.encodeCloseCmd(connID))
	}
}

func (ep *Endpoint) keepalive(pingInterval, pingTimeout time.Duration, timeoutCallback func()) {
	for {
		select {
		case <-endpointTimer.after(pingInterval):
			if time.Duration(atomic.LoadInt64(&ep.lastActive)) >= pingInterval {
				if ep.send(ep.session, ep.encodePingCmd()) != nil {
					return
				}
			}
			if timeoutCallback != nil {
				select {
				case <-endpointTimer.after(pingTimeout):
					if time.Duration(atomic.LoadInt64(&ep.lastActive)) >= pingTimeout {
						timeoutCallback()
					}
				case <-ep.closeChan:
					return
				}
			}
		case <-ep.closeChan:
			return
		}
	}
}

func (ep *Endpoint) loop() {
	defer PrintPanicStack()

	for {
		atomic.StoreInt64(&ep.lastActive, time.Now().UnixNano())

		msg, err := ep.session.Receive()
		if err != nil {
			return
		}

		buf := *(msg.(*[]byte))
		connID := ep.decodePacket(buf)

		if connID == 0 {
			ep.processCmd(buf)
			continue
		}

		vconn := ep.virtualConns.get(connID)
		if vconn != nil {
			select {
			case vconn.getCodec().(*virtualCodec).recvChan <- buf:
				continue
			default:
				vconn.Close()
			}
		}
		ep.free(buf)
		ep.send(ep.session, ep.encodeCloseCmd(connID))
	}
}

func (ep *Endpoint) processCmd(buf []byte) {
	cmd := ep.decodeCmd(buf)
	switch cmd {
	case acceptCmd:
		connID, remoteID := ep.decodeAcceptCmd(buf)
		ep.free(buf)
		ep.addVirtualConn(connID, remoteID, ep.acceptChan)

	case refuseCmd:
		remoteID := ep.decodeRefuseCmd(buf)
		ep.free(buf)
		select {
		case ep.acceptChan <- vconn{nil, 0, remoteID}:
		case <-ep.closeChan:
			return
		}

	case connectCmd:
		connID, remoteID := ep.decodeConnectCmd(buf)
		ep.free(buf)
		ep.addVirtualConn(connID, remoteID, ep.connectChan)

	case closeCmd:
		connID := ep.decodeCloseCmd(buf)
		ep.free(buf)
		vconn := ep.virtualConns.get(connID)
		if vconn != nil {
			vconn.Close()
		}

	case pingCmd:
		ep.free(buf)

	default:
		ep.free(buf)
		panic("unsupported command")
	}
}
