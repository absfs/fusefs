package fusefs

import (
	"sync/atomic"
)

// Stats contains filesystem statistics
type Stats struct {
	Mountpoint   string
	Operations   uint64
	BytesRead    uint64
	BytesWritten uint64
	Errors       uint64
	OpenFiles    int
	CachedInodes int
}

// statsCollector tracks filesystem statistics
type statsCollector struct {
	operations   atomic.Uint64
	bytesRead    atomic.Uint64
	bytesWritten atomic.Uint64
	errors       atomic.Uint64
}

// newStatsCollector creates a new statistics collector
func newStatsCollector() *statsCollector {
	return &statsCollector{}
}

// recordOperation increments the operation counter
func (s *statsCollector) recordOperation() {
	s.operations.Add(1)
}

// recordRead increments bytes read
func (s *statsCollector) recordRead(n int) {
	s.bytesRead.Add(uint64(n))
}

// recordWrite increments bytes written
func (s *statsCollector) recordWrite(n int) {
	s.bytesWritten.Add(uint64(n))
}

// recordError increments error counter
func (s *statsCollector) recordError() {
	s.errors.Add(1)
}

// snapshot returns current statistics
func (s *statsCollector) snapshot() Stats {
	return Stats{
		Operations:   s.operations.Load(),
		BytesRead:    s.bytesRead.Load(),
		BytesWritten: s.bytesWritten.Load(),
		Errors:       s.errors.Load(),
	}
}
