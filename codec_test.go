package uniway

import (
	"encoding/binary"
	"io"
	"math/rand"
	"net"
	"sync"
	"testing"
)

var TestAddr string
var TestPool = NewPool(64, 64*1024, 2, 1024*1024)
var TestProto = protocol{TestPool, 2048}

func init() {
	lsn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	TestAddr = lsn.Addr().String()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		wg.Done()
		for {
			conn, err := lsn.Accept()
			if err != nil {
				return
			}
			go func() {
				io.Copy(conn, conn)
				conn.Close()
			}()
		}
	}()
	wg.Wait()
	/*
		logFile, err := os.Create("proto.test.log")
		if err != nil {
			panic(err)
		}
		log.SetOutput(logFile)
	*/
}

type NullWriter struct{}

func (w NullWriter) Write(b []byte) (int, error) { return len(b), nil }

func Test_Codec(t *testing.T) {
	conn, err := net.Dial("tcp", TestAddr)
	ok(t, err)
	defer conn.Close()

	codec := newCodec(&TestProto, 0, conn, 1024)

	for i := 0; i < 1000; i++ {
		buffer1 := TestPool.Alloc(SizeofLen + rand.Intn(1024))
		binary.LittleEndian.PutUint32(buffer1, uint32(len(buffer1)-SizeofLen))
		for i := SizeofLen; i < len(buffer1); i++ {
			buffer1[i] = byte(rand.Intn(256))
		}

		buffer2 := make([]byte, len(buffer1))
		copy(buffer2, buffer1)

		codec.Send(&buffer1)

		buffer3, err := codec.Receive()
		ok(t, err)
		equals(t, *(buffer3.(*[]byte)), buffer2)
	}
}

func Test_VirtualCodec(t *testing.T) {
	conn, err := net.Dial("tcp", TestAddr)
	ok(t, err)
	defer conn.Close()

	codec := newCodec(&TestProto, 0, conn, 1024)
	pconn := NewSession(codec, 1000)

	var lastActive int64
	vcodec := newVirtualCodec(&TestProto, pconn, 123, 1024, &lastActive)

	for i := 0; i < 1000; i++ {
		buffer1 := make([]byte, 1024)
		for i := 0; i < len(buffer1); i++ {
			buffer1[i] = byte(rand.Intn(256))
		}
		vcodec.Send(&buffer1)

		msg, err := codec.Receive()
		ok(t, err)

		buffer2 := *(msg.(*[]byte))
		connID := codec.decodePacket(buffer2)
		assert(t, 123 == connID, "")
		vcodec.recvChan <- buffer2

		buffer3, err := vcodec.Receive()
		ok(t, err)
		equals(t, *(buffer3.(*[]byte)), buffer1)
	}
}

func Test_BadCodec(t *testing.T) {
	conn, err := net.Dial("tcp", TestAddr)
	ok(t, err)

	codec := newCodec(&TestProto, 0, conn, 1024)
	_, err = conn.Write([]byte{255, 0, 0, 0})
	ok(t, err)
	codec.reader.ReadByte()
	codec.reader.UnreadByte()
	conn.Close()

	msg, err := codec.Receive()
	assert(t, msg == nil, "msg")
	assert(t, err != nil, err.Error())

	conn, err = net.Dial("tcp", TestAddr)
	ok(t, err)

	codec = newCodec(&TestProto, 0, conn, 1024)
	_, err = conn.Write([]byte{255, 255, 255, 255})
	ok(t, err)

	msg, err = codec.Receive()
	assert(t, msg == nil, "msg")
	assert(t, err != nil, err.Error())
	equals(t, ErrTooLargePacket, err)
}

func Test_BadVirtualCodec(t *testing.T) {
	conn, err := net.Dial("tcp", TestAddr)
	ok(t, err)
	defer conn.Close()

	codec := newCodec(&TestProto, 0, conn, 1024)
	pconn := NewSession(codec, 1000)

	var lastActive int64
	vcodec := newVirtualCodec(&TestProto, pconn, 123, 1024, &lastActive)

	bigMsg := make([]byte, TestProto.maxPacketSize+1)
	err = vcodec.Send(&bigMsg)
	assert(t, err != nil, err.Error())
	equals(t, ErrTooLargePacket, err)

	vcodec.recvChan <- bigMsg
	vcodec.Close()
}
