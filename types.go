package uiway

import "io"

type Protocol interface {
	NewCodec(rw io.ReadWriter) (Codec, error)
}

type ProtocolFunc func(rw io.ReadWriter) (Codec, error)

func (pf ProtocolFunc) NewCodec(rw io.ReadWriter) (Codec, error) {
	return pf(rw)
}

type Codec interface {
	Receive() (interface{}, error)
	Send(interface{}) error
	Close() error
}

type ClearSendChan interface {
	ClearSendChan(<-chan interface{})
}
