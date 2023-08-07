//go:build plan9 || js

package sock

// plan9/js doesn't declare these constants
const (
	SHUT_RD = iota
	SHUT_RDWR
	SHUT_WR
)
