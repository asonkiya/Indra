// Package gosigar is a no-op shim used for iOS/mobile builds where the real
// gosigar (which requires macOS-only libproc.h) cannot compile.
package gosigar

// Mem holds system memory stats. On mobile this returns zeros.
type Mem struct {
	Total      uint64
	Used       uint64
	Free       uint64
	Cached     uint64
	ActualFree uint64
	ActualUsed uint64
}

// Get populates Mem. No-op on mobile.
func (m *Mem) Get() error { return nil }

// Swap holds swap stats.
type Swap struct {
	Total uint64
	Used  uint64
	Free  uint64
}

// Get populates Swap. No-op on mobile.
func (s *Swap) Get() error { return nil }
