package wazevo

import (
	"context"
	"os"
	"testing"
	"unsafe"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

var ctx = context.Background()

func TestMain(m *testing.M) {
	if !platform.CompilerSupported() {
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestNewEngine(t *testing.T) {
	e := NewEngine(ctx, api.CoreFeaturesV1, nil)
	require.NotNil(t, e)
}

func TestEngine_CompiledModuleCount(t *testing.T) {
	e, ok := NewEngine(ctx, api.CoreFeaturesV1, nil).(*engine)
	require.True(t, ok)
	require.Equal(t, uint32(0), e.CompiledModuleCount())
	e.compiledModules[wasm.ModuleID{}] = &compiledModuleWithCount{compiledModule: &compiledModule{}, refCount: 1}
	require.Equal(t, uint32(1), e.CompiledModuleCount())
}

func TestEngine_DeleteCompiledModule(t *testing.T) {
	e, ok := NewEngine(ctx, api.CoreFeaturesV1, nil).(*engine)
	require.True(t, ok)
	id := wasm.ModuleID{0xaa}
	cm := &compiledModule{executables: &executables{executable: make([]byte, 1)}}
	cm2, err := e.addCompiledModule(&wasm.Module{ID: id}, cm)
	require.NoError(t, err)
	require.Same(t, cm, cm2)

	require.Equal(t, uint32(1), e.CompiledModuleCount())
	e.DeleteCompiledModule(&wasm.Module{ID: id})
	require.Equal(t, uint32(0), e.CompiledModuleCount())
}

func Test_ExecutionContextOffsets(t *testing.T) {
	var execCtx executionContext
	require.Equal(t, wazevoapi.Offset(unsafe.Offsetof(execCtx.exitCode)), wazevoapi.ExecutionContextOffsetExitCodeOffset)
	require.Equal(t, wazevoapi.Offset(unsafe.Offsetof(execCtx.callerModuleContextPtr)), wazevoapi.ExecutionContextOffsetCallerModuleContextPtr)
	require.Equal(t, wazevoapi.Offset(unsafe.Offsetof(execCtx.originalFramePointer)), wazevoapi.ExecutionContextOffsetOriginalFramePointer)
	require.Equal(t, wazevoapi.Offset(unsafe.Offsetof(execCtx.originalStackPointer)), wazevoapi.ExecutionContextOffsetOriginalStackPointer)
	require.Equal(t, wazevoapi.Offset(unsafe.Offsetof(execCtx.goReturnAddress)), wazevoapi.ExecutionContextOffsetGoReturnAddress)
	require.Equal(t, wazevoapi.Offset(unsafe.Offsetof(execCtx.goCallReturnAddress)), wazevoapi.ExecutionContextOffsetGoCallReturnAddress)
	require.Equal(t, wazevoapi.Offset(unsafe.Offsetof(execCtx.stackPointerBeforeGoCall)), wazevoapi.ExecutionContextOffsetStackPointerBeforeGoCall)
	require.Equal(t, wazevoapi.Offset(unsafe.Offsetof(execCtx.stackGrowRequiredSize)), wazevoapi.ExecutionContextOffsetStackGrowRequiredSize)
	require.Equal(t, wazevoapi.Offset(unsafe.Offsetof(execCtx.memoryGrowTrampolineAddress)), wazevoapi.ExecutionContextOffsetMemoryGrowTrampolineAddress)
	require.Equal(t, wazevoapi.Offset(unsafe.Offsetof(execCtx.stackGrowCallTrampolineAddress)), wazevoapi.ExecutionContextOffsetStackGrowCallTrampolineAddress)
	require.Equal(t, wazevoapi.Offset(unsafe.Offsetof(execCtx.checkModuleExitCodeTrampolineAddress)), wazevoapi.ExecutionContextOffsetCheckModuleExitCodeTrampolineAddress)
	require.Equal(t, wazevoapi.Offset(unsafe.Offsetof(execCtx.savedRegisters))%16, wazevoapi.Offset(0),
		"SavedRegistersBegin must be aligned to 16 bytes")
	require.Equal(t, wazevoapi.Offset(unsafe.Offsetof(execCtx.savedRegisters)), wazevoapi.ExecutionContextOffsetSavedRegistersBegin)
	require.Equal(t, wazevoapi.Offset(unsafe.Offsetof(execCtx.goFunctionCallCalleeModuleContextOpaque)), wazevoapi.ExecutionContextOffsetGoFunctionCallCalleeModuleContextOpaque)
	require.Equal(t, wazevoapi.Offset(unsafe.Offsetof(execCtx.tableGrowTrampolineAddress)), wazevoapi.ExecutionContextOffsetTableGrowTrampolineAddress)
	require.Equal(t, wazevoapi.Offset(unsafe.Offsetof(execCtx.refFuncTrampolineAddress)), wazevoapi.ExecutionContextOffsetRefFuncTrampolineAddress)
	require.Equal(t, wazevoapi.Offset(unsafe.Offsetof(execCtx.memmoveAddress)), wazevoapi.ExecutionContextOffsetMemmoveAddress)
	require.Equal(t, wazevoapi.Offset(unsafe.Offsetof(execCtx.framePointerBeforeGoCall)), wazevoapi.ExecutionContextOffsetFramePointerBeforeGoCall)
	require.Equal(t, wazevoapi.Offset(unsafe.Offsetof(execCtx.memoryWait32TrampolineAddress)), wazevoapi.ExecutionContextOffsetMemoryWait32TrampolineAddress)
	require.Equal(t, wazevoapi.Offset(unsafe.Offsetof(execCtx.memoryWait64TrampolineAddress)), wazevoapi.ExecutionContextOffsetMemoryWait64TrampolineAddress)
	require.Equal(t, wazevoapi.Offset(unsafe.Offsetof(execCtx.memoryNotifyTrampolineAddress)), wazevoapi.ExecutionContextOffsetMemoryNotifyTrampolineAddress)
	require.Equal(t, wazevoapi.Offset(unsafe.Offsetof(execCtx.throwTrampolineAddress)), wazevoapi.ExecutionContextOffsetThrowTrampolineAddress)
	require.Equal(t, wazevoapi.Offset(unsafe.Offsetof(execCtx.tryTableEnterTrampolineAddress)), wazevoapi.ExecutionContextOffsetTryTableEnterTrampolineAddress)
	require.Equal(t, wazevoapi.Offset(unsafe.Offsetof(execCtx.tryTableLeaveTrampolineAddress)), wazevoapi.ExecutionContextOffsetTryTableLeaveTrampolineAddress)
	require.Equal(t, wazevoapi.Offset(unsafe.Offsetof(execCtx.throwAllocTrampolineAddress)), wazevoapi.ExecutionContextOffsetThrowAllocTrampolineAddress)
	require.Equal(t, wazevoapi.Offset(unsafe.Offsetof(execCtx.exceptionPtr)), wazevoapi.ExecutionContextOffsetExceptionPtr)
	require.Equal(t, wazevoapi.Offset(unsafe.Offsetof(execCtx.exceptionParamsPtr)), wazevoapi.ExecutionContextOffsetExceptionParamsPtr)
	require.Equal(t, wazevoapi.Offset(unsafe.Offsetof(execCtx.caughtExceptionClauseIdx)), wazevoapi.ExecutionContextOffsetCaughtExceptionClauseIdx)
	require.Equal(t, wazevoapi.Offset(unsafe.Offsetof(execCtx.localsSaveAreaPtr)), wazevoapi.ExecutionContextOffsetLocalsSaveAreaPtr)
}
