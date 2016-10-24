package main

import (
	"flag"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	. "github.com/unicok/uniway"
)

var (
	maxPacket = flag.Int("MaxPacket", 512*1024, "Limit max packet size.")
	// memPoolType     = flag.String("MemPoolType", "atom", "Type of memory pool ('sync', 'atom' or 'chan').")
	memPoolFactor   = flag.Int("MemPoolFactor", 2, "Growth in chunk size in memory pool.")
	memPoolMinChunk = flag.Int("MemPoolMinChunk", 64, "Smallest chunk size in memory pool.")
	memPoolMaxChunk = flag.Int("MemPoolMaxChunk", 64*1024, "Largest chunk size in memory pool.")
	memPoolPageSize = flag.Int("MemPoolPageSize", 1024*1024, "Size of each slab in memory pool.")

	clientAddr            = flag.String("ClientAddr", ":0", "The gateway address where clients connect to.")
	clientMaxConn         = flag.Int("ClientMaxConn", 16, "Limit max virtual connections for each client.")
	clientBufferSize      = flag.Int("ClientBufferSize", 2*1024, "Setting bufio.Reader's buffer size.")
	clientSendChanSize    = flag.Int("ClientSendChanSize", 1024, "Tunning client session's async behavior.")
	clientIdleTimeout     = flag.Duration("ClientIdleTimeout", 30*time.Second, "Idle timeout of client connections.")
	clientSnetEnable      = flag.Bool("ClientSnetEnable", false, "Enable/Disable snet protocol for clients.")
	clientSnetEncrypt     = flag.Bool("ClientSnetEncrypt", false, "Enable/Disable client snet protocol encrypt feature.")
	clientSnetBuffer      = flag.Int("ClientSnetBuffer", 64*1024, "Client snet protocol rewriter buffer size.")
	clientSnetInitTimeout = flag.Duration("ClientSnetInitTimeout", 10*time.Second, "Client snet protocol handshake timeout.")
	clientSnetWaitTimeout = flag.Duration("ClientSnetWaitTimeout", 60*time.Second, "Client snet protocol waitting reconnection timeout.")

	serverAddr            = flag.String("ServerAddr", ":0", "The gateway address where servers connect to.")
	serverAuthKey         = flag.String("ServerAuthKey", "", "The private key used to auth server connection.")
	serverBufferSize      = flag.Int("ServerBufferSize", 64*1024, "Buffer size of bufio.Reader for server connections.")
	serverSendChanSize    = flag.Int("ServerSendChanSize", 102400, "Tunning server session's async behavior, this value must be greater than zero.")
	serverIdleTimeout     = flag.Duration("ServerIdleTimeout", 30*time.Second, "Idle timeout of server connections.")
	serverSnetEnable      = flag.Bool("ServerSnetEnable", false, "Enable/Disable snet protocol for server.")
	serverSnetEncrypt     = flag.Bool("ServerSnetEncrypt", false, "Enable/Disable server snet protocol encrypt feature.")
	serverSnetBuffer      = flag.Int("ServerSnetBuffer", 64*1024, "Server snet protocol rewriter buffer size.")
	serverSnetInitTimeout = flag.Duration("ServerSnetInitTimeout", 10*time.Second, "Server snet protocol handshake timeout.")
	serverSnetWaitTimeout = flag.Duration("ServerSnetWaitTimeout", 60*time.Second, "Server snet protocol waitting reconnection timeout.")
)

func main() {
	flag.Parse()

	if *serverSendChanSize <= 0 {
		println("server send chan size must greater than zero.")
	}

	pool := NewPool(
		*memPoolMinChunk,
		*memPoolMaxChunk,
		*memPoolFactor,
		*memPoolPageSize)

	gw := NewGateway(pool, *maxPacket)

	// client side
	clientConfig := Config{
		MaxConn:      *clientMaxConn,
		BufferSize:   *clientBufferSize,
		SendChanSize: *clientSendChanSize,
		IdleTimeout:  *clientIdleTimeout,
	}
	go gw.ServeClients(listen("client", *clientAddr), clientConfig)

	// server side
	serverConfig := Config{
		AuthKey:      *serverAuthKey,
		BufferSize:   *serverBufferSize,
		SendChanSize: *serverSendChanSize,
		IdleTimeout:  *serverIdleTimeout,
	}
	go gw.ServeServers(listen("server", *serverAddr), serverConfig)

	// cmd.Shell("uniway")
	signalWait("uniway")

	gw.Stop()
}

func listen(who, addr string) net.Listener {
	var lsn net.Listener
	var err error

	lsn, err = net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("setup %s listener at %s failed - %s", who, addr, err)
	}

	log.Printf("setup %s listener at - %s", who, lsn.Addr())
	return lsn
}

func signalWait(name string) {
	defer PrintPanicStack()

	if pid := syscall.Getpid(); pid != 1 {
		ioutil.WriteFile(name+".pid", []byte(strconv.Itoa(pid)), 0777)
		defer os.Remove(name + ".pid")
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	for sig := range sigChan {
		switch sig {
		case syscall.SIGTERM, syscall.SIGINT:
			log.Println(name, "killed")
			return
		}
	}
}
