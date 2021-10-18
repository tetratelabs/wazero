//go:build !disablew_callstack_overflow_check

package buildoptions

const (
	CheckCallStackOverflow = true
	CallStackHeightLimit   = 2000
)
