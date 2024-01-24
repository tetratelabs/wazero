package wazevo

import (
	"runtime"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/isa/amd64"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/isa/arm64"
)

func newMachine() backend.Machine {
	switch runtime.GOARCH {
	case "arm64":
		return arm64.NewBackend()
	case "amd64":
		return amd64.NewBackend()
	default:
		panic("unsupported architecture")
	}
}

func unwindStack(sp, fp, top uintptr, returnAddresses []uintptr) []uintptr {
	switch runtime.GOARCH {
	case "arm64":
		return arm64.UnwindStack(sp, fp, top, returnAddresses)
	case "amd64":
		return amd64.UnwindStack(sp, fp, top, returnAddresses)
	default:
		panic("unsupported architecture")
	}
}

func goCallStackView(stackPointerBeforeGoCall *uint64) []uint64 {
	switch runtime.GOARCH {
	case "arm64":
		return arm64.GoCallStackView(stackPointerBeforeGoCall)
	case "amd64":
		return amd64.GoCallStackView(stackPointerBeforeGoCall)
	default:
		panic("unsupported architecture")
	}
}
