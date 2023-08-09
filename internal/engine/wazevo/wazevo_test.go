package wazevo

import (
	"context"
	"os"
	"runtime"
	"testing"
	"unsafe"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

var ctx = context.Background()

func TestMain(m *testing.M) {
	if runtime.GOARCH != "arm64" {
		os.Exit(0)
	}
}

func TestNewEngine(t *testing.T) {
	e := NewEngine(ctx, api.CoreFeaturesV1, nil)
	require.NotNil(t, e)
}

func TestEngine_CompiledModuleCount(t *testing.T) {
	e, ok := NewEngine(ctx, api.CoreFeaturesV1, nil).(*engine)
	require.True(t, ok)
	require.Equal(t, uint32(0), e.CompiledModuleCount())
	e.compiledModules[wasm.ModuleID{}] = &compiledModule{}
	require.Equal(t, uint32(1), e.CompiledModuleCount())
}

func TestEngine_DeleteCompiledModule(t *testing.T) {
	e, ok := NewEngine(ctx, api.CoreFeaturesV1, nil).(*engine)
	require.True(t, ok)
	id := wasm.ModuleID{0xaa}
	e.compiledModules[id] = &compiledModule{}
	require.Equal(t, uint32(1), e.CompiledModuleCount())
	e.DeleteCompiledModule(&wasm.Module{ID: id})
	require.Equal(t, uint32(0), e.CompiledModuleCount())
}

func Test_ExecutionContextOffsets(t *testing.T) {
	offsets := wazevoapi.ExecutionContextOffsets

	var execCtx executionContext
	require.Equal(t, wazevoapi.Offset(unsafe.Offsetof(execCtx.exitCode)), offsets.ExitCodeOffset)
	require.Equal(t, wazevoapi.Offset(unsafe.Offsetof(execCtx.callerModuleContextPtr)), offsets.CallerModuleContextPtr)
	require.Equal(t, wazevoapi.Offset(unsafe.Offsetof(execCtx.originalFramePointer)), offsets.OriginalFramePointer)
	require.Equal(t, wazevoapi.Offset(unsafe.Offsetof(execCtx.originalStackPointer)), offsets.OriginalStackPointer)
	require.Equal(t, wazevoapi.Offset(unsafe.Offsetof(execCtx.goReturnAddress)), offsets.GoReturnAddress)
	require.Equal(t, wazevoapi.Offset(unsafe.Offsetof(execCtx.goCallReturnAddress)), offsets.GoCallReturnAddress)
	require.Equal(t, wazevoapi.Offset(unsafe.Offsetof(execCtx.stackPointerBeforeGrow)), offsets.StackPointerBeforeGrow)
	require.Equal(t, wazevoapi.Offset(unsafe.Offsetof(execCtx.stackGrowRequiredSize)), offsets.StackGrowRequiredSize)
	require.Equal(t, wazevoapi.Offset(unsafe.Offsetof(execCtx.savedRegisters)), offsets.SavedRegistersBegin)
	require.Equal(t, wazevoapi.Offset(unsafe.Offsetof(execCtx.savedRegisters))%16, wazevoapi.Offset(0))
}
