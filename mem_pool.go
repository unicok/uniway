package uniway

import (
	"reflect"
	"runtime"
	"sync/atomic"
	"unsafe"
)

type Pool interface {
	Alloc(int) []byte
	Free([]byte)
}

// AtomPool is a lock-free slab allocation memory pool.
type AtomPool struct {
	classes []class
	minSize int
	maxSize int
}

func NewPool(minSize, maxSize, factor, pageSize int) Pool {
	pool := &AtomPool{
		classes: make([]class, 0, 10),
		minSize: minSize,
		maxSize: maxSize}
	for chunkSize := minSize; chunkSize <= maxSize && chunkSize <= pageSize; chunkSize *= factor {
		c := class{
			size:   chunkSize,
			page:   make([]byte, pageSize),
			chunks: make([]chunk, pageSize/chunkSize),
			head:   (1 << 32),
		}
		for i := 0; i < len(c.chunks); i++ {
			ck := &c.chunks[i]
			// lock down the capacity to protect append operation
			ck.mem = c.page[i*chunkSize : (i+1)*chunkSize : (i+1)*chunkSize]
			if i < len(c.chunks)-1 {
				ck.next = uint64(i+1+1 /* index start from 1 */) << 32
			} else {
				c.pageBegin = uintptr(unsafe.Pointer(&c.page[0]))
				c.pageEnd = uintptr(unsafe.Pointer(&ck.mem[0]))
			}
		}
		pool.classes = append(pool.classes, c)
	}
	return pool
}

// Alloc try alloc a []byte from internal slab class if no free chunk in slab class Alloc will make one.
func (p *AtomPool) Alloc(size int) []byte {
	if size <= p.maxSize {
		for i := 0; i < len(p.classes); i++ {
			if p.classes[i].size >= size {
				mem := p.classes[i].Pop()
				if mem != nil {
					return mem[:size]
				}
				break
			}
		}
	}
	return make([]byte, size)
}

// Free release a []byte that alloc from Pool.Alloc.
func (p *AtomPool) Free(mem []byte) {
	size := cap(mem)
	for i := 0; i < len(p.classes); i++ {
		if p.classes[i].size == size {
			p.classes[i].Push(mem)
			break
		}
	}
}

type class struct {
	size      int
	page      []byte
	pageBegin uintptr
	pageEnd   uintptr
	chunks    []chunk
	head      uint64
}

type chunk struct {
	mem  []byte
	aba  uint32 // reslove ABA problem
	next uint64
}

func (c *class) Push(mem []byte) {
	ptr := (*reflect.SliceHeader)(unsafe.Pointer(&mem)).Data
	if c.pageBegin <= ptr && ptr <= c.pageEnd {
		i := (ptr - c.pageBegin) / uintptr(c.size)
		ck := &c.chunks[i]
		if ck.next != 0 {
			panic("slab.AtomPool: Double Free")
		}
		ck.aba++
		new := uint64(i+1)<<32 + uint64(ck.aba)
		for {
			old := atomic.LoadUint64(&c.head)
			atomic.StoreUint64(&ck.next, old)
			if atomic.CompareAndSwapUint64(&c.head, old, new) {
				break
			}
			runtime.Gosched()
		}
	}
}

func (c *class) Pop() []byte {
	for {
		old := atomic.LoadUint64(&c.head)
		if old == 0 {
			return nil
		}
		ck := &c.chunks[old>>32-1]
		next := atomic.LoadUint64(&ck.next)
		if atomic.CompareAndSwapUint64(&c.head, old, next) {
			atomic.StoreUint64(&ck.next, 0)
			return ck.mem
		}
		runtime.Gosched()
	}
}
