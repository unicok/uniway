package uniway

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/unicok/unet"
)

var _ = (unet.Codec)((*codec)(nil))
var _ = (unet.Codec)((*virtualCodec)(nil))
var _ = (unet.ClearSendChan)((*codec)(nil))

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

func (p *protocol) newCodec(id uint32, conn net.Conn, bufferSize int) *codec {
	c := &codec{
		id:       id,
		protocol: p,
		conn:     conn,
		reader:   bufio.NewReaderSize(conn, bufferSize),
	}
	c.headBuf = c.headDat[:]
	return c
}

// Receive implements unet/Codec.Receive() method.
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

// Send implements unet/Codec.Send() method.
func (c *codec) Send(msg interface{}) error {
	buffer := *(msg.(*[]byte))
	_, err := c.conn.Write(buffer)
	c.free(buffer)
	return err
}

// Close implements unet/Codec.Close() method.
func (c *codec) Close() error {
	return c.conn.Close()
}

// ClearSendChan implements unet/ClearSendChan interface.
func (c *codec) ClearSendChan(sendChan <-chan interface{}) {
	for msg := range sendChan {
		c.free(*(msg.(*[]byte)))
	}
}

// ===========================================================================

type MsgFormat interface {
	DecodeMessage([]byte) (interface{}, error)
	EncodeMessage(interface{}) ([]byte, error)
}

type virtualCodec struct {
	*protocol
	physicalConn *unet.Session
	connID       uint32
	recvChan     chan []byte
	closeOnce    sync.Once
	lastActive   *int64
	format       MsgFormat
}

func (p *protocol) newVirtualCodec(physicalConn *unet.Session, connID uint32, recvChanSize int, lastActive *int64, format MsgFormat) *virtualCodec {
	return &virtualCodec{
		protocol:     p,
		connID:       connID,
		physicalConn: physicalConn,
		recvChan:     make(chan []byte, recvChanSize),
		lastActive:   lastActive,
		format:       format,
	}
}

func (c *virtualCodec) Receive() (interface{}, error) {
	buf, ok := <-c.recvChan
	if !ok {
		return nil, io.EOF
	}
	defer c.free(buf)
	if c.format != nil {
		return c.format.DecodeMessage(buf[cmdConnID+cmdIDSize:])
	}

	msg := make([]byte, len(buf[cmdConnID+cmdIDSize:]))
	copy(msg, buf[cmdConnID+cmdIDSize:])
	return &msg, nil
}

func (c *virtualCodec) Send(msg interface{}) error {
	var msg2 []byte
	var err error

	if c.format != nil {
		msg2, err = c.format.EncodeMessage(msg)
		if err != nil {
			return err
		}
	} else {
		msg2 = *(msg.(*[]byte))
	}

	if len(msg2) > c.maxPacketSize {
		return ErrTooLargePacket
	}

	buf := c.alloc(SizeofLen + cmdIDSize + len(msg2))
	copy(buf[cmdConnID+cmdIDSize:], msg2)
	binary.LittleEndian.PutUint32(buf, uint32(cmdIDSize+len(msg2)))
	binary.LittleEndian.PutUint32(buf[cmdConnID:], c.connID)
	err = c.send(c.physicalConn, buf)
	if err != nil {
		atomic.StoreInt64(c.lastActive, time.Now().Unix())
	}
	return err
}

func (c *virtualCodec) Close() error {
	c.closeOnce.Do(func() {
		close(c.recvChan)
		c.send(c.physicalConn, c.encodeCloseCmd(c.connID))
	})
	for buf := range c.recvChan {
		c.free(buf)
	}
	return nil
}
