package fusefs

import (
	"testing"
)

func TestStatsCollector_RecordOperation(t *testing.T) {
	sc := newStatsCollector()

	stats := sc.snapshot()
	if stats.Operations != 0 {
		t.Errorf("Initial operations = %d, want 0", stats.Operations)
	}

	sc.recordOperation()
	stats = sc.snapshot()
	if stats.Operations != 1 {
		t.Errorf("Operations = %d, want 1", stats.Operations)
	}

	sc.recordOperation()
	sc.recordOperation()
	stats = sc.snapshot()
	if stats.Operations != 3 {
		t.Errorf("Operations = %d, want 3", stats.Operations)
	}
}

func TestStatsCollector_RecordRead(t *testing.T) {
	sc := newStatsCollector()

	stats := sc.snapshot()
	if stats.BytesRead != 0 {
		t.Errorf("Initial BytesRead = %d, want 0", stats.BytesRead)
	}

	sc.recordRead(100)
	stats = sc.snapshot()
	if stats.BytesRead != 100 {
		t.Errorf("BytesRead = %d, want 100", stats.BytesRead)
	}

	sc.recordRead(50)
	stats = sc.snapshot()
	if stats.BytesRead != 150 {
		t.Errorf("BytesRead = %d, want 150", stats.BytesRead)
	}
}

func TestStatsCollector_RecordWrite(t *testing.T) {
	sc := newStatsCollector()

	stats := sc.snapshot()
	if stats.BytesWritten != 0 {
		t.Errorf("Initial BytesWritten = %d, want 0", stats.BytesWritten)
	}

	sc.recordWrite(200)
	stats = sc.snapshot()
	if stats.BytesWritten != 200 {
		t.Errorf("BytesWritten = %d, want 200", stats.BytesWritten)
	}

	sc.recordWrite(300)
	stats = sc.snapshot()
	if stats.BytesWritten != 500 {
		t.Errorf("BytesWritten = %d, want 500", stats.BytesWritten)
	}
}

func TestStatsCollector_RecordError(t *testing.T) {
	sc := newStatsCollector()

	stats := sc.snapshot()
	if stats.Errors != 0 {
		t.Errorf("Initial Errors = %d, want 0", stats.Errors)
	}

	sc.recordError()
	stats = sc.snapshot()
	if stats.Errors != 1 {
		t.Errorf("Errors = %d, want 1", stats.Errors)
	}

	sc.recordError()
	sc.recordError()
	stats = sc.snapshot()
	if stats.Errors != 3 {
		t.Errorf("Errors = %d, want 3", stats.Errors)
	}
}

func TestStatsCollector_Concurrent(t *testing.T) {
	sc := newStatsCollector()

	// Run multiple goroutines concurrently
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				sc.recordOperation()
				sc.recordRead(10)
				sc.recordWrite(20)
				sc.recordError()
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	stats := sc.snapshot()
	if stats.Operations != 1000 {
		t.Errorf("Operations = %d, want 1000", stats.Operations)
	}
	if stats.BytesRead != 10000 {
		t.Errorf("BytesRead = %d, want 10000", stats.BytesRead)
	}
	if stats.BytesWritten != 20000 {
		t.Errorf("BytesWritten = %d, want 20000", stats.BytesWritten)
	}
	if stats.Errors != 1000 {
		t.Errorf("Errors = %d, want 1000", stats.Errors)
	}
}
