//go:build !(unix || windows) || tinygo

package platform

import (
	"fmt"
	"runtime"
)

var errUnsupported = fmt.Errorf("mmap unsupported on GOOS=%s. Use interpreter instead.", runtime.GOOS)

func mmapCodeSegment(size int, exec bool) ([]byte, error) {
	panic(errUnsupported)
}

func munmapCodeSegment(code []byte) error {
	panic(errUnsupported)
}

func MprotectRX(b []byte) (err error) {
	panic(errUnsupported)
}
