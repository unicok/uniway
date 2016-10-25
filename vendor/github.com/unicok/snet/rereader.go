package snet

import "io"

type rereader struct {
	head *rereadData
	tail *rereadData
}

type rereadData struct {
	data []byte
	next *rereadData
}

func (r *rereader) pull(b []byte) (n int) {
	if r.head != nil {
		copy(b, r.head.data)
		if len(r.head.data) > len(b) {
			r.head.data = r.head.data[len(b):]
			n = len(b)
			return
		}
		n = len(r.head.data)
		r.head = r.head.next
		if r.head == nil {
			r.tail = nil
		}
	}
	return
}

func (r *rereader) reread(rd io.Reader, n int) bool {
	b := make([]byte, n)
	if _, err := io.ReadFull(rd, b); err != nil {
		return false
	}
	data := &rereadData{b, nil}
	if r.head == nil {
		r.head = data
	} else {
		r.tail.next = data
	}
	r.tail = data
	return true
}
