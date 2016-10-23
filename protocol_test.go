package main

import (
	"math/rand"
	"net"
	"testing"
)

func Test_DialCmd(t *testing.T) {
	conn, err := net.Dial("tcp", TestAddr)
	ok(t, err)
	defer conn.Close()

	codec := newCodec(&TestProto, 0, conn, 1024)

	for i := 0; i < 10000; i++ {
		remoteID1 := rand.Uint32()
		msg1 := TestProto.encodeDialCmd(remoteID1)

		err := codec.Send(&msg1)
		ok(t, err)

		msg2, err := codec.Receive()
		ok(t, err)

		msg3 := *(msg2.(*[]byte))
		cmd := TestProto.decodeCmd(msg3)
		assert(t, cmd == dialCmd, "")

		remoteID2 := TestProto.decodeDialCmd(msg3)
		assert(t, remoteID1 == remoteID2, "")
	}
}

func Test_AcceptCmd(t *testing.T) {
	conn, err := net.Dial("tcp", TestAddr)
	ok(t, err)
	defer conn.Close()

	codec := newCodec(&TestProto, 0, conn, 1024)

	for i := 0; i < 10000; i++ {
		connID1 := rand.Uint32()
		remoteID1 := rand.Uint32()
		msg1 := TestProto.encodeAcceptCmd(connID1, remoteID1)

		err := codec.Send(&msg1)
		ok(t, err)

		msg2, err := codec.Receive()
		ok(t, err)

		msg3 := *(msg2.(*[]byte))
		cmd := TestProto.decodeCmd(msg3)
		assert(t, cmd == acceptCmd, "")

		connID2, remoteID2 := TestProto.decodeAcceptCmd(msg3)
		assert(t, connID1 == connID2, "")
		assert(t, remoteID1 == remoteID2, "")
	}
}

func Test_RefuseCmd(t *testing.T) {
	conn, err := net.Dial("tcp", TestAddr)
	ok(t, err)
	defer conn.Close()

	codec := newCodec(&TestProto, 0, conn, 1024)

	for i := 0; i < 10000; i++ {
		remoteID1 := rand.Uint32()
		msg1 := TestProto.encodeRefuseCmd(remoteID1)

		err := codec.Send(&msg1)
		ok(t, err)

		msg2, err := codec.Receive()
		ok(t, err)

		msg3 := *(msg2.(*[]byte))
		cmd := TestProto.decodeCmd(msg3)
		assert(t, cmd == refuseCmd, "")

		remoteID2 := TestProto.decodeRefuseCmd(msg3)
		assert(t, remoteID1 == remoteID2, "")
	}
}

func Test_ConnectCmd(t *testing.T) {
	conn, err := net.Dial("tcp", TestAddr)
	ok(t, err)
	defer conn.Close()

	codec := newCodec(&TestProto, 0, conn, 1024)

	for i := 0; i < 10000; i++ {
		connID1 := rand.Uint32()
		remoteID1 := rand.Uint32()
		msg1 := TestProto.encodeConnectCmd(connID1, remoteID1)

		err := codec.Send(&msg1)
		ok(t, err)

		msg2, err := codec.Receive()
		ok(t, err)

		msg3 := *(msg2.(*[]byte))
		cmd := TestProto.decodeCmd(msg3)
		assert(t, cmd == connectCmd, "")

		connID2, remoteID2 := TestProto.decodeConnectCmd(msg3)
		assert(t, connID1 == connID2, "")
		assert(t, remoteID1 == remoteID2, "")
	}
}

func Test_CloseCmd(t *testing.T) {
	conn, err := net.Dial("tcp", TestAddr)
	ok(t, err)
	defer conn.Close()

	codec := newCodec(&TestProto, 0, conn, 1024)

	for i := 0; i < 10000; i++ {
		conndID1 := rand.Uint32()
		msg1 := TestProto.encodeCloseCmd(conndID1)

		err := codec.Send(&msg1)
		ok(t, err)

		msg2, err := codec.Receive()
		ok(t, err)

		msg3 := *(msg2.(*[]byte))
		cmd := TestProto.decodeCmd(msg3)
		assert(t, cmd == closeCmd, "")

		conndID2 := TestProto.decodeCloseCmd(msg3)
		assert(t, conndID1 == conndID2, "")
	}
}

func Test_PingCmd(t *testing.T) {
	conn, err := net.Dial("tcp", TestAddr)
	ok(t, err)
	defer conn.Close()

	codec := newCodec(&TestProto, 0, conn, 1024)

	for i := 0; i < 10000; i++ {
		msg1 := TestProto.encodePingCmd()

		err := codec.Send(&msg1)
		ok(t, err)

		msg2, err := codec.Receive()
		ok(t, err)

		msg3 := *(msg2.(*[]byte))
		cmd := TestProto.decodeCmd(msg3)
		assert(t, cmd == pingCmd, "")
	}
}

func Test_ServerHandshake(t *testing.T) {
	lsn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}

	go func() {
		conn, err := net.Dial("tcp", lsn.Addr().String())
		ok(t, err)
		TestProto.serverInit(conn, 123, []byte("test"))
	}()

	conn, err := lsn.Accept()
	ok(t, err)

	serverID, err := TestProto.serverAuth(conn, []byte("test"))
	ok(t, err)
	assert(t, serverID == 123, "")
}

func Test_BadSession(t *testing.T) {
	conn, err := net.Dial("tcp", TestAddr)
	ok(t, err)
	defer conn.Close()

	codec := newCodec(&TestProto, 0, conn, 1024)
	session := NewSession(codec, 10)
	session.Close()

	err = TestProto.send(session, TestProto.encodePingCmd())
	assert(t, err != nil, "")
}

func Test_BadHandshake(t *testing.T) {
	lsn1, err := net.Listen("tcp", "127.0.0.1:0")
	ok(t, err)
	defer lsn1.Close()

	go func() {
		lsn1.Accept()
	}()
	conn1, err := net.Dial("tcp", lsn1.Addr().String())
	ok(t, err)
	defer conn1.Close()
	err = TestProto.serverInit(conn1, 1, []byte("1"))
	assert(t, err != nil, "")

	go func() {
		net.Dial("tcp", lsn1.Addr().String())
	}()
	conn2, err := lsn1.Accept()
	ok(t, err)
	defer conn2.Close()
	_, err = TestProto.serverAuth(conn2, []byte("1"))
	assert(t, err != nil, "")
}

func XXOO() interface{} {
	return make([]byte, 100)
}

func OOXX() interface{} {
	m := make([]byte, 100)
	return &m
}

func Benchmark_Bytes(b *testing.B) {
	var x []byte
	for i := 0; i < b.N; i++ {
		x = XXOO().([]byte)
	}
	x[0] = 1
}

func Benchmark_BytesPtr(b *testing.B) {
	var x []byte
	for i := 0; i < b.N; i++ {
		x = *(OOXX().(*[]byte))
	}
	x[0] = 1
}
