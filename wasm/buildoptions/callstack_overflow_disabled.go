//go:build disable_callstack_overflow_check

package buildoptions

const (
	CheckCallStackOverflow = false
	CallStackHeightLimit   = 0
)
