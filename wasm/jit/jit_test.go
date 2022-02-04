package jit

import (
	"os"
	"runtime"
	"testing"
)

func TestMain(m *testing.M) {
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		// JIT is currently implemented only for amd64 or arm64.
		os.Exit(0)
	}
	os.Exit(m.Run())
}
