package uiway

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var _ = (Codec)((*codec)(nil))
var _ = (Codec)((*virtualCodec)(nil))
var _ = (ClearSendChan)((*codec)(nil))

// SizeofLen is the size of `Length` field.
const SizeofLen = 4

// ErrTooLargePacket happens when gateway receive a packet length greater than `MaxPacketSize` setting.
var ErrTooLargePacket = errors.New("too large packet")

type codec struct {
	*protocol
	id      uint32
	conn    net.Conn
	reader  *bufio.Reader
	headBuf []byte
	headDat [SizeofLen]byte
}

func newCodec(p *protocol, id uint32, conn net.Conn, bufferSize int) *codec {
	c := &codec{
		protocol: p,
		id:       id,
		conn:     conn,
		reader:   bufio.NewReaderSize(conn, bufferSize),
	}
	c.headBuf = c.headDat[:]
	return c
}

// Receive implements Codec.Receive() method
func (c *codec) Receive() (interface{}, error) {
	if _, err := io.ReadFull(c.reader, c.headBuf); err != nil {
		return nil, err
	}
	length := int(binary.LittleEndian.Uint32(c.headBuf))
	if length > c.maxPacketSize {
		return nil, ErrTooLargePacket
	}
	buffer := c.alloc(SizeofLen + length)
	copy(buffer, c.headBuf)
	if _, err := io.ReadFull(c.reader, buffer[SizeofLen:]); err != nil {
		c.free(buffer)
		return nil, err
	}
	return &buffer, nil
}

// Send implements Codec.Send() method.
func (c *codec) Send(msg interface{}) error {
	buffer := *(msg.(*[]byte))
	_, err := c.conn.Write(buffer)
	c.free(buffer)
	return err
}

// Close implements Codec.Close() method.
func (c *codec) Close() error {
	return c.conn.Close()
}

// ClearSendChan implements ClearSendChan interface.
func (c *codec) ClearSendChan(sendChan <-chan interface{}) {
	for msg := range sendChan {
		c.free(*(msg.(*[]byte)))
	}
}

// ===========================================================================

type virtualCodec struct {
	*protocol
	phyConn    *Session
	connID     uint32
	recvChan   chan []byte
	closeOnce  sync.Once
	lastActive *int64
}

func newVirtualCodec(p *protocol, phyConn *Session, connID uint32, recvChanSize int, lastActive *int64) *virtualCodec {
	return &virtualCodec{
		protocol:   p,
		connID:     connID,
		phyConn:    phyConn,
		recvChan:   make(chan []byte, recvChanSize),
		lastActive: lastActive,
	}
}

func (c *virtualCodec) Receive() (interface{}, error) {
	buf, ok := <-c.recvChan
	if !ok {
		return nil, io.EOF
	}
	msg := make([]byte, len(buf[cmdConnID+cmdIDSize:]))
	copy(msg, buf[cmdConnID+cmdIDSize:])
	return &msg, nil
}

func (c *virtualCodec) Send(msgraw interface{}) error {
	msg := *(msgraw.(*[]byte))
	if len(msg) > c.maxPacketSize {
		return ErrTooLargePacket
	}
	buf := c.alloc(SizeofLen + cmdIDSize + len(msg))
	copy(buf[cmdConnID+cmdIDSize:], msg)
	binary.LittleEndian.PutUint32(buf, uint32(cmdIDSize+len(msg)))
	binary.LittleEndian.PutUint32(buf[cmdConnID:], c.connID)
	err := c.send(c.phyConn, buf)
	if err != nil {
		atomic.StoreInt64(c.lastActive, time.Now().Unix())
	}
	return err
}

func (c *virtualCodec) Close() error {
	c.closeOnce.Do(func() {
		close(c.recvChan)
		c.send(c.phyConn, c.encodeCloseCmd(c.connID))
	})
	for buf := range c.recvChan {
		c.free(buf)
	}
	return nil
}
