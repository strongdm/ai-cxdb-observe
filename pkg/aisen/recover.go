// recover.go provides the Recover helper for standalone panic recovery.
// Use this in HTTP handlers, goroutines, or other code outside of Runner.

package aisen

import (
	"context"
	"fmt"
	"runtime/debug"
)

// Recover captures a panic, records it to the collector, and returns the recovered value.
// Unlike RunWrapper, Recover does NOT re-panic after recording.
//
// Use in defer:
//
//	func handler(ctx context.Context) {
//	    defer aisen.Recover(ctx, collector)
//	    // code that might panic
//	}
//
// Or to capture the recovered value:
//
//	func handler(ctx context.Context) (err error) {
//	    defer func() {
//	        if r := aisen.Recover(ctx, collector); r != nil {
//	            err = fmt.Errorf("panic: %v", r)
//	        }
//	    }()
//	    // code that might panic
//	}
func Recover(ctx context.Context, collector Collector) any {
	r := recover()
	if r == nil {
		return nil
	}

	event := ErrorEvent{
		Severity:   SeverityCrash,
		ErrorType:  "panic",
		Message:    formatRecovered(r),
		StackTrace: string(debug.Stack()),
	}

	// Extract context ID from context if available
	if contextID, ok := ContextIDFromContext(ctx); ok {
		event.ContextID = &contextID
	}

	// Record the event (ignore errors - we don't want to affect caller)
	_ = collector.Record(ctx, event)

	return r
}

// formatRecovered formats a recovered panic value as a string.
func formatRecovered(recovered any) string {
	if recovered == nil {
		return "<nil>"
	}
	if err, ok := recovered.(error); ok {
		return err.Error()
	}
	return fmt.Sprintf("%v", recovered)
}
