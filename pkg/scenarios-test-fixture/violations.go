// Package scenariostestfixture is a deliberate test fixture used by
// the bborbe/coding scenario 004 acceptance suite. Each declaration
// below intentionally violates one or more ast-grep YAML rules from
// the coding plugin's rule base. The PR (bborbe/maintainer#2,
// branch delete-this-pr-never) stays open as a stable target for
// /coding:pr-review acceptance walks. DO NOT MERGE.
package scenariostestfixture

import (
	"fmt"
	"time"
)

// Service1 + NewService1: violates go-architecture/constructor-returns-interface
// (constructor returns the concrete *Service1 instead of an interface).
type Service1 struct{}

// NewService1 is the constructor for Service1.
func NewService1() *Service1 { return &Service1{} }

// sharedService1: violates go-architecture/no-globals-or-singletons
// (package-level variable holding a service constructor result).
var sharedService1 = NewService1()

// Use sharedService1 so the compiler doesn't strip it.
var _ = sharedService1

// Now: violates go-time/no-time-now-direct
// (direct time.Now() call in production code).
func Now() time.Time { return time.Now() }

// Run: violates go-concurrency/no-raw-go-func
// (bare `go func(){...}()` instead of an injected goroutine launcher).
func Run() { go func() { _ = 1 }() }

// Boom: violates go-errors/no-fmt-errorf
// (fmt.Errorf in production code instead of bborbe/errors.Wrapf).
func Boom() error { return fmt.Errorf("boom: %d", 42) }
