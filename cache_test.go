package fusefs

import (
	"testing"
	"time"
)

func TestLRUCache_Basic(t *testing.T) {
	cache := newLRUCache(3, 0) // size 3, no TTL

	// Test Put and Get
	cache.Put("key1", "value1")
	cache.Put("key2", "value2")
	cache.Put("key3", "value3")

	val, ok := cache.Get("key1")
	if !ok || val.(string) != "value1" {
		t.Errorf("expected key1=value1, got ok=%v, val=%v", ok, val)
	}

	val, ok = cache.Get("key2")
	if !ok || val.(string) != "value2" {
		t.Errorf("expected key2=value2, got ok=%v, val=%v", ok, val)
	}

	val, ok = cache.Get("key3")
	if !ok || val.(string) != "value3" {
		t.Errorf("expected key3=value3, got ok=%v, val=%v", ok, val)
	}
}

func TestLRUCache_Eviction(t *testing.T) {
	cache := newLRUCache(3, 0)

	// Fill cache
	cache.Put("key1", "value1")
	cache.Put("key2", "value2")
	cache.Put("key3", "value3")

	// Add one more, should evict key1 (least recently used)
	cache.Put("key4", "value4")

	// key1 should be evicted
	_, ok := cache.Get("key1")
	if ok {
		t.Error("expected key1 to be evicted")
	}

	// Others should still be present
	if _, ok := cache.Get("key2"); !ok {
		t.Error("expected key2 to be present")
	}
	if _, ok := cache.Get("key3"); !ok {
		t.Error("expected key3 to be present")
	}
	if _, ok := cache.Get("key4"); !ok {
		t.Error("expected key4 to be present")
	}
}

func TestLRUCache_LRUOrder(t *testing.T) {
	cache := newLRUCache(3, 0)

	cache.Put("key1", "value1")
	cache.Put("key2", "value2")
	cache.Put("key3", "value3")

	// Access key1, making it most recently used
	cache.Get("key1")

	// Add new key, should evict key2 (now least recently used)
	cache.Put("key4", "value4")

	// key2 should be evicted
	if _, ok := cache.Get("key2"); ok {
		t.Error("expected key2 to be evicted")
	}

	// key1 should still be present
	if _, ok := cache.Get("key1"); !ok {
		t.Error("expected key1 to be present")
	}
}

func TestLRUCache_Update(t *testing.T) {
	cache := newLRUCache(3, 0)

	cache.Put("key1", "value1")
	cache.Put("key1", "value2") // Update

	val, ok := cache.Get("key1")
	if !ok || val.(string) != "value2" {
		t.Errorf("expected updated value2, got ok=%v, val=%v", ok, val)
	}

	// Should still have only 1 entry
	if cache.Len() != 1 {
		t.Errorf("expected size 1, got %d", cache.Len())
	}
}

func TestLRUCache_TTL(t *testing.T) {
	cache := newLRUCache(10, 50*time.Millisecond)

	cache.Put("key1", "value1")

	// Should be present immediately
	if _, ok := cache.Get("key1"); !ok {
		t.Error("expected key1 to be present")
	}

	// Wait for expiration
	time.Sleep(60 * time.Millisecond)

	// Should be expired now
	if _, ok := cache.Get("key1"); ok {
		t.Error("expected key1 to be expired")
	}
}

func TestLRUCache_Delete(t *testing.T) {
	cache := newLRUCache(10, 0)

	cache.Put("key1", "value1")
	cache.Put("key2", "value2")

	cache.Delete("key1")

	if _, ok := cache.Get("key1"); ok {
		t.Error("expected key1 to be deleted")
	}
	if _, ok := cache.Get("key2"); !ok {
		t.Error("expected key2 to still be present")
	}

	if cache.Len() != 1 {
		t.Errorf("expected size 1, got %d", cache.Len())
	}
}

func TestLRUCache_Clear(t *testing.T) {
	cache := newLRUCache(10, 0)

	cache.Put("key1", "value1")
	cache.Put("key2", "value2")
	cache.Put("key3", "value3")

	cache.Clear()

	if cache.Len() != 0 {
		t.Errorf("expected size 0, got %d", cache.Len())
	}

	if _, ok := cache.Get("key1"); ok {
		t.Error("expected cache to be empty")
	}
}

func TestLRUCache_Stats(t *testing.T) {
	cache := newLRUCache(10, 0)

	cache.Put("key1", "value1")
	cache.Put("key2", "value2")

	// Generate hits
	cache.Get("key1")
	cache.Get("key1")
	cache.Get("key2")

	// Generate misses
	cache.Get("key3")
	cache.Get("key4")

	stats := cache.Stats()

	if stats.Size != 2 {
		t.Errorf("expected size 2, got %d", stats.Size)
	}

	if stats.Hits != 3 {
		t.Errorf("expected 3 hits, got %d", stats.Hits)
	}

	if stats.Misses != 2 {
		t.Errorf("expected 2 misses, got %d", stats.Misses)
	}

	expectedHitRate := 3.0 / 5.0
	if stats.HitRate != expectedHitRate {
		t.Errorf("expected hit rate %.2f, got %.2f", expectedHitRate, stats.HitRate)
	}
}

func TestLRUCache_UnlimitedSize(t *testing.T) {
	cache := newLRUCache(0, 0) // Unlimited size

	// Add many entries
	for i := 0; i < 1000; i++ {
		cache.Put(string(rune(i)), i)
	}

	// All should be present
	if cache.Len() != 1000 {
		t.Errorf("expected size 1000, got %d", cache.Len())
	}
}

func TestLRUCache_Concurrent(t *testing.T) {
	cache := newLRUCache(100, 0)

	// Test concurrent access
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				key := string(rune(id*100 + j))
				cache.Put(key, j)
				cache.Get(key)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should not crash and should have some entries
	if cache.Len() == 0 {
		t.Error("expected cache to have entries")
	}
}

func BenchmarkLRUCache_Put(b *testing.B) {
	cache := newLRUCache(1000, 0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Put(string(rune(i%1000)), i)
	}
}

func BenchmarkLRUCache_Get(b *testing.B) {
	cache := newLRUCache(1000, 0)

	// Populate cache
	for i := 0; i < 1000; i++ {
		cache.Put(string(rune(i)), i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Get(string(rune(i % 1000)))
	}
}

func BenchmarkLRUCache_Mixed(b *testing.B) {
	cache := newLRUCache(1000, 0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			cache.Put(string(rune(i%1000)), i)
		} else {
			cache.Get(string(rune(i % 1000)))
		}
	}
}

// Additional Cache Tests for Phase 3

func TestLRUCache_DeleteNonExistent(t *testing.T) {
	cache := newLRUCache(10, 0)

	// Delete non-existent key should not panic
	cache.Delete("nonexistent")

	// Cache should still work
	cache.Put("key1", "value1")
	if _, ok := cache.Get("key1"); !ok {
		t.Error("Expected cache to work after deleting non-existent key")
	}
}

func TestLRUCache_EvictionStats(t *testing.T) {
	cache := newLRUCache(3, 0)

	// Fill cache
	cache.Put("key1", "value1")
	cache.Put("key2", "value2")
	cache.Put("key3", "value3")

	// Add more to cause evictions
	cache.Put("key4", "value4")
	cache.Put("key5", "value5")

	stats := cache.Stats()
	if stats.Evictions != 2 {
		t.Errorf("Expected 2 evictions, got %d", stats.Evictions)
	}
}

func TestLRUCache_MaxSizeStats(t *testing.T) {
	cache := newLRUCache(5, 0)

	stats := cache.Stats()
	if stats.MaxSize != 5 {
		t.Errorf("Expected MaxSize 5, got %d", stats.MaxSize)
	}
}

func TestLRUCache_ZeroHitRate(t *testing.T) {
	cache := newLRUCache(10, 0)

	// Stats with no operations
	stats := cache.Stats()
	if stats.HitRate != 0 {
		t.Errorf("Expected HitRate 0 with no operations, got %f", stats.HitRate)
	}
}

func TestLRUCache_TTLNotExpired(t *testing.T) {
	cache := newLRUCache(10, 1*time.Second)

	cache.Put("key1", "value1")

	// Access immediately - should not be expired
	val, ok := cache.Get("key1")
	if !ok || val.(string) != "value1" {
		t.Error("Expected value to be present before TTL")
	}
}

func TestLRUCache_UpdateMovesFront(t *testing.T) {
	cache := newLRUCache(3, 0)

	cache.Put("key1", "value1")
	cache.Put("key2", "value2")
	cache.Put("key3", "value3")

	// Update key1 - should move it to front
	cache.Put("key1", "updated")

	// Add new key - should evict key2 (oldest non-updated)
	cache.Put("key4", "value4")

	// key1 should still be present
	if _, ok := cache.Get("key1"); !ok {
		t.Error("Expected key1 to be present after update")
	}

	// key2 should be evicted
	if _, ok := cache.Get("key2"); ok {
		t.Error("Expected key2 to be evicted")
	}
}

func TestLRUCache_EmptyEvict(t *testing.T) {
	cache := newLRUCache(1, 0)

	// This should trigger evictOldest on empty list which should not panic
	cache.Put("key1", "value1")
	// Add second should trigger eviction
	cache.Put("key2", "value2")

	if cache.Len() != 1 {
		t.Errorf("Expected size 1, got %d", cache.Len())
	}
}
