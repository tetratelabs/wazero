package wazevoapi

// ExitCode is an exit code of an execution of a function.
type ExitCode uint32

const (
	ExitCodeOK ExitCode = iota
	ExitCodeGrowStack
	ExitCodeGrowMemory
	ExitCodeUnreachable
	ExitCodeMemoryOutOfBounds
	// ExitCodeCallGoModuleFunction is an exit code for a call to an api.GoModuleFunction.
	ExitCodeCallGoModuleFunction
	// ExitCodeCallGoFunction is an exit code for a call to an api.GoFunction.
	ExitCodeCallGoFunction
	ExitCodeTableOutOfBounds
	ExitCodeIndirectCallNullPointer
	ExitCodeIndirectCallTypeMismatch
	ExitCodeIntegerDivisionByZero
	ExitCodeIntegerOverflow
	ExitCodeInvalidConversionToInteger
	ExitCodeCheckModuleExitCode
	ExitCodeCallListenerBefore
	ExitCodeCallListenerAfter
	ExitCodeCallGoModuleFunctionWithListener
	ExitCodeCallGoFunctionWithListener
	ExitCodeTableGrow
	ExitCodeRefFunc
	ExitCodeMemoryWait32
	ExitCodeMemoryWait64
	ExitCodeMemoryNotify
	ExitCodeUnalignedAtomic
	// ExitCodeThrowAlloc is an exit code for starting a throw: it records the raise and
	// returns a params buffer sized to the tag, for compiled code to store the params into.
	ExitCodeThrowAlloc
	// ExitCodeThrow is the shared throw/throw_ref exit code.
	// The exnref is passed on the stack, and is zero for a throw, whose exception
	// the throw-alloc exit has already recorded. The handler searches for a
	// matching catch clause and restores the stack checkpoint.
	ExitCodeThrow
	// ExitCodeNullReference is an exit code for a null reference trap (throw_ref with null exnref).
	ExitCodeNullReference
	// ExitCodeTryTableEnter is an exit code for entering a try_table block.
	// The catch clause info is encoded in the upper bits. The dispatch loop
	// saves the current SP/FP/returnAddress as a try handler checkpoint.
	ExitCodeTryTableEnter
	// ExitCodeTryTableLeave is an exit code for leaving a try_table block.
	// The dispatch loop pops the most recent try handler.
	ExitCodeTryTableLeave
	// ExitCodeExnrefSlotFill is an exit code for the write barrier over a run of
	// exnref-typed table slots, which table.fill writes.
	ExitCodeExnrefSlotFill
	// ExitCodeExnrefSlotCopy is an exit code for the write barrier over a run of
	// exnref-typed table slots copied from elsewhere, which table.copy and table.init write.
	ExitCodeExnrefSlotCopy
	// ExitCodeExnrefSlotLoad is an exit code for the read barrier on an exnref-typed global
	// or table slot: what the slot names becomes reachable from this call, so the runtime
	// has to pin it before compiled code can hold the handle.
	ExitCodeExnrefSlotLoad
	// ExitCodeExnrefSlotStore is an exit code for the write barrier on an exnref-typed
	// global or table slot: the slot becomes a durable holder of what it now names, and
	// stops being one for what it held.
	ExitCodeExnrefSlotStore
	// ExitCodeAdjustExnrefs is an exit code for adjusting the call's exnref reference counts:
	// one handle gains a reference, another loses one. Either may be zero, meaning nothing.
	// One exit covers all of it because the instructions that move an exnref between a local
	// and the operand stack do both at once.
	ExitCodeAdjustExnrefs
	exitCodeMax
)

const ExitCodeMask = 0xff

// String implements fmt.Stringer.
func (e ExitCode) String() string {
	switch e {
	case ExitCodeOK:
		return "ok"
	case ExitCodeGrowStack:
		return "grow_stack"
	case ExitCodeCallGoModuleFunction:
		return "call_go_module_function"
	case ExitCodeCallGoFunction:
		return "call_go_function"
	case ExitCodeUnreachable:
		return "unreachable"
	case ExitCodeMemoryOutOfBounds:
		return "memory_out_of_bounds"
	case ExitCodeUnalignedAtomic:
		return "unaligned_atomic"
	case ExitCodeTableOutOfBounds:
		return "table_out_of_bounds"
	case ExitCodeIndirectCallNullPointer:
		return "indirect_call_null_pointer"
	case ExitCodeIndirectCallTypeMismatch:
		return "indirect_call_type_mismatch"
	case ExitCodeIntegerDivisionByZero:
		return "integer_division_by_zero"
	case ExitCodeIntegerOverflow:
		return "integer_overflow"
	case ExitCodeInvalidConversionToInteger:
		return "invalid_conversion_to_integer"
	case ExitCodeCheckModuleExitCode:
		return "check_module_exit_code"
	case ExitCodeCallListenerBefore:
		return "call_listener_before"
	case ExitCodeCallListenerAfter:
		return "call_listener_after"
	case ExitCodeCallGoModuleFunctionWithListener:
		return "call_go_module_function_with_listener"
	case ExitCodeCallGoFunctionWithListener:
		return "call_go_function_with_listener"
	case ExitCodeGrowMemory:
		return "grow_memory"
	case ExitCodeTableGrow:
		return "table_grow"
	case ExitCodeRefFunc:
		return "ref_func"
	case ExitCodeMemoryWait32:
		return "memory_wait32"
	case ExitCodeMemoryWait64:
		return "memory_wait64"
	case ExitCodeExnrefSlotFill:
		return "exnref_slot_fill"
	case ExitCodeExnrefSlotCopy:
		return "exnref_slot_copy"
	case ExitCodeExnrefSlotLoad:
		return "exnref_slot_load"
	case ExitCodeExnrefSlotStore:
		return "exnref_slot_store"
	case ExitCodeAdjustExnrefs:
		return "adjust_exnrefs"
	case ExitCodeMemoryNotify:
		return "memory_notify"
	case ExitCodeThrowAlloc:
		return "throw_alloc"
	case ExitCodeNullReference:
		return "null_reference"
	case ExitCodeThrow:
		return "throw"
	case ExitCodeTryTableEnter:
		return "try_table_enter"
	case ExitCodeTryTableLeave:
		return "try_table_leave"
	}
	panic("TODO")
}

func ExitCodeCallGoModuleFunctionWithIndex(index int, withListener bool) ExitCode {
	if withListener {
		return ExitCodeCallGoModuleFunctionWithListener | ExitCode(index<<8)
	}
	return ExitCodeCallGoModuleFunction | ExitCode(index<<8)
}

func ExitCodeCallGoFunctionWithIndex(index int, withListener bool) ExitCode {
	if withListener {
		return ExitCodeCallGoFunctionWithListener | ExitCode(index<<8)
	}
	return ExitCodeCallGoFunction | ExitCode(index<<8)
}

func GoFunctionIndexFromExitCode(exitCode ExitCode) int {
	return int(exitCode >> 8)
}

// TryTableIDFromExitCode extracts the try-table ID from an ExitCodeTryTableEnter
// exit code. Uses the same encoding as GoFunctionIndexFromExitCode (upper 24 bits).
func TryTableIDFromExitCode(exitCode ExitCode) int {
	return GoFunctionIndexFromExitCode(exitCode)
}
