package uniway

import (
	"errors"
	"io"
	"log"
	"net"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/unicok/slab"
	"github.com/unicok/unet"
)

var EndPointTimer = newTimingWheel(time.Second, 1800)

// ErrRefused happens when virtual connection couldn't dial to remote EndPoint.
var ErrRefused = errors.New("virtual connection refused")

// EndPointCfg used to config EndPoint.
type EndPointCfg struct {
	MemPool         slab.Pool
	MaxPacket       int
	BufferSize      int
	SendChanSize    int
	RecvChanSize    int
	PingInterval    time.Duration
	PingTimeout     time.Duration
	TimeoutCallback func()
	ServerID        uint32
	AuthKey         string
	MsgFormat       MsgFormat
}

// DialClient dial to gateway and return a client EndPoint.
// addr is the gateway address.
func DialClient(network, addr string, cfg EndPointCfg) (*EndPoint, error) {
	conn, err := net.Dial(network, addr)
	if err != nil {
		return nil, err
	}
	return NewClient(conn, cfg)
}

// DialServer dial to gateway and return a server EndPoint.
// addr is the gateway address.
func DialServer(network, addr string, cfg EndPointCfg) (*EndPoint, error) {
	conn, err := net.Dial(network, addr)
	if err != nil {
		return nil, err
	}
	return NewServer(conn, cfg)
}

// NewClient dial to gateway and return a client EndPoint.
// conn is the physical connection.
func NewClient(conn net.Conn, cfg EndPointCfg) (*EndPoint, error) {
	ep := newEndPoint(cfg.MemPool, cfg.MaxPacket, cfg.RecvChanSize, cfg.MsgFormat)
	ep.session = unet.NewSession(ep.newCodec(0, conn, cfg.BufferSize), cfg.SendChanSize)
	go ep.loop()
	go ep.keepalive(cfg.PingInterval, cfg.PingTimeout, cfg.TimeoutCallback)
	return ep, nil
}

// NewServer dial to gateway and return a server EndPoint.
// conn is the physical connection.
func NewServer(conn net.Conn, cfg EndPointCfg) (*EndPoint, error) {
	ep := newEndPoint(cfg.MemPool, cfg.MaxPacket, cfg.RecvChanSize, cfg.MsgFormat)
	if err := ep.serverInit(conn, cfg.ServerID, []byte(cfg.AuthKey)); err != nil {
		return nil, err
	}
	ep.session = unet.NewSession(ep.newCodec(0, conn, cfg.BufferSize), cfg.SendChanSize)
	go ep.loop()
	go ep.keepalive(cfg.PingInterval, cfg.PingTimeout, cfg.TimeoutCallback)
	return ep, nil
}

type Conn struct {
	*unet.Session
	connID   uint32
	remoteID uint32
}

func (c *Conn) ConnID() uint32 {
	return c.connID
}

func (c *Conn) RemoteID() uint32 {
	return c.remoteID
}

// EndPoint is can be a client or a server.
type EndPoint struct {
	protocol
	format       MsgFormat
	manager      *unet.Manager
	recvChanSize int
	session      *unet.Session
	lastActive   int64
	newConnMutex sync.Mutex
	newConnChan  chan uint32
	dialMutex    sync.Mutex
	acceptChan   chan *Conn
	connectChan  chan *Conn
	virtualConns *unet.Channel
	closeChan    chan struct{}
	closeFlag    int32
}

func newEndPoint(pool slab.Pool, maxPacketSize, recvChanSize int, format MsgFormat) *EndPoint {
	return &EndPoint{
		protocol: protocol{
			pool:          pool,
			maxPacketSize: maxPacketSize,
		},
		format:       format,
		manager:      unet.NewManager(),
		recvChanSize: recvChanSize,
		newConnChan:  make(chan uint32),
		acceptChan:   make(chan *Conn, 1),
		connectChan:  make(chan *Conn, 1000),
		virtualConns: unet.NewChannel(),
		closeChan:    make(chan struct{}),
	}
}

// Accept accept a virtual connection.
func (p *EndPoint) Accept() (*Conn, error) {
	select {
	case conn := <-p.connectChan:
		return conn, nil
	case <-p.closeChan:
		return nil, io.EOF
	}
}

// Dial create a virtual connection and dial to a remote EndPoint.
func (p *EndPoint) Dial(remoteID uint32) (*Conn, error) {
	p.dialMutex.Lock()
	defer p.dialMutex.Unlock()

	if err := p.send(p.session, p.encodeDialCmd(remoteID)); err != nil {
		return nil, err
	}

	select {
	case conn := <-p.acceptChan:
		if conn == nil {
			return nil, ErrRefused
		}
		return conn, nil
	case <-p.closeChan:
		return nil, io.EOF
	}
}

// GetSession get a virtual connection session by session ID.
func (p *EndPoint) GetSession(sessionID uint64) *unet.Session {
	return p.manager.GetSession(sessionID)
}

// Close EndPoint.
func (p *EndPoint) Close() {
	if atomic.CompareAndSwapInt32(&p.closeFlag, 0, 1) {
		p.manager.Dispose()
		p.session.Close()
		close(p.closeChan)
		log.Printf("endpoint closed %d", p.session.ID())
	}
}

func (p *EndPoint) keepalive(pingInterval, pingTimeout time.Duration, timeoutCallback func()) {
	for {
		select {
		case <-EndPointTimer.After(pingInterval):
			if time.Duration(atomic.LoadInt64(&p.lastActive)) >= pingInterval {
				if p.send(p.session, p.encodePingCmd()) != nil {
					return
				}
			}
			if timeoutCallback != nil {
				select {
				case <-EndPointTimer.After(pingTimeout):
					if time.Duration(atomic.LoadInt64(&p.lastActive)) >= pingTimeout {
						timeoutCallback()
					}
				case <-p.closeChan:
					return
				}
			}
		case <-p.closeChan:
			return
		}
	}
}

func (p *EndPoint) addVirtualConn(connID, remoteID uint32, c chan *Conn) {
	codec := p.newVirtualCodec(p.session, connID, p.recvChanSize, &p.lastActive, p.format)
	session := p.manager.NewSession(codec, 0)
	p.virtualConns.Put(connID, session)
	select {
	case c <- &Conn{Session: session, connID: connID, remoteID: remoteID}:
	case <-p.closeChan:
	default:
		p.send(p.session, p.encodeCloseCmd(connID))
	}
}

func (p *EndPoint) loop() {
	defer func() {
		p.Close()
		if err := recover(); err != nil {
			log.Printf("uniway.EndPoint panic: %v\n%s", err, debug.Stack())
		}
	}()
	for {
		atomic.StoreInt64(&p.lastActive, time.Now().UnixNano())

		msg, err := p.session.Receive()
		if err != nil {
			return
		}

		buf := *(msg.(*[]byte))
		connID := p.decodePacket(buf)

		if connID == 0 {
			p.processCmd(buf)
			continue
		}

		vconn := p.virtualConns.Get(connID)
		if vconn != nil {
			select {
			case vconn.Codec().(*virtualCodec).recvChan <- buf:
				continue
			default:
				vconn.Close()
			}
		}
		p.free(buf)
		p.send(p.session, p.encodeCloseCmd(connID))
	}
}

func (p *EndPoint) processCmd(buf []byte) {
	cmd := p.decodeCmd(buf)
	switch cmd {
	case acceptCmd:
		connID, remoteID := p.decodeAcceptCmd(buf)
		p.free(buf)
		p.addVirtualConn(connID, remoteID, p.acceptChan)

	case refuseCmd:
		p.free(buf)
		select {
		case p.acceptChan <- nil:
		case <-p.closeChan:
			return
		}

	case connectCmd:
		connID, remoteID := p.decodeConnectCmd(buf)
		p.free(buf)
		p.addVirtualConn(connID, remoteID, p.connectChan)

	case closeCmd:
		connID := p.decodeCloseCmd(buf)
		p.free(buf)
		vconn := p.virtualConns.Get(connID)
		if vconn != nil {
			vconn.Close()
		}

	case pingCmd:
		p.free(buf)

	default:
		p.free(buf)
		panic("unsupported command")
	}
}
