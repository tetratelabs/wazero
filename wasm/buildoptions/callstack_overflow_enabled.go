//go:build !disable_callstack_overflow_check
// +build !disable_callstack_overflow_check

package buildoptions

const (
	CheckCallStackOverflow = true
	CallStackHeightLimit   = 2000
)
