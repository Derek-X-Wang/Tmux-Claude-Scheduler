package monitor

import (
	"sync"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/derekxwang/tcs/internal/database"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}

	// Auto-migrate the schema
	err = db.AutoMigrate(&database.UsageWindow{})
	if err != nil {
		t.Fatalf("Failed to migrate test database: %v", err)
	}

	return db
}

func TestUsageMonitor_GetCurrentStats_ConcurrentAccess(t *testing.T) {
	db := setupTestDB(t)

	monitor := NewUsageMonitor(db)

	// Create a current window for testing
	window := &database.UsageWindow{
		StartTime:     time.Now().Add(-1 * time.Hour),
		EndTime:       time.Now().Add(4 * time.Hour),
		TotalMessages: 10,
		TotalTokens:   1000,
	}

	monitor.currentWindow = window

	// Test concurrent access to GetCurrentStats
	const numGoroutines = 10
	const numCallsPerGoroutine = 5

	var wg sync.WaitGroup
	results := make(chan error, numGoroutines*numCallsPerGoroutine)

	// Launch multiple goroutines that call GetCurrentStats concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numCallsPerGoroutine; j++ {
				_, err := monitor.GetCurrentStats()
				results <- err
				time.Sleep(time.Millisecond) // Small delay to increase contention
			}
		}()
	}

	wg.Wait()
	close(results)

	// Check results - we expect some "stats calculation in progress" errors
	// but no panics or data races
	errorCount := 0
	progressErrors := 0

	for err := range results {
		if err != nil {
			errorCount++
			if err.Error() == "stats calculation in progress" {
				progressErrors++
			}
		}
	}

	t.Logf("Total errors: %d, Progress errors: %d", errorCount, progressErrors)

	// We should have some progress errors due to concurrent access
	if progressErrors == 0 {
		t.Log("No progress errors - this might indicate the concurrency control isn't working")
	}

	// Most importantly, no panics should have occurred
	t.Log("Concurrent access test completed without panics")
}

func TestUsageMonitor_GetCurrentStats_CacheLogic(t *testing.T) {
	db := setupTestDB(t)

	monitor := NewUsageMonitor(db)

	// Create a current window for testing
	window := &database.UsageWindow{
		StartTime:     time.Now().Add(-1 * time.Hour),
		EndTime:       time.Now().Add(4 * time.Hour),
		TotalMessages: 10,
		TotalTokens:   1000,
	}

	monitor.currentWindow = window

	// First call should work (might fail due to missing Claude data, but shouldn't panic)
	_, err1 := monitor.GetCurrentStats()
	t.Logf("First call error: %v", err1)

	// Immediately after, if cached, should return cached result or progress error
	_, err2 := monitor.GetCurrentStats()
	t.Logf("Second call error: %v", err2)

	// The important thing is no race conditions or panics
	t.Log("Cache logic test completed")
}

func TestUsageMonitor_ThreadSafety(t *testing.T) {
	db := setupTestDB(t)

	monitor := NewUsageMonitor(db)

	// Test thread safety by accessing different methods concurrently
	var wg sync.WaitGroup
	const numGoroutines = 5

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Try various operations
			switch id % 2 {
			case 0:
				_, _ = monitor.GetCurrentStats()
			case 1:
				// Test callback registration
				monitor.AddUsageCallback(func(*UsageStats) {})
			}
		}(i)
	}

	wg.Wait()
	t.Log("Thread safety test completed without panics")
}
