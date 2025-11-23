package fusefs

import (
	"sync"
)

// bufferPool manages reusable byte slices to reduce GC pressure.
//
// The pool uses sync.Pool internally and provides buffers in multiple
// size classes to accommodate different I/O patterns efficiently.
//
// Buffers are automatically selected based on the requested size,
// choosing the smallest size class that can fit the request.
type bufferPool struct {
	// Size classes for buffer pools
	// Using power-of-2 sizes for efficient allocation
	pools []*sync.Pool
	sizes []int
}

// newBufferPool creates a new buffer pool with predefined size classes.
//
// Size classes:
//   - 4KB: Small reads (metadata, directory entries)
//   - 64KB: Medium reads (typical file operations)
//   - 128KB: Default read/write size (matches MaxReadahead/MaxWrite)
//   - 1MB: Large sequential reads
func newBufferPool() *bufferPool {
	sizes := []int{
		4 * 1024,    // 4KB
		64 * 1024,   // 64KB
		128 * 1024,  // 128KB
		1024 * 1024, // 1MB
	}

	pools := make([]*sync.Pool, len(sizes))
	for i, size := range sizes {
		size := size // capture for closure
		pools[i] = &sync.Pool{
			New: func() interface{} {
				buf := make([]byte, size)
				return &buf
			},
		}
	}

	return &bufferPool{
		pools: pools,
		sizes: sizes,
	}
}

// Get retrieves a buffer of at least the requested size.
//
// The returned buffer may be larger than requested. The caller should
// slice it to the desired size if needed.
//
// The buffer must be returned to the pool using Put() when no longer needed.
func (p *bufferPool) Get(size int) []byte {
	// Find the smallest size class that fits
	for i, poolSize := range p.sizes {
		if size <= poolSize {
			bufPtr := p.pools[i].Get().(*[]byte)
			return (*bufPtr)[:size]
		}
	}

	// If larger than all pools, allocate directly
	// (not pooled to avoid holding large buffers)
	return make([]byte, size)
}

// Put returns a buffer to the pool for reuse.
//
// The buffer will be returned to the appropriate size class pool.
// Buffers larger than the largest pool size are not pooled and will
// be garbage collected normally.
func (p *bufferPool) Put(buf []byte) {
	// Restore to full capacity for proper pooling
	capacity := cap(buf)

	// Find matching size class
	for i, size := range p.sizes {
		if capacity == size {
			fullBuf := buf[:capacity]
			p.pools[i].Put(&fullBuf)
			return
		}
	}

	// If not a pooled size, let it be garbage collected
}

// globalBufferPool is the shared buffer pool for all I/O operations
var globalBufferPool = newBufferPool()

// GetBuffer retrieves a buffer from the global pool
func GetBuffer(size int) []byte {
	return globalBufferPool.Get(size)
}

// PutBuffer returns a buffer to the global pool
func PutBuffer(buf []byte) {
	globalBufferPool.Put(buf)
}
