package wazevo

import (
	"context"
	"fmt"
	"reflect"
	"runtime"
	"slices"
	"sync/atomic"
	"unsafe"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
	"github.com/tetratelabs/wazero/internal/expctxkeys"
	"github.com/tetratelabs/wazero/internal/internalapi"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasmdebug"
	"github.com/tetratelabs/wazero/internal/wasmruntime"
)

type (
	// callEngine implements api.Function.
	callEngine struct {
		internalapi.WazeroOnly
		stack []byte
		// stackTop is the pointer to the *aligned* top of the stack. This must be updated
		// whenever the stack is changed. This is passed to the assembly function
		// at the very beginning of api.Function Call/CallWithStack.
		stackTop uintptr
		// executable is the pointer to the executable code for this function.
		executable         *byte
		preambleExecutable *byte
		// parent is the *moduleEngine from which this callEngine is created.
		parent *moduleEngine
		// indexInModule is the index of the function in the module.
		indexInModule wasm.Index
		// sizeOfParamResultSlice is the size of the parameter/result slice.
		sizeOfParamResultSlice int
		requiredParams         int
		// execCtx holds various information to be read/written by assembly functions.
		execCtx executionContext
		// execCtxPtr holds the pointer to the executionContext which doesn't change after callEngine is created.
		execCtxPtr        uintptr
		numberOfResults   int
		stackIteratorImpl stackIterator
		// heldExceptions is the reference table for exnrefs stored in locals/stack slots.
		//
		// What a global or table slot holds is counted separately, in the engine's
		// wasm.ExceptionStore; an exception is alive while either count is above zero.
		heldExceptions map[wasm.Reference]*exceptionRefs
		// thrown is the raise this call is propagating, and is nil when it is not
		// propagating one. A call that never throws never allocates it.
		thrown *thrownException
		// tryHandlers is the stack of active try_table exception handlers, used to find the
		// one that catches a raise.
		tryHandlers []tryHandler
	}

	// tryHandler records the state at a try_table entry for exception handling.
	// On match, we restore the stack to the checkpoint state and re-enter at returnAddress.
	tryHandler struct {
		// Cloned stack and state from the try_table entry checkpoint,
		// using the same approach as experimental.Snapshot.
		sp, fp, top    uintptr
		returnAddress  *byte
		savedRegisters [64][2]uint64
		stack          []byte // cloned stack
		// catchClauses describes what exceptions this handler catches.
		catchClauses []wazevoapi.CatchClauseInstance
		// localsSaveArea is a heap buffer where locals are mirrored inside
		// try bodies. Handlers read from it to get updated values.
		// Nil for nested same-function try_tables that reuse the enclosing
		// handler's save area.
		localsSaveArea []uint64
		// moduleInstance is the module that set up this try handler.
		// Used for tag matching in doHandleException (the tag index in
		// catch clauses is relative to this module's tag index space).
		moduleInstance *wasm.ModuleInstance
		// heldExceptions is the reference table as it was here. Rewinding to this
		// checkpoint restores the operand stack and locals, so it has to restore what they
		// held too -- which is how a raise releases the references of every frame and slot
		// it discards, including those of frames it passes clean through, whose compiled
		// code never runs again to release anything itself.
		heldExceptions map[wasm.Reference]*exceptionRefs
		// exnrefLocals and effectiveSaveArea say where the exnref locals a handler will
		// carry across the rewind are. Those values outlive it while the references they
		// held do not, so the rewind takes new ones for them. See restoreCheckpoint.
		exnrefLocals      []uint32
		effectiveSaveArea []uint64
		// frameDepth is how many wasm frames were on the stack here. Catching unwinds
		// whatever is above that, which is what says how many frames a raise took -- and so
		// which listeners have to be told.
		frameDepth int
	}

	// exceptionRefs is an exception and the number of references to it in this call. The table
	// holds it by pointer so that a reference coming or going is one hash lookup and an
	// increment through it, rather than a lookup, a copy out and a store back.
	exceptionRefs struct {
		exn   *wasm.Exception
		count int32
	}

	// thrownException is one raise, from the throw or throw_ref that starts it to the handler
	// that catches it, and lives no longer than that.
	thrownException struct {
		// tag is what was thrown.
		tag *wasm.TagInstance
		// params holds the tag's argument values, which compiled code stores here at the
		// throw.
		params []uint64
		// trace is the wasm return addresses of where the raise started, innermost first.
		// Propagation unwinds by returning, so by the time an exception turns out to be
		// uncaught its frames are gone; this is captured at the raise, while the stack
		// still reaches it.
		trace []uintptr
		// rethrown is the exception a throw_ref re-raised, and nil when this raise is a fresh
		// throw.
		rethrown *wasm.Exception
	}

	// executionContext is the struct to be read/written by assembly functions.
	executionContext struct {
		// exitCode holds the wazevoapi.ExitCode describing the state of the function execution.
		exitCode wazevoapi.ExitCode
		// callerModuleContextPtr holds the moduleContextOpaque for Go function calls.
		callerModuleContextPtr *byte
		// originalFramePointer holds the original frame pointer of the caller of the assembly function.
		originalFramePointer uintptr
		// originalStackPointer holds the original stack pointer of the caller of the assembly function.
		originalStackPointer uintptr
		// goReturnAddress holds the return address to go back to the caller of the assembly function.
		goReturnAddress uintptr
		// stackBottomPtr holds the pointer to the bottom of the stack.
		stackBottomPtr *byte
		// goCallReturnAddress holds the return address to go back to the caller of the Go function.
		goCallReturnAddress *byte
		// stackPointerBeforeGoCall holds the stack pointer before calling a Go function.
		stackPointerBeforeGoCall *uint64
		// stackGrowRequiredSize holds the required size of stack grow.
		stackGrowRequiredSize uintptr
		// memoryGrowTrampolineAddress holds the address of memory grow trampoline function.
		memoryGrowTrampolineAddress *byte
		// stackGrowCallTrampolineAddress holds the address of stack grow trampoline function.
		stackGrowCallTrampolineAddress *byte
		// checkModuleExitCodeTrampolineAddress holds the address of check-module-exit-code function.
		checkModuleExitCodeTrampolineAddress *byte
		// savedRegisters is the opaque spaces for save/restore registers.
		// We want to align 16 bytes for each register, so we use [64][2]uint64.
		savedRegisters [64][2]uint64
		// goFunctionCallCalleeModuleContextOpaque is the pointer to the target Go function's moduleContextOpaque.
		goFunctionCallCalleeModuleContextOpaque uintptr
		// tableGrowTrampolineAddress holds the address of table grow trampoline function.
		tableGrowTrampolineAddress *byte
		// refFuncTrampolineAddress holds the address of ref-func trampoline function.
		refFuncTrampolineAddress *byte
		// memmoveAddress holds the address of memmove function implemented by Go runtime. See memmove.go.
		memmoveAddress uintptr
		// framePointerBeforeGoCall holds the frame pointer before calling a Go function. Note: only used in amd64.
		framePointerBeforeGoCall uintptr
		// memoryWait32TrampolineAddress holds the address of memory_wait32 trampoline function.
		memoryWait32TrampolineAddress *byte
		// memoryWait32TrampolineAddress holds the address of memory_wait64 trampoline function.
		memoryWait64TrampolineAddress *byte
		// memoryNotifyTrampolineAddress holds the address of the memory_notify trampoline function.
		memoryNotifyTrampolineAddress *byte
		// throwAllocTrampolineAddress holds the address of the throw-alloc trampoline:
		// phase 1 of throw, which records the raise and returns the params buffer.
		throwAllocTrampolineAddress *byte
		// throwTrampolineAddress holds the address of the throw/throw_ref trampoline function.
		throwTrampolineAddress *byte
		// tryTableEnterTrampolineAddress holds the address of the try_table enter trampoline function.
		tryTableEnterTrampolineAddress *byte
		// tryTableLeaveTrampolineAddress holds the address of the try_table leave trampoline function.
		tryTableLeaveTrampolineAddress *byte
		// exnrefSlotLoadTrampolineAddress and exnrefSlotStoreTrampolineAddress hold the
		// addresses of the barriers an exnref-typed global or table slot is accessed
		// through. Compiled code passes the address of the slot and lets the runtime do the
		// access itself, so that it cannot race another barrier on the same slot.
		exnrefSlotLoadTrampolineAddress  *byte
		exnrefSlotStoreTrampolineAddress *byte
		// exnrefSlotFillTrampolineAddress and exnrefSlotCopyTrampolineAddress hold the
		// addresses of the barriers over a run of exnref-typed table slots, which the bulk
		// table operations write.
		exnrefSlotFillTrampolineAddress *byte
		exnrefSlotCopyTrampolineAddress *byte
		// adjustExnrefsTrampolineAddress holds the address of the trampoline compiled code
		// calls to adjust this call's exnref reference counts.
		adjustExnrefsTrampolineAddress *byte
		// caughtExceptionParams points at the params of the exception a handler was just
		// entered for. The raise trampoline sets it; the handler loads the values out of it
		// and nothing reads it after that, so the next raise is free to overwrite it.
		//
		// Its purpose is to be a Go pointer: compiled code reads the params through this
		// address, and the field being pointer-typed is what keeps the array from being
		// collected while it does -- a pointer to an object's first word keeps the whole
		// object alive.
		caughtExceptionParams *uint64
		// caughtExceptionRef holds the handle naming the exception a handler was just
		// entered for, which catch_ref and catch_all_ref hand to guest code as an exnref.
		caughtExceptionRef uintptr
		// caughtExceptionClauseIdx is -1 on the normal path through a try_table entry, and
		// the matched catch clause index when a raise re-enters one. Compiled code reads it
		// to decide which handler to dispatch to.
		caughtExceptionClauseIdx int64
		// localsSaveAreaPtr points to the tryHandler's localsSaveArea slice
		// backing array. Handlers load locals from this slice.
		localsSaveAreaPtr uintptr
	}
)

func (c *callEngine) requiredInitialStackSize() int {
	const initialStackSizeDefault = 10240
	stackSize := initialStackSizeDefault
	paramResultInBytes := c.sizeOfParamResultSlice * 8 * 2 // * 8 because uint64 is 8 bytes, and *2 because we need both separated param/result slots.
	required := paramResultInBytes + 32 + 16               // 32 is enough to accommodate the call frame info, and 16 exists just in case when []byte is not aligned to 16 bytes.
	if required > stackSize {
		stackSize = required
	}
	return stackSize
}

func (c *callEngine) init() {
	stackSize := c.requiredInitialStackSize()
	if wazevoapi.StackGuardCheckEnabled {
		stackSize += wazevoapi.StackGuardCheckGuardPageSize
	}
	c.stack = make([]byte, stackSize)
	c.stackTop = alignedStackTop(c.stack)
	if wazevoapi.StackGuardCheckEnabled {
		c.execCtx.stackBottomPtr = &c.stack[wazevoapi.StackGuardCheckGuardPageSize]
	} else {
		c.execCtx.stackBottomPtr = &c.stack[0]
	}
	c.execCtxPtr = uintptr(unsafe.Pointer(&c.execCtx))
}

// alignedStackTop returns 16-bytes aligned stack top of given stack.
// 16 bytes should be good for all platform (arm64/amd64).
func alignedStackTop(s []byte) uintptr {
	stackAddr := uintptr(unsafe.Pointer(&s[len(s)-1]))
	return stackAddr - (stackAddr & (16 - 1))
}

// Definition implements api.Function.
func (c *callEngine) Definition() api.FunctionDefinition {
	return c.parent.module.Source.FunctionDefinition(c.indexInModule)
}

// Call implements api.Function.
func (c *callEngine) Call(ctx context.Context, params ...uint64) ([]uint64, error) {
	if c.requiredParams != len(params) {
		return nil, fmt.Errorf("expected %d params, but passed %d", c.requiredParams, len(params))
	}
	paramResultSlice := make([]uint64, c.sizeOfParamResultSlice)
	copy(paramResultSlice, params)
	if err := c.callWithStack(ctx, paramResultSlice); err != nil {
		return nil, err
	}
	return paramResultSlice[:c.numberOfResults], nil
}

// storeExnrefSlot is the write barrier for an exnref-typed global or table slot. The handle
// comes from guest code, so this call must hold what it names.
func (c *callEngine) storeExnrefSlot(slot *wasm.Reference, handle wasm.Reference) {
	c.storeSlotHoldingOperand(slot, handle)
	c.releaseIfHeld(handle)
}

// storeSlotHoldingOperand is storeExnrefSlot without releasing the operand's reference, for
// the bulk fill: one operand fills many slots, so it is one reference to release once at the
// end rather than per slot.
func (c *callEngine) storeSlotHoldingOperand(slot *wasm.Reference, handle wasm.Reference) {
	var exn *wasm.Exception
	if handle != 0 {
		if exn = c.heldException(handle); exn == nil {
			panic(wasmruntime.ErrRuntimeExpiredExceptionRef)
		}
	}
	c.parent.parent.parent.exceptions.StoreSlot(slot, exn)
}

// loadExnrefSlot is the read barrier: what a slot names becomes reachable from this call,
// so it has to stay resolvable even if the slot is overwritten before the call ends.
func (c *callEngine) loadExnrefSlot(slot *wasm.Reference) wasm.Reference {
	exn := c.parent.parent.parent.exceptions.LoadSlot(slot)
	if exn == nil {
		return 0
	}
	c.holdException(exn)
	return exn.ID
}

// exceptionOf returns the exception naming this raise: the one a throw_ref re-raised, or a new
// one around what a throw left behind.
func (c *callEngine) exceptionOf(t *thrownException) *wasm.Exception {
	if t.rethrown != nil {
		return t.rethrown
	}
	// Only this call can resolve the exnref params: it is the one that had them on its
	// operand stack. Ones it cannot reach are left out -- nothing can reach those, so
	// there is nothing to keep alive.
	var paramRefs []*wasm.Exception
	for i, vt := range t.tag.Type.Params {
		if wasm.IsExnref(vt) {
			if p := c.heldException(wasm.Reference(t.params[i])); p != nil {
				paramRefs = append(paramRefs, p)
			}
		}
	}
	return c.parent.parent.parent.exceptions.NewException(t.tag, t.params, t.trace, paramRefs)
}

// heldException is the exception this call holds under the given handle, and nil when it holds
// none -- which is the same answer for a handle that never named one and for a handle whose
// last reference has gone, because guest code has no way to tell those apart either.
func (c *callEngine) heldException(handle wasm.Reference) *wasm.Exception {
	if e := c.heldExceptions[handle]; e != nil {
		return e.exn
	}
	return nil
}

// holdException records one more reference to exn. Every path by which guest code comes to
// hold an exnref goes through here, which is what makes the count a complete answer rather
// than a lower bound -- a reference the compiler creates without saying so would let the
// count reach zero while guest code still has the handle.
func (c *callEngine) holdException(exn *wasm.Exception) {
	if e := c.heldExceptions[exn.ID]; e != nil {
		e.count++
		return
	}
	if c.heldExceptions == nil {
		c.heldExceptions = make(map[wasm.Reference]*exceptionRefs)
	}
	c.heldExceptions[exn.ID] = &exceptionRefs{exn: exn, count: 1}
}

// releaseIfHeld drops one reference unless the handle is `ref.null exn`, which never had one.
func (c *callEngine) releaseIfHeld(handle wasm.Reference) {
	if handle != 0 {
		c.releaseException(handle)
	}
}

// releaseException drops one reference. The exception stops being resolvable by this call
// once the last one goes, which is what bounds the table to what guest code can still reach.
func (c *callEngine) releaseException(handle wasm.Reference) {
	e := c.heldExceptions[handle]
	if e == nil {
		// A handle with no reference means the compiler emitted a release without a matching
		// hold. Staying quiet here would turn that into a use-after-free somewhere further
		// away, so it is worth failing at the point the accounting first disagrees.
		panic(fmt.Sprintf("BUG: released exnref %#x that this call holds no reference to", handle))
	}
	if e.count--; e.count == 0 {
		delete(c.heldExceptions, handle)
	}
}

// holdParamRefs holds the exceptions named by exn's exnref params, which a tag-matched
// clause is about to push for the handler.
func (c *callEngine) holdParamRefs(exn *wasm.Exception) {
	for _, p := range exn.ParamRefs {
		c.holdException(p)
	}
}

// abortListenerOf notifies the listener of the function containing addr, if it has one,
// that an exception is unwinding its frame. Compiled code calls the after-listener on the
// return paths, which a frame an exception takes out never reaches.
func (c *callEngine) abortListenerOf(ctx context.Context, addr uintptr) {
	me := c.moduleEngineOfAddr(addr)
	if me == nil {
		return
	}
	cm := me.parent
	if len(cm.listeners) == 0 {
		return
	}
	index := cm.functionIndexOf(addr)
	if lsn := cm.listeners[index]; lsn != nil {
		def := cm.module.FunctionDefinition(cm.module.ImportFunctionCount + index)
		lsn.Abort(ctx, me.module, def, experimental.ErrUnwoundByException)
	}
}

// moduleEngineOfAddr is the module engine whose executable holds addr, which says both what
// function the frame is in and which instance it belongs to -- a listener is told the module
// the function it is watching is in, not the one the exception came from.
//
// The search is over the instances this call can reach: the one it is running in, and the
// ones it imports from, which between them cover the frames a call can have. Unwinding runs
// this per frame, so it is a couple of range checks rather than a search over everything the
// engine holds.
func (c *callEngine) moduleEngineOfAddr(addr uintptr) *moduleEngine {
	if checkAddrInBytes(addr, c.parent.parent.executable) {
		return c.parent
	}
	for i := range c.parent.importedFunctions {
		if me := c.parent.importedFunctions[i].me; checkAddrInBytes(addr, me.parent.executable) {
			return me
		}
	}
	return nil
}

func (c *callEngine) addFrame(builder wasmdebug.ErrorBuilder, addr uintptr) (def api.FunctionDefinition, listener experimental.FunctionListener) {
	eng := c.parent.parent.parent
	cm := eng.compiledModuleOfAddr(addr)
	if cm == nil {
		// This case, the module might have been closed and deleted from the engine.
		// We fall back to searching the imported modules that can be referenced from this callEngine.

		// First, we check itself.
		if checkAddrInBytes(addr, c.parent.parent.executable) {
			cm = c.parent.parent
		} else {
			// Otherwise, search all imported modules. TODO: maybe recursive, but not sure it's useful in practice.
			p := c.parent
			for i := range p.importedFunctions {
				candidate := p.importedFunctions[i].me.parent
				if checkAddrInBytes(addr, candidate.executable) {
					cm = candidate
					break
				}
			}
		}
	}

	if cm != nil {
		index := cm.functionIndexOf(addr)
		def = cm.module.FunctionDefinition(cm.module.ImportFunctionCount + index)
		var sources []string
		if dw := cm.module.DWARFLines; dw != nil {
			sourceOffset := cm.getSourceOffset(addr)
			sources = dw.Line(sourceOffset)
		}
		builder.AddFrame(def.DebugName(), def.ParamTypes(), def.ResultTypes(), sources)
		if len(cm.listeners) > 0 {
			listener = cm.listeners[index]
		}
	}
	return
}

// CallWithStack implements api.Function.
func (c *callEngine) CallWithStack(ctx context.Context, paramResultStack []uint64) (err error) {
	if c.sizeOfParamResultSlice > len(paramResultStack) {
		return fmt.Errorf("need %d params, but stack size is %d", c.sizeOfParamResultSlice, len(paramResultStack))
	}
	return c.callWithStack(ctx, paramResultStack)
}

// CallWithStack implements api.Function.
func (c *callEngine) callWithStack(ctx context.Context, paramResultStack []uint64) (err error) {
	snapshotEnabled := ctx.Value(expctxkeys.EnableSnapshotterKey{}) != nil
	if snapshotEnabled {
		ctx = context.WithValue(ctx, expctxkeys.SnapshotterKey{}, c)
	}

	if wazevoapi.StackGuardCheckEnabled {
		defer func() {
			wazevoapi.CheckStackGuardPage(c.stack)
		}()
	}

	p := c.parent
	ensureTermination := p.parent.ensureTermination
	m := p.module
	if ensureTermination {
		select {
		case <-ctx.Done():
			// If the provided context is already done, close the module and return the error.
			m.CloseWithCtxErr(ctx)
			return m.FailIfClosed()
		default:
		}
	}

	// Clear any stale in-flight exception state from a previous call.
	c.thrown = nil
	c.tryHandlers = c.tryHandlers[:0]
	c.execCtx.caughtExceptionParams = nil
	c.execCtx.caughtExceptionRef, c.execCtx.localsSaveAreaPtr = 0, 0
	clear(c.heldExceptions)

	var paramResultPtr *uint64
	if len(paramResultStack) > 0 {
		paramResultPtr = &paramResultStack[0]
	}
	defer func() {
		r := recover()
		if s, ok := r.(*snapshot); ok {
			// A snapshot that wasn't handled was created by a different call engine possibly from a nested wasm invocation,
			// let it propagate up to be handled by the caller.
			panic(s)
		}
		if r != nil {
			type listenerForAbort struct {
				def api.FunctionDefinition
				lsn experimental.FunctionListener
			}

			var listeners []listenerForAbort
			builder := wasmdebug.NewErrorBuilder()
			addFrame := func(addr uintptr) {
				def, lsn := c.addFrame(builder, addr)
				if lsn != nil {
					listeners = append(listeners, listenerForAbort{def, lsn})
				}
			}
			// An uncaught exception, as opposed to a trap, which aborts on the live stack.
			if t := c.thrown; t != nil {
				trace := t.trace
				// Uncaught. The frames are still live here, but the trace captured at the
				// raise is what the frames it already unwound are recorded in, and it is the
				// same stack either way. These frames get no listener notification: each was
				// aborted as it was unwound.
				for _, retAddr := range trace {
					c.addFrame(builder, retAddr)
				}
				// Thrown somewhere other than where it was last thrown: say both. These
				// frames are long gone, so they are context, not the stack that aborted.
				// Only an exception carried here by throw_ref has an origin of its own; a
				// fresh throw's is the raise trace itself.
				var origin []uintptr
				if t.rethrown != nil {
					origin, _ = t.rethrown.Origin.([]uintptr)
				}
				if len(origin) > 0 && !slices.Equal(origin, trace) {
					builder.StartSection(wasmdebug.ExceptionOriginSection)
					for _, retAddr := range origin {
						c.addFrame(builder, retAddr)
					}
				}
			} else if c.execCtx.stackPointerBeforeGoCall != nil {
				addFrame(uintptr(unsafe.Pointer(c.execCtx.goCallReturnAddress)))
				returnAddrs := unwindStack(
					uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)),
					c.execCtx.framePointerBeforeGoCall,
					c.stackTop,
					nil,
					wasmdebug.MaxFrames,
				)
				if len(returnAddrs) > 1 {
					for _, retAddr := range returnAddrs[:len(returnAddrs)-1] { // the last return addr is the trampoline, so we skip it.
						addFrame(retAddr)
					}
				}
			}
			err = builder.FromRecovered(r)

			for _, lsn := range listeners {
				lsn.lsn.Abort(ctx, m, lsn.def, err)
			}
		} else {
			if err != wasmruntime.ErrRuntimeStackOverflow { // Stackoverflow case shouldn't be panic (to avoid extreme stack unwinding).
				err = c.parent.module.FailIfClosed()
			}
		}

		if err != nil {
			// Ensures that we can reuse this callEngine even after an error.
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
		}
		if err == nil && len(c.heldExceptions) != 0 {
			// By the time a call returns normally, every operand stack slot and local that
			// held a reference should have released it
			n := len(c.heldExceptions)
			clear(c.heldExceptions)
			c.thrown, c.execCtx.caughtExceptionParams = nil, nil
			panic(fmt.Sprintf("BUG: %d exnref reference(s) still held when the call returned", n))
		}
		// Drop this call's pins. What a global or table holds stays alive in the store.
		c.thrown = nil
		c.execCtx.caughtExceptionParams = nil
		clear(c.heldExceptions)
	}()

	if ensureTermination {
		done := m.CloseModuleOnCanceledOrTimeout(ctx)
		defer done()
	}

	if c.stackTop&(16-1) != 0 {
		panic("BUG: stack must be aligned to 16 bytes")
	}
	entrypoint(c.preambleExecutable, c.executable, c.execCtxPtr, c.parent.opaquePtr, paramResultPtr, c.stackTop)
	for {
		switch ec := c.execCtx.exitCode; ec & wazevoapi.ExitCodeMask {
		case wazevoapi.ExitCodeOK:
			return nil
		case wazevoapi.ExitCodeGrowStack:
			oldsp := uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall))
			oldTop := c.stackTop
			oldStack := c.stack
			var newsp, newfp uintptr
			if wazevoapi.StackGuardCheckEnabled {
				newsp, newfp, err = c.growStackWithGuarded()
			} else {
				newsp, newfp, err = c.growStack()
			}
			if err != nil {
				return err
			}
			adjustClonedStack(oldsp, oldTop, newsp, newfp, c.stackTop)
			// Old stack must be alive until the new stack is adjusted.
			runtime.KeepAlive(oldStack)
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr, newsp, newfp)
		case wazevoapi.ExitCodeGrowMemory:
			mod := c.callerModuleInstance()
			mem := mod.MemoryInstance
			s := goCallStackView(c.execCtx.stackPointerBeforeGoCall)
			argRes := &s[0]
			if res, ok := mem.Grow(uint32(*argRes)); !ok {
				*argRes = uint64(0xffffffff) // = -1 in signed 32-bit integer.
			} else {
				*argRes = uint64(res)
			}
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr, uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeTableGrow:
			mod := c.callerModuleInstance()
			s := goCallStackView(c.execCtx.stackPointerBeforeGoCall)
			tableIndex, num, ref := uint32(s[0]), uint32(s[1]), uintptr(s[2])
			table := mod.Tables[tableIndex]
			before := uint64(len(table.References))
			s[0] = uint64(uint32(int32(table.Grow(num, ref))))
			if wasm.IsExnref(table.Type) && ref != 0 {
				// Grow wrote the new slots already; record one more naming apiece.
				exn := c.heldException(ref)
				if exn == nil {
					panic(wasmruntime.ErrRuntimeExpiredExceptionRef)
				}
				for i := before; i < uint64(len(table.References)); i++ {
					c.parent.parent.parent.exceptions.Retain(exn)
				}
			}
			if wasm.IsExnref(table.Type) {
				c.releaseIfHeld(ref) // the operand, whether or not the grow succeeded.
			}
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeCallGoFunction:
			index := wazevoapi.GoFunctionIndexFromExitCode(ec)
			f := hostModuleGoFuncFromOpaque[api.GoFunction](index, c.execCtx.goFunctionCallCalleeModuleContextOpaque)
			func() {
				if snapshotEnabled {
					defer snapshotRecoverFn(c)
				}
				f.Call(ctx, goCallStackView(c.execCtx.stackPointerBeforeGoCall))
			}()
			// Back to the native code.
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeCallGoFunctionWithListener:
			index := wazevoapi.GoFunctionIndexFromExitCode(ec)
			f := hostModuleGoFuncFromOpaque[api.GoFunction](index, c.execCtx.goFunctionCallCalleeModuleContextOpaque)
			listeners := hostModuleListenersSliceFromOpaque(c.execCtx.goFunctionCallCalleeModuleContextOpaque)
			s := goCallStackView(c.execCtx.stackPointerBeforeGoCall)
			// Call Listener.Before.
			callerModule := c.callerModuleInstance()
			listener := listeners[index]
			hostModule := hostModuleFromOpaque(c.execCtx.goFunctionCallCalleeModuleContextOpaque)
			def := hostModule.FunctionDefinition(wasm.Index(index))
			listener.Before(ctx, callerModule, def, s, c.stackIterator(true))
			// Call into the Go function.
			func() {
				if snapshotEnabled {
					defer snapshotRecoverFn(c)
				}
				f.Call(ctx, s)
			}()
			// Call Listener.After.
			listener.After(ctx, callerModule, def, s)
			// Back to the native code.
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeCallGoModuleFunction:
			index := wazevoapi.GoFunctionIndexFromExitCode(ec)
			f := hostModuleGoFuncFromOpaque[api.GoModuleFunction](index, c.execCtx.goFunctionCallCalleeModuleContextOpaque)
			mod := c.callerModuleInstance()
			func() {
				if snapshotEnabled {
					defer snapshotRecoverFn(c)
				}
				f.Call(ctx, mod, goCallStackView(c.execCtx.stackPointerBeforeGoCall))
			}()
			// Back to the native code.
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeCallGoModuleFunctionWithListener:
			index := wazevoapi.GoFunctionIndexFromExitCode(ec)
			f := hostModuleGoFuncFromOpaque[api.GoModuleFunction](index, c.execCtx.goFunctionCallCalleeModuleContextOpaque)
			listeners := hostModuleListenersSliceFromOpaque(c.execCtx.goFunctionCallCalleeModuleContextOpaque)
			s := goCallStackView(c.execCtx.stackPointerBeforeGoCall)
			// Call Listener.Before.
			callerModule := c.callerModuleInstance()
			listener := listeners[index]
			hostModule := hostModuleFromOpaque(c.execCtx.goFunctionCallCalleeModuleContextOpaque)
			def := hostModule.FunctionDefinition(wasm.Index(index))
			listener.Before(ctx, callerModule, def, s, c.stackIterator(true))
			// Call into the Go function.
			func() {
				if snapshotEnabled {
					defer snapshotRecoverFn(c)
				}
				f.Call(ctx, callerModule, s)
			}()
			// Call Listener.After.
			listener.After(ctx, callerModule, def, s)
			// Back to the native code.
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeCallListenerBefore:
			stack := goCallStackView(c.execCtx.stackPointerBeforeGoCall)
			index := wasm.Index(stack[0])
			mod := c.callerModuleInstance()
			listener := mod.Engine.(*moduleEngine).listeners[index]
			def := mod.Source.FunctionDefinition(index + mod.Source.ImportFunctionCount)
			listener.Before(ctx, mod, def, stack[1:], c.stackIterator(false))
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeCallListenerAfter:
			stack := goCallStackView(c.execCtx.stackPointerBeforeGoCall)
			index := wasm.Index(stack[0])
			mod := c.callerModuleInstance()
			listener := mod.Engine.(*moduleEngine).listeners[index]
			def := mod.Source.FunctionDefinition(index + mod.Source.ImportFunctionCount)
			listener.After(ctx, mod, def, stack[1:])
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeCheckModuleExitCode:
			// Note: this operation must be done in Go, not native code. The reason is that
			// native code cannot be preempted and that means it can block forever if there are not
			// enough OS threads (which we don't have control over).
			if err := m.FailIfClosed(); err != nil {
				panic(err)
			}
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeRefFunc:
			mod := c.callerModuleInstance()
			s := goCallStackView(c.execCtx.stackPointerBeforeGoCall)
			funcIndex := wasm.Index(s[0])
			ref := mod.Engine.FunctionInstanceReference(funcIndex)
			s[0] = uint64(ref)
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeMemoryWait32:
			mod := c.callerModuleInstance()
			mem := mod.MemoryInstance
			if !mem.Shared {
				panic(wasmruntime.ErrRuntimeExpectedSharedMemory)
			}

			s := goCallStackView(c.execCtx.stackPointerBeforeGoCall)
			timeout, exp, addr := int64(s[0]), uint32(s[1]), uintptr(s[2])
			base := uintptr(unsafe.Pointer(&mem.Buffer[0]))

			offset := uint32(addr - base)
			res := mem.Wait32(offset, exp, timeout, func(mem *wasm.MemoryInstance, offset uint32) uint32 {
				addr := unsafe.Add(unsafe.Pointer(&mem.Buffer[0]), offset)
				return atomic.LoadUint32((*uint32)(addr))
			})
			s[0] = res
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeMemoryWait64:
			mod := c.callerModuleInstance()
			mem := mod.MemoryInstance
			if !mem.Shared {
				panic(wasmruntime.ErrRuntimeExpectedSharedMemory)
			}

			s := goCallStackView(c.execCtx.stackPointerBeforeGoCall)
			timeout, exp, addr := int64(s[0]), uint64(s[1]), uintptr(s[2])
			base := uintptr(unsafe.Pointer(&mem.Buffer[0]))

			offset := uint32(addr - base)
			res := mem.Wait64(offset, exp, timeout, func(mem *wasm.MemoryInstance, offset uint32) uint64 {
				addr := unsafe.Add(unsafe.Pointer(&mem.Buffer[0]), offset)
				return atomic.LoadUint64((*uint64)(addr))
			})
			s[0] = uint64(res)
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeMemoryNotify:
			mod := c.callerModuleInstance()
			mem := mod.MemoryInstance

			s := goCallStackView(c.execCtx.stackPointerBeforeGoCall)
			count, addr := uint32(s[0]), s[1]
			offset := uint32(uintptr(addr) - uintptr(unsafe.Pointer(&mem.Buffer[0])))
			res := mem.Notify(offset, count)
			s[0] = uint64(res)
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeUnreachable:
			panic(wasmruntime.ErrRuntimeUnreachable)
		case wazevoapi.ExitCodeMemoryOutOfBounds:
			panic(wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess)
		case wazevoapi.ExitCodeTableOutOfBounds:
			panic(wasmruntime.ErrRuntimeInvalidTableAccess)
		case wazevoapi.ExitCodeIndirectCallNullPointer:
			panic(wasmruntime.ErrRuntimeInvalidTableAccess)
		case wazevoapi.ExitCodeIndirectCallTypeMismatch:
			panic(wasmruntime.ErrRuntimeIndirectCallTypeMismatch)
		case wazevoapi.ExitCodeIntegerOverflow:
			panic(wasmruntime.ErrRuntimeIntegerOverflow)
		case wazevoapi.ExitCodeIntegerDivisionByZero:
			panic(wasmruntime.ErrRuntimeIntegerDivideByZero)
		case wazevoapi.ExitCodeInvalidConversionToInteger:
			panic(wasmruntime.ErrRuntimeInvalidConversionToInteger)
		case wazevoapi.ExitCodeUnalignedAtomic:
			panic(wasmruntime.ErrRuntimeUnalignedAtomic)
		case wazevoapi.ExitCodeThrowAlloc:
			// Start the raise and return a params buffer sized exactly to the tag, for
			// compiled code to store the params into. No exception object yet: it takes a
			// handler to say whether one is ever needed.
			s := goCallStackView(c.execCtx.stackPointerBeforeGoCall)
			tagIndex := int(s[0])
			mod := c.callerModuleInstance()
			tag := mod.Tags[tagIndex]
			trace := c.liveFrames()
			t := &thrownException{tag: tag, trace: trace}
			if n := len(tag.Type.Params); n > 0 {
				t.params = make([]uint64, n)
			}
			c.thrown = t
			s[0] = uint64(uintptr(unsafe.Pointer(unsafe.SliceData(t.params))))
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeThrow:
			// Throw trampoline: (execCtx, exnref) → ().
			// A throw_ref passes what it is raising, which only this call can resolve; a
			// throw passes zero, its raise having been recorded by the throw-alloc trampoline
			// already.
			s := goCallStackView(c.execCtx.stackPointerBeforeGoCall)
			if handle := wasm.Reference(s[0]); handle != 0 {
				exn := c.heldException(handle)
				if exn == nil {
					// One this call cannot reach, such as a handle host code kept from an
					// earlier call.
					panic(wasmruntime.ErrRuntimeExpiredExceptionRef)
				}
				// This raise starts here, not where the exception was first thrown, which
				// exn.Origin still has.
				c.thrown = &thrownException{
					tag: exn.Tag, params: exn.Params, rethrown: exn, trace: c.liveFrames(),
				}
				// The raise consumes the operand. The exception stays alive through c.thrown,
				// and a handler that catches it takes a fresh reference.
				c.releaseIfHeld(handle)
			}
			// Search for a matching catch clause and rewind to its checkpoint, so compiled
			// code resumes at that try_table's entry with the matched clause index.
			if !c.doHandleException(ctx) {
				// Uncaught, so the raise takes every live frame rather than stopping below
				// a handler. The error the recover path builds is a separate thing from
				// this: a frame is aborted because the exception unwound it, whether or not
				// anything catches it further out.
				c.notifyUnwound(ctx, c.thrown.trace)
				panic(wasmruntime.ErrRuntimeUncaughtException)
			}
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeNullReference:
			panic(wasmruntime.ErrRuntimeNullReference)
		case wazevoapi.ExitCodeTryTableEnter:
			// Save current state as a try handler checkpoint using stack cloning
			// (same approach as experimental.Snapshot).
			// The encoded exit code (with tryTableID in upper bits) is on the
			// Go call stack as the second trampoline argument, not in execCtx.exitCode.
			tryTableEnterStack := goCallStackView(c.execCtx.stackPointerBeforeGoCall)
			tryTableID := wazevoapi.TryTableIDFromExitCode(wazevoapi.ExitCode(tryTableEnterStack[0]))
			mod := c.callerModuleInstance()
			me := mod.Engine.(*moduleEngine)
			info := &me.parent.tryTableInfo[tryTableID]
			returnAddress := c.execCtx.goCallReturnAddress
			oldTop, oldSp := c.stackTop, uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall))
			newSP, newFP, newTop, newStack := c.cloneStack(uintptr(len(c.stack)) + 16)
			adjustClonedStack(oldSp, oldTop, newSP, newFP, newTop)

			// Allocate a heap buffer for locals so handlers can read throw-time values.
			// Nested try_tables in the same function (ReuseLocals) share the enclosing handler's save area.
			var saveArea []uint64
			if info.NumLocals > 0 && !info.ReuseLocals {
				saveArea = make([]uint64, info.NumLocals*2) // 16 bytes per local
				c.execCtx.localsSaveAreaPtr = uintptr(unsafe.Pointer(&saveArea[0]))
			}

			c.tryHandlers = append(c.tryHandlers, tryHandler{
				sp:                newSP,
				fp:                newFP,
				top:               newTop,
				returnAddress:     returnAddress,
				savedRegisters:    c.execCtx.savedRegisters,
				stack:             newStack,
				catchClauses:      info.CatchClauses,
				moduleInstance:    mod,
				localsSaveArea:    saveArea,
				effectiveSaveArea: c.effectiveSaveArea(saveArea),
				exnrefLocals:      info.ExnrefLocals,
				heldExceptions:    c.cloneHeldExceptions(),
				frameDepth:        len(c.liveFrames()),
			})
			// Set clauseIdx = -1 (no exception) in execCtx for the compiled code
			// to read after the trampoline returns.
			c.execCtx.caughtExceptionClauseIdx = -1
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeTryTableLeave:
			// Pop the most recent try handler and restore the locals save
			// area pointer from the handler below (or clear it).
			if len(c.tryHandlers) > 0 {
				c.tryHandlers = c.tryHandlers[:len(c.tryHandlers)-1]
				c.restoreLocalsSaveAreaPtr(len(c.tryHandlers) - 1)
			}
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeExnrefSlotFill:
			// (execCtx, slot address, exnref, count): every slot the fill covers becomes a
			// durable holder of what it now names, and stops being one for what it held.
			s := goCallStackView(c.execCtx.stackPointerBeforeGoCall)
			slots := *(**wasm.Reference)(unsafe.Pointer(&s[0]))
			handle, count := wasm.Reference(s[1]), s[2]
			for i := uint64(0); i < count; i++ {
				c.storeSlotHoldingOperand((*wasm.Reference)(unsafe.Add(unsafe.Pointer(slots), i*8)), handle)
			}
			// One operand, one reference, however many slots it filled.
			c.releaseIfHeld(handle)
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeExnrefSlotCopy:
			// (execCtx, destination address, source address, count), for table.copy and
			// table.init. The source is snapshotted first: the runs may overlap, and each
			// write releases what the destination slot held.
			s := goCallStackView(c.execCtx.stackPointerBeforeGoCall)
			dst := *(**wasm.Reference)(unsafe.Pointer(&s[0]))
			src := *(**wasm.Reference)(unsafe.Pointer(&s[1]))
			count := s[2]
			moved := make([]wasm.Reference, count)
			for i := range moved {
				moved[i] = *(*wasm.Reference)(unsafe.Add(unsafe.Pointer(src), uintptr(i)*8))
			}
			for i := range moved {
				c.parent.parent.parent.exceptions.CopySlot(
					(*wasm.Reference)(unsafe.Add(unsafe.Pointer(dst), uintptr(i)*8)), &moved[i])
			}
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeExnrefSlotLoad:
			// Read barrier: (execCtx, slot address) → exnref. What the slot names becomes
			// reachable from this call, so it is held before compiled code gets the handle.
			s := goCallStackView(c.execCtx.stackPointerBeforeGoCall)
			// Through a double pointer: a uintptr→unsafe.Pointer conversion trips checkptr.
			slot := *(**wasm.Reference)(unsafe.Pointer(&s[0]))
			s[0] = uint64(c.loadExnrefSlot(slot))
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeExnrefSlotStore:
			// Write barrier: (execCtx, slot address, exnref). The slot becomes a durable
			// holder of what it names and stops being one for what it held.
			s := goCallStackView(c.execCtx.stackPointerBeforeGoCall)
			slot := *(**wasm.Reference)(unsafe.Pointer(&s[0]))
			c.storeExnrefSlot(slot, wasm.Reference(s[1]))
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		case wazevoapi.ExitCodeAdjustExnrefs:
			// (execCtx, handle gaining a reference, handle losing one). Compiled code skips
			// the call when both are zero, so at least one is a real handle here.
			s := goCallStackView(c.execCtx.stackPointerBeforeGoCall)
			if inc := wasm.Reference(s[0]); inc != 0 {
				exn := c.heldException(inc)
				if exn == nil {
					// A handle this call cannot reach, such as one host code kept from an
					// earlier call and handed back.
					panic(wasmruntime.ErrRuntimeExpiredExceptionRef)
				}
				c.holdException(exn)
			}
			if dec := wasm.Reference(s[1]); dec != 0 {
				c.releaseException(dec)
			}
			c.execCtx.exitCode = wazevoapi.ExitCodeOK
			afterGoFunctionCallEntrypoint(c.execCtx.goCallReturnAddress, c.execCtxPtr,
				uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall)
		default:
			panic("BUG")
		}
	}
}

// doHandleException finds the handler that catches the raise now in flight and rewinds
// execution to its checkpoint, so compiled code resumes at that try_table's entry with the
// matched clause index. It reports whether one was found; if none was, the exception is
// uncaught and this call is over.
//
// Handlers are searched innermost first, and their clauses in order, which is the order the
// spec gives them.
func (c *callEngine) doHandleException(ctx context.Context) bool {
	t := c.thrown
	for i := len(c.tryHandlers) - 1; i >= 0; i-- {
		h := &c.tryHandlers[i]
		for clauseIdx := range h.catchClauses {
			clause := &h.catchClauses[clauseIdx]
			// The clause's tag index is in the tag space of the module that set the handler
			// up, not of the one that threw.
			matched := false
			switch clause.Kind {
			case wasm.CatchKindCatch, wasm.CatchKindCatchRef:
				matched = h.moduleInstance.Tags[clause.TagIndex] == t.tag
			case wasm.CatchKindCatchAll, wasm.CatchKindCatchAllRef:
				matched = true
			}
			if !matched {
				continue
			}

			// Build the exception if this clause hands anything to guest code, since what it
			// hands over -- params to read, a handle to throw again -- outlives the raise.
			// This has to happen before the reference table is rewound: the exnref params of
			// a raise are resolved through what the raising frames held, which the rewind
			// undoes.
			var exn *wasm.Exception
			switch clause.Kind {
			case wasm.CatchKindCatch:
				if len(t.params) > 0 {
					exn = c.exceptionOf(t)
				}
			case wasm.CatchKindCatchRef, wasm.CatchKindCatchAllRef:
				exn = c.exceptionOf(t)
			case wasm.CatchKindCatchAll:
				// Hands over nothing, so nothing needs to outlive the raise.
			}

			// Everything above the frame this handler belongs to leaves the stack without
			// returning, which is what Abort reports.
			if unwound := len(t.trace) - h.frameDepth; unwound > 0 {
				c.notifyUnwound(ctx, t.trace[:unwound])
			}

			// Restore localsSaveAreaPtr from the matched handler or
			// the nearest enclosing one (same-function reuse).
			c.restoreLocalsSaveAreaPtr(i)

			// Pop all handlers at and above this one.
			c.tryHandlers = c.tryHandlers[:i]

			// The rewind puts the reference counts back the way they were at this
			// try_table's entry, which is what releases everything the raise discarded.
			previous := c.heldExceptions
			c.heldExceptions, h.heldExceptions = h.heldExceptions, nil
			c.adoptSaveAreaExnrefs(h, previous)

			// Restore the cloned stack (like snapshot.doRestore).
			spp := *(**uint64)(unsafe.Pointer(&h.sp))
			c.stack = h.stack
			c.stackTop = h.top
			ec := &c.execCtx
			ec.stackBottomPtr = &c.stack[0]
			ec.stackPointerBeforeGoCall = spp
			ec.framePointerBeforeGoCall = h.fp
			ec.goCallReturnAddress = h.returnAddress
			ec.savedRegisters = h.savedRegisters

			// Then take the references for what this clause is about to hand over, which
			// the frame this handler belongs to is the one holding from here on.
			switch clause.Kind {
			case wasm.CatchKindCatch:
				if exn != nil {
					c.execCtx.caughtExceptionParams = unsafe.SliceData(exn.Params)
					c.holdParamRefs(exn)
				}
			case wasm.CatchKindCatchRef:
				c.execCtx.caughtExceptionParams = unsafe.SliceData(exn.Params)
				c.holdParamRefs(exn)
				c.holdException(exn)
				c.execCtx.caughtExceptionRef = uintptr(exn.ID)
			case wasm.CatchKindCatchAllRef:
				c.holdException(exn)
				c.execCtx.caughtExceptionRef = uintptr(exn.ID)
			}

			// The raise is over -- whatever outlives it belongs to the exception by now --
			// and a later throw_ref is a new one with its own stack.
			c.thrown = nil
			c.execCtx.caughtExceptionClauseIdx = int64(clauseIdx)
			return true
		}
	}
	return false
}

// restoreLocalsSaveAreaPtr walks tryHandlers from index `from` downward
// and sets localsSaveAreaPtr to the first handler that owns a save area,
// or clears it if none is found.
func (c *callEngine) restoreLocalsSaveAreaPtr(from int) {
	for i := from; i >= 0; i-- {
		if sa := c.tryHandlers[i].localsSaveArea; len(sa) > 0 {
			c.execCtx.localsSaveAreaPtr = uintptr(unsafe.Pointer(&sa[0]))
			return
		}
	}
	c.execCtx.localsSaveAreaPtr = 0
}

func (c *callEngine) callerModuleInstance() *wasm.ModuleInstance {
	return moduleInstanceFromOpaquePtr(c.execCtx.callerModuleContextPtr)
}

// effectiveSaveArea is the buffer a try_table's handlers read the locals out of: its own
// when it allocated one, and otherwise the enclosing try_table's, which a nested one in the
// same function shares.
func (c *callEngine) effectiveSaveArea(own []uint64) []uint64 {
	if len(own) > 0 {
		return own
	}
	for i := len(c.tryHandlers) - 1; i >= 0; i-- {
		if sa := c.tryHandlers[i].localsSaveArea; len(sa) > 0 {
			return sa
		}
	}
	return nil
}

// adoptSaveAreaExnrefs is half of what a catch does to the reference count of an exnref
// local. The local ends up holding what it held at the throw, which the save area carries
// across the rewind -- but the rewind put the count back to what it was at the try_table's
// entry, which is a count of what the local held *there*. So the two differ by a swap: out
// with the value the rewind restored, in with the one the save area names.
//
// This is the in. It has to be here because `previous`, the table as it was at the raise, is
// the last place a reference to the save area's handle still exists. The out is emitted into
// the handler by the compiler (see reloadLocalsFromSaveArea), which is the only place the
// value the rewind restored is known. Neither half is conditional on the two being different
// -- when the local did not change they are the same handle, and the pair cancels.
func (c *callEngine) adoptSaveAreaExnrefs(h *tryHandler, previous map[wasm.Reference]*exceptionRefs) {
	for _, localIdx := range h.exnrefLocals {
		handle := wasm.Reference(h.effectiveSaveArea[localIdx*2])
		if handle == 0 {
			continue // `ref.null exn`, which is not counted.
		}
		if e := previous[handle]; e != nil {
			c.holdException(e.exn)
		}
	}
}

// notifyUnwound tells the listener of each of the given frames that an exception took it off
// the stack.
func (c *callEngine) notifyUnwound(ctx context.Context, unwound []uintptr) {
	for _, addr := range unwound {
		c.abortListenerOf(ctx, addr)
	}
}

// liveFrames is the address of every live wasm frame, innermost first, as of an exit to Go
// through one of the shared trampolines. Each address is inside the function whose frame it
// stands for, which is what identifies it.
//
// The trampoline has a frame of its own, so the first return address unwinding reports is
// already the innermost wasm frame; the last is the entry trampoline rather than a wasm
// frame, so it is dropped.
func (c *callEngine) liveFrames() []uintptr {
	retAddrs := unwindStack(
		uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)),
		c.execCtx.framePointerBeforeGoCall,
		c.stackTop,
		nil,
		0, // Every frame: a listener has to be told about each one the exception unwinds.
	)
	if len(retAddrs) == 0 {
		return nil
	}
	return retAddrs[:len(retAddrs)-1]
}

const callStackCeiling = uintptr(50000000) // in uint64 (8 bytes) == 400000000 bytes in total == 400mb.

func (c *callEngine) growStackWithGuarded() (newSP uintptr, newFP uintptr, err error) {
	if wazevoapi.StackGuardCheckEnabled {
		wazevoapi.CheckStackGuardPage(c.stack)
	}
	newSP, newFP, err = c.growStack()
	if err != nil {
		return
	}
	if wazevoapi.StackGuardCheckEnabled {
		c.execCtx.stackBottomPtr = &c.stack[wazevoapi.StackGuardCheckGuardPageSize]
	}
	return
}

// growStack grows the stack, and returns the new stack pointer.
func (c *callEngine) growStack() (newSP, newFP uintptr, err error) {
	currentLen := uintptr(len(c.stack))
	if callStackCeiling < currentLen {
		err = wasmruntime.ErrRuntimeStackOverflow
		return
	}

	newLen := 2*currentLen + c.execCtx.stackGrowRequiredSize + 16 // Stack might be aligned to 16 bytes, so add 16 bytes just in case.
	newSP, newFP, c.stackTop, c.stack = c.cloneStack(newLen)
	c.execCtx.stackBottomPtr = &c.stack[0]
	return
}

func (c *callEngine) cloneStack(l uintptr) (newSP, newFP, newTop uintptr, newStack []byte) {
	newStack = make([]byte, l)

	relSp := c.stackTop - uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall))
	relFp := c.stackTop - c.execCtx.framePointerBeforeGoCall

	// Copy the existing contents in the previous Go-allocated stack into the new one.
	var prevStackAligned, newStackAligned []byte
	{
		//nolint:staticcheck
		sh := (*reflect.SliceHeader)(unsafe.Pointer(&prevStackAligned))
		sh.Data = c.stackTop - relSp
		sh.Len = int(relSp)
		sh.Cap = int(relSp)
	}
	newTop = alignedStackTop(newStack)
	{
		newSP = newTop - relSp
		newFP = newTop - relFp
		//nolint:staticcheck
		sh := (*reflect.SliceHeader)(unsafe.Pointer(&newStackAligned))
		sh.Data = newSP
		sh.Len = int(relSp)
		sh.Cap = int(relSp)
	}
	copy(newStackAligned, prevStackAligned)
	return
}

func (c *callEngine) stackIterator(onHostCall bool) experimental.StackIterator {
	c.stackIteratorImpl.reset(c, onHostCall)
	return &c.stackIteratorImpl
}

// stackIterator implements experimental.StackIterator.
type stackIterator struct {
	retAddrs      []uintptr
	retAddrCursor int
	eng           *engine
	pc            uint64

	currentDef *wasm.FunctionDefinition
}

func (si *stackIterator) reset(c *callEngine, onHostCall bool) {
	if onHostCall {
		si.retAddrs = append(si.retAddrs[:0], uintptr(unsafe.Pointer(c.execCtx.goCallReturnAddress)))
	} else {
		si.retAddrs = si.retAddrs[:0]
	}
	si.retAddrs = unwindStack(uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall)), c.execCtx.framePointerBeforeGoCall, c.stackTop, si.retAddrs, wasmdebug.MaxFrames)
	si.retAddrs = si.retAddrs[:len(si.retAddrs)-1] // the last return addr is the trampoline, so we skip it.
	si.retAddrCursor = 0
	si.eng = c.parent.parent.parent
}

// Next implements the same method as documented on experimental.StackIterator.
func (si *stackIterator) Next() bool {
	if si.retAddrCursor >= len(si.retAddrs) {
		return false
	}

	addr := si.retAddrs[si.retAddrCursor]
	cm := si.eng.compiledModuleOfAddr(addr)
	if cm != nil {
		index := cm.functionIndexOf(addr)
		def := cm.module.FunctionDefinition(cm.module.ImportFunctionCount + index)
		si.currentDef = def
		si.retAddrCursor++
		si.pc = uint64(addr)
		return true
	}
	return false
}

// ProgramCounter implements the same method as documented on experimental.StackIterator.
func (si *stackIterator) ProgramCounter() experimental.ProgramCounter {
	return experimental.ProgramCounter(si.pc)
}

// Function implements the same method as documented on experimental.StackIterator.
func (si *stackIterator) Function() experimental.InternalFunction {
	return si
}

// Definition implements the same method as documented on experimental.InternalFunction.
func (si *stackIterator) Definition() api.FunctionDefinition {
	return si.currentDef
}

// SourceOffsetForPC implements the same method as documented on experimental.InternalFunction.
func (si *stackIterator) SourceOffsetForPC(pc experimental.ProgramCounter) uint64 {
	upc := uintptr(pc)
	cm := si.eng.compiledModuleOfAddr(upc)
	return cm.getSourceOffset(upc)
}

// snapshot implements experimental.Snapshot
type snapshot struct {
	sp, fp, top    uintptr
	returnAddress  *byte
	stack          []byte
	savedRegisters [64][2]uint64
	ret            []uint64
	c              *callEngine
	// heldExceptions and thrown are the call's exception state at the snapshot. A restore
	// rewinds the stack the references live in, so they have to be rewound with it: the
	// locals and operand slots that took them are gone, and the compiled releases that would
	// have balanced them are on paths that no longer run.
	heldExceptions map[wasm.Reference]*exceptionRefs
	thrown         *thrownException
}

// Snapshot implements the same method as documented on experimental.Snapshotter.
func (c *callEngine) Snapshot() experimental.Snapshot {
	returnAddress := c.execCtx.goCallReturnAddress
	oldTop, oldSp := c.stackTop, uintptr(unsafe.Pointer(c.execCtx.stackPointerBeforeGoCall))
	newSP, newFP, newTop, newStack := c.cloneStack(uintptr(len(c.stack)) + 16)
	adjustClonedStack(oldSp, oldTop, newSP, newFP, newTop)
	return &snapshot{
		sp:             newSP,
		fp:             newFP,
		top:            newTop,
		savedRegisters: c.execCtx.savedRegisters,
		returnAddress:  returnAddress,
		stack:          newStack,
		c:              c,
		heldExceptions: c.cloneHeldExceptions(),
		thrown:         c.thrown,
	}
}

// cloneHeldExceptions copies the reference table for a snapshot. The counts are reached
// through a pointer, so the entries are copied too and not just the map around them.
func (c *callEngine) cloneHeldExceptions() map[wasm.Reference]*exceptionRefs {
	if len(c.heldExceptions) == 0 {
		return nil
	}
	out := make(map[wasm.Reference]*exceptionRefs, len(c.heldExceptions))
	for handle, e := range c.heldExceptions {
		out[handle] = &exceptionRefs{exn: e.exn, count: e.count}
	}
	return out
}

// Restore implements the same method as documented on experimental.Snapshot.
func (s *snapshot) Restore(ret []uint64) {
	s.ret = ret
	panic(s)
}

func (s *snapshot) doRestore() {
	spp := *(**uint64)(unsafe.Pointer(&s.sp))
	view := goCallStackView(spp)
	copy(view, s.ret)

	c := s.c
	c.stack = s.stack
	c.stackTop = s.top
	c.heldExceptions = s.heldExceptions
	c.thrown = s.thrown
	ec := &c.execCtx
	ec.stackBottomPtr = &c.stack[0]
	ec.stackPointerBeforeGoCall = spp
	ec.framePointerBeforeGoCall = s.fp
	ec.goCallReturnAddress = s.returnAddress
	ec.savedRegisters = s.savedRegisters
}

// Error implements the same method on error.
func (s *snapshot) Error() string {
	return "unhandled snapshot restore, this generally indicates restore was called from a different " +
		"exported function invocation than snapshot"
}

func snapshotRecoverFn(c *callEngine) {
	if r := recover(); r != nil {
		if s, ok := r.(*snapshot); ok && s.c == c {
			s.doRestore()
		} else {
			panic(r)
		}
	}
}
