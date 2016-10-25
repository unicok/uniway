package snet

import (
	"io"
)

type rewriter struct {
	data  []byte
	begin int
}

func (r *rewriter) push(b []byte) {
	for c, n := b, 0; len(c) > 0; c = c[n:] {
		n = len(r.data) - r.begin
		if n > len(c) {
			n = len(c)
		}
		copy(r.data[r.begin:], c[:n])
		r.begin = (r.begin + n) % len(r.data)
	}
}

func (r *rewriter) rewrite(w io.Writer, writeCount, readCount uint64) bool {
	n := int(writeCount - readCount)

	switch {
	case n == 0:
		return true
	case n < 0 || n > len(r.data):
		return false
	case int(writeCount) <= len(r.data):
		_, err := w.Write(r.data[readCount : int(readCount)+n])
		return err == nil
	}

	var (
		begin = (r.begin + (len(r.data) - n)) % len(r.data)
		end   = begin + n
	)
	if end > len(r.data) {
		end = len(r.data)
	}
	if _, err := w.Write(r.data[begin:end]); err != nil {
		return false
	}

	n -= end - begin
	if n == 0 {
		return true
	}
	_, err := w.Write(r.data[:n])
	return err == nil
}
