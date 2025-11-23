package fusefs

import (
	"testing"
)

func TestBufferPool_SizeClasses(t *testing.T) {
	pool := newBufferPool()

	tests := []struct {
		requestSize  int
		expectedCap  int
		description  string
	}{
		{1024, 4 * 1024, "1KB should get 4KB buffer"},
		{4 * 1024, 4 * 1024, "4KB should get 4KB buffer"},
		{32 * 1024, 64 * 1024, "32KB should get 64KB buffer"},
		{64 * 1024, 64 * 1024, "64KB should get 64KB buffer"},
		{100 * 1024, 128 * 1024, "100KB should get 128KB buffer"},
		{128 * 1024, 128 * 1024, "128KB should get 128KB buffer"},
		{512 * 1024, 1024 * 1024, "512KB should get 1MB buffer"},
		{1024 * 1024, 1024 * 1024, "1MB should get 1MB buffer"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			buf := pool.Get(tt.requestSize)

			if len(buf) != tt.requestSize {
				t.Errorf("expected length %d, got %d", tt.requestSize, len(buf))
			}

			if cap(buf) != tt.expectedCap {
				t.Errorf("expected capacity %d, got %d", tt.expectedCap, cap(buf))
			}

			pool.Put(buf)
		})
	}
}

func TestBufferPool_LargeBuffer(t *testing.T) {
	pool := newBufferPool()

	// Request buffer larger than largest pool size
	size := 2 * 1024 * 1024 // 2MB
	buf := pool.Get(size)

	if len(buf) != size {
		t.Errorf("expected length %d, got %d", size, len(buf))
	}

	if cap(buf) != size {
		t.Errorf("expected capacity %d, got %d", size, cap(buf))
	}

	// Putting it back should not crash (it won't be pooled)
	pool.Put(buf)
}

func TestBufferPool_Reuse(t *testing.T) {
	pool := newBufferPool()

	// Get a buffer
	buf1 := pool.Get(4 * 1024)
	addr1 := &buf1[0]

	// Modify it
	buf1[0] = 42

	// Return it
	pool.Put(buf1)

	// Get another buffer of same size
	buf2 := pool.Get(4 * 1024)
	addr2 := &buf2[0]

	// Should be the same underlying array (reused)
	if addr1 != addr2 {
		t.Log("Note: buffers may not be reused immediately due to sync.Pool behavior")
	}

	pool.Put(buf2)
}

func TestBufferPool_GlobalPool(t *testing.T) {
	// Test global pool functions
	buf := GetBuffer(64 * 1024)

	if len(buf) != 64*1024 {
		t.Errorf("expected length %d, got %d", 64*1024, len(buf))
	}

	PutBuffer(buf)
}

func TestBufferPool_ZeroSize(t *testing.T) {
	pool := newBufferPool()

	buf := pool.Get(0)

	if len(buf) != 0 {
		t.Errorf("expected length 0, got %d", len(buf))
	}

	pool.Put(buf)
}

func TestBufferPool_Concurrent(t *testing.T) {
	pool := newBufferPool()

	// Test concurrent access
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				buf := pool.Get(4 * 1024)
				buf[0] = byte(j)
				pool.Put(buf)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}
}

func BenchmarkBufferPool_Get4KB(b *testing.B) {
	pool := newBufferPool()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := pool.Get(4 * 1024)
		pool.Put(buf)
	}
}

func BenchmarkBufferPool_Get64KB(b *testing.B) {
	pool := newBufferPool()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := pool.Get(64 * 1024)
		pool.Put(buf)
	}
}

func BenchmarkBufferPool_Get128KB(b *testing.B) {
	pool := newBufferPool()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := pool.Get(128 * 1024)
		pool.Put(buf)
	}
}

func BenchmarkBufferPool_Get1MB(b *testing.B) {
	pool := newBufferPool()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := pool.Get(1024 * 1024)
		pool.Put(buf)
	}
}

func BenchmarkDirectAlloc_4KB(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := make([]byte, 4*1024)
		_ = buf
	}
}

func BenchmarkDirectAlloc_64KB(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := make([]byte, 64*1024)
		_ = buf
	}
}

func BenchmarkDirectAlloc_128KB(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := make([]byte, 128*1024)
		_ = buf
	}
}

func BenchmarkDirectAlloc_1MB(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := make([]byte, 1024*1024)
		_ = buf
	}
}
