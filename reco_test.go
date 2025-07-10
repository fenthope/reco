package reco

import (
	"io"
	"testing"
	"time" // For fields if needed
)

// Base benchmark function to reduce repetition
func benchmarkLogger(b *testing.B, mode OutputMode, async bool, enableCaller bool, withFields bool) {
	cfg := Config{
		Level:           LevelDebug, // Process all logs
		Mode:            mode,
		Async:           async,
		Output:          io.Discard, // Discard output to measure CPU/memory, not I/O
		EnableCaller:    enableCaller,
		TimeFormat:      time.RFC3339Nano, // Consistent time format
		EnableRotation:  false,            // Disable rotation features
		CompressBackups: false,
		// DefaultFields: nil, // DefaultFields are added if withFields is true, via cfg.DefaultFields
		BufferSize: defaultBufferSize, // Use defaults for async
		CallerSkip: DefaultCallerSkip, // Standard skip
	}

	// Add DefaultFields if testing withFields scenario.
	// These are distinct from the fields added per log call.
	// For simplicity, let's assume 'withFields' means fields per log call,
	// and DefaultFields could be a separate dimension if needed.
	// The current `logFields` variable handles per-call fields.
	// If `withFields` is true, we can also add some DefaultFields to stress that path.
	if withFields {
		// cfg.DefaultFields = Fields{"default_field1": "value1", "default_field2": 123}
	}


	logger, err := New(cfg)
	if err != nil {
		b.Fatalf("Failed to create logger: %v", err)
	}

	// b.Cleanup ensures logger.Close is called even if the benchmark panics or calls b.Fatal.
	// This is important for async loggers to flush their log queues.
	b.Cleanup(func() {
		if err := logger.Close(); err != nil {
			// Log the error but don't fail the benchmark at cleanup,
			// as the benchmark run itself might have already passed or failed.
			// b.Errorf("Error closing logger: %v", err)
			// Or simply ignore, as "logger already closed" is possible if test calls Close.
		}
	})

	b.ReportAllocs()
	b.ResetTimer() // Start timing after all setup.

	logMessage := "This is a benchmark log message for testing purposes"
	var logFields Fields
	if withFields {
		// These are the fields added at each log call.
		logFields = Fields{
			"field_A": "string_value_for_benchmark",
			"field_B": 456.7890123,
			"field_C": true,
			"time_D":  time.Now(),
		}
	}

	// The actual benchmark loop
	for i := 0; i < b.N; i++ {
		if withFields {
			// Using Info (not Infof) as it exercises the Fields logic path
			logger.Info(logMessage, logFields)
		} else {
			logger.Info(logMessage)
		}
	}

	b.StopTimer() // Stop timing before Cleanup functions (like logger.Close()) are called.
	// logger.Close() is handled by b.Cleanup
}

// --- Text Mode Benchmarks ---
func BenchmarkTextSyncSimple(b *testing.B) {
	benchmarkLogger(b, ModeText, false, false, false)
}
func BenchmarkTextSyncWithFields(b *testing.B) {
	benchmarkLogger(b, ModeText, false, false, true)
}
func BenchmarkTextSyncWithCaller(b *testing.B) {
	benchmarkLogger(b, ModeText, false, true, false)
}
func BenchmarkTextAsyncSimple(b *testing.B) {
	benchmarkLogger(b, ModeText, true, false, false)
}
func BenchmarkTextAsyncWithFields(b *testing.B) {
	benchmarkLogger(b, ModeText, true, false, true)
}
func BenchmarkTextAsyncWithCaller(b *testing.B) {
	benchmarkLogger(b, ModeText, true, true, false)
}

// --- JSON Mode Benchmarks ---
func BenchmarkJSONSyncSimple(b *testing.B) {
	benchmarkLogger(b, ModeJSON, false, false, false)
}
func BenchmarkJSONSyncWithFields(b *testing.B) {
	benchmarkLogger(b, ModeJSON, false, false, true)
}
func BenchmarkJSONSyncWithCaller(b *testing.B) {
	benchmarkLogger(b, ModeJSON, false, true, false)
}
func BenchmarkJSONAsyncSimple(b *testing.B) {
	benchmarkLogger(b, ModeJSON, true, false, false)
}
func BenchmarkJSONAsyncWithFields(b *testing.B) {
	benchmarkLogger(b, ModeJSON, true, false, true)
}
func BenchmarkJSONAsyncWithCaller(b *testing.B) {
	benchmarkLogger(b, ModeJSON, true, true, false)
}
