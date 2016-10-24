package api

import (
	"net"

	uw "github.com/unicok/uniway"
)

// DialClient dial to gateway and return a client EndPoint.
// addr is the gateway address.
func DialClient(network, addr string, config Config) (*uw.Endpoint, error) {
	conn, err := net.Dial(network, addr)
	if err != nil {
		return nil, err
	}
	return uw.NewClient(conn, config)
}

// DialServer dial to gateway and return a server EndPoint.
// addr is the gateway address.
func DialServer(network, addr string, config Config) (*uw.Endpoint, error) {
	conn, err := net.Dial(network, addr)
	if err != nil {
		return nil, err
	}
	return uw.NewServer(conn, config)
}
