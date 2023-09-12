package wazevo

import (
	"runtime"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/isa/arm64"
)

func newMachine() backend.Machine {
	switch runtime.GOARCH {
	case "arm64":
		return arm64.NewBackend()
	default:
		panic("unsupported architecture")
	}
}

func newStackUnwinder() func(sp, top *byte) (returnAddresses []uintptr) {
	switch runtime.GOARCH {
	case "arm64":
		return arm64.StackUnwinder
	default:
		panic("unsupported architecture")
	}
}
