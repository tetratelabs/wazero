package wazeroir

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/buildoptions"
	"github.com/tetratelabs/wazero/wasm/leb128"
)

type controlFrameKind byte

const (
	controlFrameKindBlockWithContinuationLabel controlFrameKind = iota
	controlFrameKindBlockWithoutContinuationLabel
	controlFrameKindFunction
	controlFrameKindLoop
	controlFrameKindIfWithElse
	controlFrameKindIfWithoutElse
)

type (
	controlFrame struct {
		frameID          uint32
		originalStackLen int
		returns          []SignLessType
		kind             controlFrameKind
	}
	controlFrames struct{ frames []*controlFrame }
)

func (c *controlFrame) ensureContinuation() {
	// Make sure that if the frame is block and doesn't have continuation,
	// change the kind so we can emit the continuation block
	// later when we reach the end instruction of this frame.
	if c.kind == controlFrameKindBlockWithoutContinuationLabel {
		c.kind = controlFrameKindBlockWithContinuationLabel
	}
}

func (c *controlFrame) asBranchTarget() *BranchTarget {
	switch c.kind {
	case controlFrameKindBlockWithContinuationLabel,
		controlFrameKindBlockWithoutContinuationLabel:
		return &BranchTarget{Label: &Label{FrameID: c.frameID, Kind: LabelKindContinuation}}
	case controlFrameKindLoop:
		return &BranchTarget{Label: &Label{FrameID: c.frameID, Kind: LabelKindHeader}}
	case controlFrameKindFunction:
		// Note nil target is translated as return.
		return &BranchTarget{Label: nil}
	case controlFrameKindIfWithElse,
		controlFrameKindIfWithoutElse:
		return &BranchTarget{Label: &Label{FrameID: c.frameID, Kind: LabelKindContinuation}}
	}
	panic(fmt.Sprintf("unreachable: a bug in wazeroir implementation: %v", c.kind))
}

func (c *controlFrames) functionFrame() *controlFrame {
	// No need to check stack bound
	// as we can assume that all the operations
	// are valid thanks to analyzeFunction
	// at module validation phase.
	return c.frames[0]
}

func (c *controlFrames) get(n int) *controlFrame {
	// No need to check stack bound
	// as we can assume that all the operations
	// are valid thanks to analyzeFunction
	// at module validation phase.
	return c.frames[len(c.frames)-n-1]
}

func (c *controlFrames) top() *controlFrame {
	// No need to check stack bound
	// as we can assume that all the operations
	// are valid thanks to analyzeFunction
	// at module validation phase.
	return c.frames[len(c.frames)-1]
}

func (c *controlFrames) empty() bool {
	return len(c.frames) == 0
}

func (c *controlFrames) pop() (frame *controlFrame) {
	// No need to check stack bound
	// as we can assume that all the operations
	// are valid thanks to analyzeFunction
	// at module validation phase.
	frame = c.top()
	c.frames = c.frames[:len(c.frames)-1]
	return
}

func (c *controlFrames) push(frame *controlFrame) {
	c.frames = append(c.frames, frame)
}

type compiler struct {
	stack            []SignLessType
	currentID        uint32
	controlFrames    *controlFrames
	unreachableState struct {
		on    bool
		depth int
	}
	pc     uint64
	f      *wasm.FunctionInstance
	result []Operation
}

// For debugging only.
func (c *compiler) stackDump() string {
	strs := make([]string, 0, len(c.stack))
	for _, s := range c.stack {
		strs = append(strs, s.String())
	}
	return "[" + strings.Join(strs, ", ") + "]"
}

// Compile lowers given function instance into wazeroir operations
// so that the resulting operations can be consumed by the interpreter
// or the JIT compilation engine.
func Compile(f *wasm.FunctionInstance) ([]Operation, error) {
	c := compiler{controlFrames: &controlFrames{}, f: f}

	// Push function arguments.
	for _, t := range f.Signature.InputTypes {
		c.stackPush(wasmValueTypeToSignless(t))
	}
	// Emit const expressions for locals.
	// Note that here we don't take function arguments
	// into account, meaning that callers must push
	// arguments before entering into the function body.
	for _, t := range f.LocalTypes {
		c.emitDefaultValue(t)
	}

	// Insert the function control frame.
	returns := make([]SignLessType, 0, len(f.Signature.ReturnTypes))
	for _, t := range f.Signature.ReturnTypes {
		returns = append(returns, wasmValueTypeToSignless(t))
	}
	c.controlFrames.push(&controlFrame{
		frameID:          c.nextID(),
		originalStackLen: len(f.Signature.InputTypes),
		returns:          returns,
		kind:             controlFrameKindFunction,
	})

	// Now enter the function body.
	for !c.controlFrames.empty() {
		if err := c.handleInstruction(); err != nil {
			return nil, fmt.Errorf("handling instruction: %w\ndisassemble: %v", err, Disassemble(c.result))
		}
	}
	return c.result, nil
}

// Translate the current Wasm instruction to wazeroir's operations,
// and emit the results into c.results.
func (c *compiler) handleInstruction() error {
	op := c.f.Body[c.pc]
	if buildoptions.IsDebugMode {
		fmt.Printf("handling %s, unreachable_state(on=%v,depth=%d)\n",
			buildoptions.OptcodeStrs[op],
			c.unreachableState.on, c.unreachableState.depth,
		)
	}

	// Modify the stack according the current instruction.
	// Note that some instructions will read "index" in
	// applyToStack and advance c.pc inside the function.
	index, err := c.applyToStack(op)
	if err != nil {
		return fmt.Errorf("apply stack failed for 0x%x: %w: stack: %s", op, err, c.stackDump())
	}

	// Now we handle each instruction, and
	// emit the corresponding wazeroir operations to the results.
operatorSwitch:
	switch op {
	case wasm.OptCodeUnreachable:
		c.emit(
			&OperationUnreachable{},
		)
		c.unreachableState.on = true
	case wasm.OptCodeNop:
		// Nop is noop!
	case wasm.OptCodeBlock:
		bt, num, err := wasm.ReadBlockType(c.f.ModuleInstance.Types,
			bytes.NewBuffer(c.f.Body[c.pc+1:]))
		if err != nil {
			return fmt.Errorf("reading block type for block instruction: %w", err)
		}
		c.pc += num

		if c.unreachableState.on {
			// If it is currently in unreachable,
			// just remove the entire block.
			c.unreachableState.depth++
			break operatorSwitch
		}

		// Create a new frame -- entering this block.
		frame := &controlFrame{
			frameID:          c.nextID(),
			originalStackLen: len(c.stack),
			kind:             controlFrameKindBlockWithoutContinuationLabel,
		}
		for _, t := range bt.ReturnTypes {
			frame.returns = append(frame.returns, wasmValueTypeToSignless(t))
		}
		c.controlFrames.push(frame)

	case wasm.OptCodeLoop:
		bt, num, err := wasm.ReadBlockType(c.f.ModuleInstance.Types,
			bytes.NewBuffer(c.f.Body[c.pc+1:]))
		if err != nil {
			return fmt.Errorf("reading block type for loop instruction: %w", err)
		}
		c.pc += num

		if c.unreachableState.on {
			// If it is currently in unreachable,
			// just remove the entire block.
			c.unreachableState.depth++
			break operatorSwitch
		}

		// Create a new frame -- entering loop.
		frame := &controlFrame{
			frameID:          c.nextID(),
			originalStackLen: len(c.stack),
			kind:             controlFrameKindLoop,
		}
		for _, t := range bt.ReturnTypes {
			frame.returns = append(frame.returns, wasmValueTypeToSignless(t))
		}
		c.controlFrames.push(frame)

		// Prep labels for inside and the continuation of this loop.
		loopLabel := &Label{FrameID: frame.frameID, Kind: LabelKindHeader}

		// Emit the branch operation to enter inside the loop.
		c.emit(
			&OperationBr{
				Target: loopLabel.asBranchTarget(),
			},
			&OperationLabel{Label: loopLabel},
		)

	case wasm.OptCodeIf:
		bt, num, err := wasm.ReadBlockType(c.f.ModuleInstance.Types,
			bytes.NewBuffer(c.f.Body[c.pc+1:]))
		if err != nil {
			return fmt.Errorf("reading block type for if instruction: %w", err)
		}
		c.pc += num

		if c.unreachableState.on {
			// If it is currently in unreachable,
			// just remove the entire block.
			c.unreachableState.depth++
			break operatorSwitch
		}

		// Create a new frame -- entering if.
		frame := &controlFrame{
			frameID:          c.nextID(),
			originalStackLen: len(c.stack),
			// Note this will be set to controlFrameKindIfWithElse
			// when else optcode found later.
			kind: controlFrameKindIfWithoutElse,
		}
		for _, t := range bt.ReturnTypes {
			frame.returns = append(frame.returns, wasmValueTypeToSignless(t))
		}
		c.controlFrames.push(frame)

		// Prep labels for if and else of this if.
		thenLabel := &Label{Kind: LabelKindHeader, FrameID: frame.frameID}
		elseLabel := &Label{Kind: LabelKindElse, FrameID: frame.frameID}

		// Emit the branch operation to enter the then block.
		c.emit(
			&OperationBrIf{
				Then: thenLabel.asBranchTargetDrop(),
				Else: elseLabel.asBranchTargetDrop(),
			},
			&OperationLabel{
				Label: thenLabel,
			},
		)
	case wasm.OptCodeElse:
		frame := c.controlFrames.top()
		if c.unreachableState.on && c.unreachableState.depth > 0 {
			// If it is currently in unreachable, and the nested if,
			// just remove the entire else block.
			break operatorSwitch
		} else if c.unreachableState.on {
			// If it is currently in unreachable, and the non-nested if,
			// reset the stack so we can correctly handle the else block.
			top := c.controlFrames.top()
			c.stack = c.stack[:top.originalStackLen]
			top.kind = controlFrameKindIfWithElse

			// We are no longer unreachable in else frame,
			// so emit the correct label, and reset the unreachable state.
			elseLabel := &Label{FrameID: frame.frameID, Kind: LabelKindElse}
			c.unreachableState.on = false
			c.emit(
				&OperationLabel{Label: elseLabel},
			)
			break operatorSwitch
		}

		// Change the kind of this If block, indicating that
		// the if has else block.
		frame.kind = controlFrameKindIfWithElse

		// We need to reset the stack so that
		// the values pushed inside the then block
		// do not affect the else block.
		dropOp := &OperationDrop{Range: c.getFrameDropRange(frame)}
		c.stack = c.stack[:frame.originalStackLen]

		// Prep labels for else and the continueation of this if block.
		elseLabel := &Label{FrameID: frame.frameID, Kind: LabelKindElse}
		continuationLabel := &Label{FrameID: frame.frameID, Kind: LabelKindContinuation}

		// Emit the instructions for exiting the if loop,
		// and then the initiation of else block.
		c.emit(
			dropOp,
			// Jump to the continuation of this block.
			&OperationBr{Target: continuationLabel.asBranchTarget()},
			// Initiate the else block.
			&OperationLabel{Label: elseLabel},
		)
	case wasm.OptCodeEnd:
		if c.unreachableState.on && c.unreachableState.depth > 0 {
			c.unreachableState.depth--
			break operatorSwitch
		} else if c.unreachableState.on {
			c.unreachableState.on = false

			frame := c.controlFrames.pop()
			if c.controlFrames.empty() {
				return nil
			}

			c.stack = c.stack[:frame.originalStackLen]
			for _, t := range frame.returns {
				c.stackPush(t)
			}

			continuationLabel := &Label{FrameID: frame.frameID, Kind: LabelKindContinuation}
			if frame.kind == controlFrameKindIfWithoutElse {
				// Emit the else label.
				elseLabel := &Label{Kind: LabelKindElse, FrameID: frame.frameID}
				c.emit(
					&OperationLabel{Label: elseLabel},
					&OperationBr{Target: continuationLabel.asBranchTarget()},
					&OperationLabel{Label: continuationLabel},
				)
			} else {
				c.emit(
					&OperationLabel{Label: continuationLabel},
				)
			}

			break operatorSwitch
		}

		frame := c.controlFrames.pop()

		// We need to reset the stack so that
		// the values pushed inside the block.
		dropOp := &OperationDrop{Range: c.getFrameDropRange(frame)}
		c.stack = c.stack[:frame.originalStackLen]

		// Push the result types onto the stack.
		for _, t := range frame.returns {
			c.stackPush(t)
		}

		// Emit the instructions according to the kind of the current control frame.
		switch frame.kind {
		case controlFrameKindFunction:
			if !c.controlFrames.empty() {
				// Should never happen. If so, there's a bug in the translation.
				return fmt.Errorf("invalid function frame")
			}
			// Return from function.
			c.emit(
				dropOp,
				// Pass empty target instead of nil to avoid misinterpretation as "return"
				&OperationBr{Target: &BranchTarget{}},
			)
		case controlFrameKindIfWithoutElse:
			// This case we have to emit "empty" else label.
			elseLabel := &Label{Kind: LabelKindElse, FrameID: frame.frameID}
			continuationLabel := &Label{Kind: LabelKindContinuation, FrameID: frame.frameID}
			c.emit(
				dropOp,
				&OperationBr{Target: continuationLabel.asBranchTarget()},
				// Emit the else which soon branches into the continuation.
				&OperationLabel{Label: elseLabel},
				&OperationBr{Target: continuationLabel.asBranchTarget()},
				// Initiate the continuation.
				&OperationLabel{Label: continuationLabel},
			)
		case controlFrameKindBlockWithContinuationLabel,
			controlFrameKindIfWithElse:
			continuationLabel := &Label{Kind: LabelKindContinuation, FrameID: frame.frameID}
			c.emit(
				dropOp,
				&OperationBr{Target: continuationLabel.asBranchTarget()},
				&OperationLabel{Label: continuationLabel},
			)
		case controlFrameKindLoop, controlFrameKindBlockWithoutContinuationLabel:
			c.emit(
				dropOp,
			)
		default:
			// Should never happen. If so, there's a bug in the translation.
			return fmt.Errorf("invalid control frame kind: 0x%x", frame.kind)
		}

	case wasm.OptCodeBr:
		target, n, err := leb128.DecodeUint32(bytes.NewBuffer(c.f.Body[c.pc+1:]))
		if err != nil {
			return fmt.Errorf("read the target for br_if: %w", err)
		}
		c.pc += n

		targetFrame := c.controlFrames.get(int(target))
		targetFrame.ensureContinuation()
		dropOp := &OperationDrop{Range: c.getFrameDropRange(targetFrame)}

		c.emit(
			dropOp,
			&OperationBr{Target: targetFrame.asBranchTarget()},
		)
		// Br operation is stack-polymorphic, and mark the state as unreachable.
		// That means subsequent instructions in the current control frame are "unreachable"
		// and can be safely removed.
		c.unreachableState.on = true
	case wasm.OptCodeBrIf:
		target, n, err := leb128.DecodeUint32(bytes.NewBuffer(c.f.Body[c.pc+1:]))
		if err != nil {
			return fmt.Errorf("read the target for br_if: %w", err)
		}
		c.pc += n

		targetFrame := c.controlFrames.get(int(target))
		targetFrame.ensureContinuation()
		drop := c.getFrameDropRange(targetFrame)

		continuationLabel := &Label{FrameID: c.nextID(), Kind: LabelKindHeader}
		c.emit(
			&OperationBrIf{
				Then: &BranchTargetDrop{ToDrop: drop, Target: targetFrame.asBranchTarget()},
				Else: continuationLabel.asBranchTargetDrop(),
			},
			// Start emitting then block operations.
			&OperationLabel{
				Label: continuationLabel,
			},
		)
	case wasm.OptCodeBrTable:
		r := bytes.NewBuffer(c.f.Body[c.pc+1:])
		numTargets, n, err := leb128.DecodeUint32(r)
		if err != nil {
			return fmt.Errorf("error reading number of targets in br_table: %w", err)
		}
		c.pc += n

		// Read the branch targets.
		targets := make([]*BranchTargetDrop, numTargets)
		for i := range targets {
			l, n, err := leb128.DecodeUint32(r)
			if err != nil {
				return fmt.Errorf("error reading target %d in br_table: %w", i, err)
			}
			c.pc += n
			targetFrame := c.controlFrames.get(int(l))
			targetFrame.ensureContinuation()
			drop := c.getFrameDropRange(targetFrame)
			targets[i] = &BranchTargetDrop{ToDrop: drop, Target: targetFrame.asBranchTarget()}
		}

		// Prep default target control frame.
		l, n, err := leb128.DecodeUint32(r)
		if err != nil {
			return fmt.Errorf("error reading default target of br_table: %w", err)
		}
		c.pc += n
		defaultTargetFrame := c.controlFrames.get(int(l))
		defaultTargetFrame.ensureContinuation()
		defaultTargetDrop := c.getFrameDropRange(defaultTargetFrame)

		c.emit(
			&OperationBrTable{
				Targets: targets,
				Default: &BranchTargetDrop{
					ToDrop: defaultTargetDrop, Target: defaultTargetFrame.asBranchTarget(),
				},
			},
		)
		// Br operation is stack-polymorphic, and mark the state as unreachable.
		// That means subsequent instructions in the current control frame are "unreachable"
		// and can be safely removed.
		c.unreachableState.on = true
	case wasm.OptCodeReturn:
		functionFrame := c.controlFrames.functionFrame()
		dropOp := &OperationDrop{Range: c.getFrameDropRange(functionFrame)}

		// Cleanup the stack and then jmp to function frame's continuation (meaning return).
		c.emit(
			dropOp,
			&OperationBr{Target: functionFrame.asBranchTarget()},
		)

		// Return operation is stack-polymorphic, and mark the state as unreachable.
		// That means subsequent instructions in the current control frame are "unreachable"
		// and can be safely removed.
		c.unreachableState.on = true
	case wasm.OptCodeCall:
		if index == nil {
			return fmt.Errorf("index does not exist for function call")
		}
		c.emit(
			&OperationCall{FunctionIndex: *index},
		)
	case wasm.OptCodeCallIndirect:
		if index == nil {
			return fmt.Errorf("index does not exist for indirect function call")
		}
		tableIndex, n, err := leb128.DecodeUint32(bytes.NewBuffer(c.f.Body[c.pc+1:]))
		if err != nil {
			return fmt.Errorf("read target for br_table: %w", err)
		}
		c.pc += n
		c.emit(
			&OperationCallIndirect{TypeIndex: *index, TableIndex: tableIndex},
		)
	case wasm.OptCodeDrop:
		c.emit(
			&OperationDrop{Range: &InclusiveRange{Start: 0, End: 0}},
		)
	case wasm.OptCodeSelect:
		c.emit(
			&OperationSelect{},
		)
	case wasm.OptCodeLocalGet:
		if index == nil {
			return fmt.Errorf("index does not exist for local.get")
		}
		depth := c.localDepth(*index)
		c.emit(
			// -1 because we already manipulated the stack before
			// called localDepth ^^.
			&OperationPick{Depth: depth - 1},
		)
	case wasm.OptCodeLocalSet:
		if index == nil {
			return fmt.Errorf("index does not exist for local.set")
		}
		depth := c.localDepth(*index)
		c.emit(
			// +1 because we already manipulated the stack before
			// called localDepth ^^.
			&OperationSwap{Depth: depth + 1},
			&OperationDrop{Range: &InclusiveRange{Start: 0, End: 0}},
		)
	case wasm.OptCodeLocalTee:
		if index == nil {
			return fmt.Errorf("index does not exist for local.tee")
		}
		depth := c.localDepth(*index)
		c.emit(
			&OperationPick{Depth: 0},
			&OperationSwap{Depth: depth + 1},
			&OperationDrop{Range: &InclusiveRange{Start: 0, End: 0}},
		)
	case wasm.OptCodeGlobalGet:
		if index == nil {
			return fmt.Errorf("index does not exist for global.get")
		}
		c.emit(
			&OperationGlobalGet{Index: *index},
		)
	case wasm.OptCodeGlobalSet:
		if index == nil {
			return fmt.Errorf("index does not exist for global.set")
		}
		c.emit(
			&OperationGlobalSet{Index: *index},
		)
	case wasm.OptCodeI32Load:
		imm, err := c.readMemoryImmediate("i32.load")
		if err != nil {
			return err
		}
		c.emit(
			&OperationLoad{Type: SignLessTypeI32, Arg: imm},
		)
	case wasm.OptCodeI64Load:
		imm, err := c.readMemoryImmediate("i64.load")
		if err != nil {
			return err
		}
		c.emit(
			&OperationLoad{Type: SignLessTypeI64, Arg: imm},
		)
	case wasm.OptCodeF32Load:
		imm, err := c.readMemoryImmediate("f32.load")
		if err != nil {
			return err
		}
		c.emit(
			&OperationLoad{Type: SignLessTypeF32, Arg: imm},
		)
	case wasm.OptCodeF64Load:
		imm, err := c.readMemoryImmediate("f64.load")
		if err != nil {
			return err
		}
		c.emit(
			&OperationLoad{Type: SignLessTypeF64, Arg: imm},
		)
	case wasm.OptCodeI32Load8s:
		imm, err := c.readMemoryImmediate("i32.load8_s")
		if err != nil {
			return err
		}
		c.emit(
			&OperationLoad8{Type: SignFulInt32, Arg: imm},
		)
	case wasm.OptCodeI32Load8u:
		imm, err := c.readMemoryImmediate("i32.load8_u")
		if err != nil {
			return err
		}
		c.emit(
			&OperationLoad8{Type: SignFulUint32, Arg: imm},
		)
	case wasm.OptCodeI32Load16s:
		imm, err := c.readMemoryImmediate("i32.load16_s")
		if err != nil {
			return err
		}
		c.emit(
			&OperationLoad16{Type: SignFulInt32, Arg: imm},
		)
	case wasm.OptCodeI32Load16u:
		imm, err := c.readMemoryImmediate("i32.load16_u")
		if err != nil {
			return err
		}
		c.emit(
			&OperationLoad16{Type: SignFulUint32, Arg: imm},
		)
	case wasm.OptCodeI64Load8s:
		imm, err := c.readMemoryImmediate("i64.load8_s")
		if err != nil {
			return err
		}
		c.emit(
			&OperationLoad8{Type: SignFulInt64, Arg: imm},
		)
	case wasm.OptCodeI64Load8u:
		imm, err := c.readMemoryImmediate("i64.load8_u")
		if err != nil {
			return err
		}
		c.emit(
			&OperationLoad8{Type: SignFulUint64, Arg: imm},
		)
	case wasm.OptCodeI64Load16s:
		imm, err := c.readMemoryImmediate("i64.load16_s")
		if err != nil {
			return err
		}
		c.emit(
			&OperationLoad16{Type: SignFulInt64, Arg: imm},
		)
	case wasm.OptCodeI64Load16u:
		imm, err := c.readMemoryImmediate("i64.load16_u")
		if err != nil {
			return err
		}
		c.emit(
			&OperationLoad16{Type: SignFulUint64, Arg: imm},
		)
	case wasm.OptCodeI64Load32s:
		imm, err := c.readMemoryImmediate("i64.load32_s")
		if err != nil {
			return err
		}
		c.emit(
			&OperationLoad32{Signed: true, Arg: imm},
		)
	case wasm.OptCodeI64Load32u:
		imm, err := c.readMemoryImmediate("i64.load32_s")
		if err != nil {
			return err
		}
		c.emit(
			&OperationLoad32{Signed: false, Arg: imm},
		)
	case wasm.OptCodeI32Store:
		imm, err := c.readMemoryImmediate("i32.store")
		if err != nil {
			return err
		}
		c.emit(
			&OperationStore{Type: SignLessTypeI32, Arg: imm},
		)
	case wasm.OptCodeI64Store:
		imm, err := c.readMemoryImmediate("i64.store")
		if err != nil {
			return err
		}
		c.emit(
			&OperationStore{Type: SignLessTypeI64, Arg: imm},
		)
	case wasm.OptCodeF32Store:
		imm, err := c.readMemoryImmediate("f32.store")
		if err != nil {
			return err
		}
		c.emit(
			&OperationStore{Type: SignLessTypeF32, Arg: imm},
		)
	case wasm.OptCodeF64Store:
		imm, err := c.readMemoryImmediate("f64.store")
		if err != nil {
			return err
		}
		c.emit(
			&OperationStore{Type: SignLessTypeF64, Arg: imm},
		)
	case wasm.OptCodeI32Store8:
		imm, err := c.readMemoryImmediate("i32.store8")
		if err != nil {
			return err
		}
		c.emit(
			&OperationStore8{Type: SignLessInt32, Arg: imm},
		)
	case wasm.OptCodeI32Store16:
		imm, err := c.readMemoryImmediate("i32.store16")
		if err != nil {
			return err
		}
		c.emit(
			&OperationStore16{Type: SignLessInt32, Arg: imm},
		)
	case wasm.OptCodeI64Store8:
		imm, err := c.readMemoryImmediate("i64.store8")
		if err != nil {
			return err
		}
		c.emit(
			&OperationStore8{Type: SignLessInt64, Arg: imm},
		)
	case wasm.OptCodeI64Store16:
		imm, err := c.readMemoryImmediate("i64.store16")
		if err != nil {
			return err
		}
		c.emit(
			&OperationStore16{Type: SignLessInt64, Arg: imm},
		)
	case wasm.OptCodeI64Store32:
		imm, err := c.readMemoryImmediate("i64.store32")
		if err != nil {
			return err
		}
		c.emit(
			&OperationStore32{Arg: imm},
		)
	case wasm.OptCodeMemorySize:
		c.pc++ // Skip the reserved one byte.
		c.emit(
			&OperationMemorySize{},
		)
	case wasm.OptCodeMemoryGrow:
		c.pc++ // Skip the reserved one byte.
		c.emit(
			&OperationMemoryGrow{},
		)
	case wasm.OptCodeI32Const:
		val, num, err := leb128.DecodeInt32(bytes.NewBuffer(c.f.Body[c.pc+1:]))
		if err != nil {
			return fmt.Errorf("reading i32.const value: %v", err)
		}
		c.pc += num
		c.emit(
			&OperationConstI32{Value: uint32(val)},
		)
	case wasm.OptCodeI64Const:
		val, num, err := leb128.DecodeInt64(bytes.NewBuffer(c.f.Body[c.pc+1:]))
		if err != nil {
			return fmt.Errorf("reading i64.const value: %v", err)
		}
		c.pc += num
		c.emit(
			&OperationConstI64{Value: uint64(val)},
		)
	case wasm.OptCodeF32Const:
		v := math.Float32frombits(binary.LittleEndian.Uint32(c.f.Body[c.pc+1:]))
		c.pc += 4
		c.emit(
			&OperationConstF32{Value: v},
		)
	case wasm.OptCodeF64Const:
		v := math.Float64frombits(binary.LittleEndian.Uint64(c.f.Body[c.pc+1:]))
		c.pc += 8
		c.emit(
			&OperationConstF64{Value: v},
		)
	case wasm.OptCodeI32eqz:
		c.emit(
			&OperationEqz{Type: SignLessInt32},
		)
	case wasm.OptCodeI32eq:
		c.emit(
			&OperationEq{Type: SignLessTypeI32},
		)
	case wasm.OptCodeI32ne:
		c.emit(
			&OperationNe{Type: SignLessTypeI32},
		)
	case wasm.OptCodeI32lts:
		c.emit(
			&OperationLt{Type: SignFulTypeInt32},
		)
	case wasm.OptCodeI32ltu:
		c.emit(
			&OperationLt{Type: SignFulTypeUint32},
		)
	case wasm.OptCodeI32gts:
		c.emit(
			&OperationGt{Type: SignFulTypeInt32},
		)
	case wasm.OptCodeI32gtu:
		c.emit(
			&OperationGt{Type: SignFulTypeUint32},
		)
	case wasm.OptCodeI32les:
		c.emit(
			&OperationLe{Type: SignFulTypeInt32},
		)
	case wasm.OptCodeI32leu:
		c.emit(
			&OperationLe{Type: SignFulTypeUint32},
		)
	case wasm.OptCodeI32ges:
		c.emit(
			&OperationGe{Type: SignFulTypeInt32},
		)
	case wasm.OptCodeI32geu:
		c.emit(
			&OperationGe{Type: SignFulTypeUint32},
		)
	case wasm.OptCodeI64eqz:
		c.emit(
			&OperationEqz{Type: SignLessInt64},
		)
	case wasm.OptCodeI64eq:
		c.emit(
			&OperationEq{Type: SignLessTypeI64},
		)
	case wasm.OptCodeI64ne:
		c.emit(
			&OperationNe{Type: SignLessTypeI64},
		)
	case wasm.OptCodeI64lts:
		c.emit(
			&OperationLt{Type: SignFulTypeInt64},
		)
	case wasm.OptCodeI64ltu:
		c.emit(
			&OperationLt{Type: SignFulTypeUint64},
		)
	case wasm.OptCodeI64gts:
		c.emit(
			&OperationGt{Type: SignFulTypeInt64},
		)
	case wasm.OptCodeI64gtu:
		c.emit(
			&OperationGt{Type: SignFulTypeUint64},
		)
	case wasm.OptCodeI64les:
		c.emit(
			&OperationLe{Type: SignFulTypeInt64},
		)
	case wasm.OptCodeI64leu:
		c.emit(
			&OperationLe{Type: SignFulTypeUint64},
		)
	case wasm.OptCodeI64ges:
		c.emit(
			&OperationGe{Type: SignFulTypeInt64},
		)
	case wasm.OptCodeI64geu:
		c.emit(
			&OperationGe{Type: SignFulTypeUint64},
		)
	case wasm.OptCodeF32eq:
		c.emit(
			&OperationEq{Type: SignLessTypeF32},
		)
	case wasm.OptCodeF32ne:
		c.emit(
			&OperationNe{Type: SignLessTypeF32},
		)
	case wasm.OptCodeF32lt:
		c.emit(
			&OperationLt{Type: SignFulTypeFloat32},
		)
	case wasm.OptCodeF32gt:
		c.emit(
			&OperationGt{Type: SignFulTypeFloat32},
		)
	case wasm.OptCodeF32le:
		c.emit(
			&OperationLe{Type: SignFulTypeFloat32},
		)
	case wasm.OptCodeF32ge:
		c.emit(
			&OperationGe{Type: SignFulTypeFloat32},
		)
	case wasm.OptCodeF64eq:
		c.emit(
			&OperationEq{Type: SignLessTypeF64},
		)
	case wasm.OptCodeF64ne:
		c.emit(
			&OperationNe{Type: SignLessTypeF64},
		)
	case wasm.OptCodeF64lt:
		c.emit(
			&OperationLt{Type: SignFulTypeFloat64},
		)
	case wasm.OptCodeF64gt:
		c.emit(
			&OperationGt{Type: SignFulTypeFloat64},
		)
	case wasm.OptCodeF64le:
		c.emit(
			&OperationLe{Type: SignFulTypeFloat64},
		)
	case wasm.OptCodeF64ge:
		c.emit(
			&OperationGe{Type: SignFulTypeFloat64},
		)
	case wasm.OptCodeI32clz:
		c.emit(
			&OperationClz{Type: SignLessInt32},
		)
	case wasm.OptCodeI32ctz:
		c.emit(
			&OperationCtz{Type: SignLessInt32},
		)
	case wasm.OptCodeI32popcnt:
		c.emit(
			&OperationPopcnt{Type: SignLessInt32},
		)
	case wasm.OptCodeI32add:
		c.emit(
			&OperationAdd{Type: SignLessTypeI32},
		)
	case wasm.OptCodeI32sub:
		c.emit(
			&OperationSub{Type: SignLessTypeI32},
		)
	case wasm.OptCodeI32mul:
		c.emit(
			&OperationMul{Type: SignLessTypeI32},
		)
	case wasm.OptCodeI32divs:
		c.emit(
			&OperationDiv{Type: SignFulTypeInt32},
		)
	case wasm.OptCodeI32divu:
		c.emit(
			&OperationDiv{Type: SignFulTypeUint32},
		)
	case wasm.OptCodeI32rems:
		c.emit(
			&OperationRem{Type: SignFulInt32},
		)
	case wasm.OptCodeI32remu:
		c.emit(
			&OperationRem{Type: SignFulUint32},
		)
	case wasm.OptCodeI32and:
		c.emit(
			&OperationAnd{Type: SignLessInt32},
		)
	case wasm.OptCodeI32or:
		c.emit(
			&OperationOr{Type: SignLessInt32},
		)
	case wasm.OptCodeI32xor:
		c.emit(
			&OperationXor{Type: SignLessInt64},
		)
	case wasm.OptCodeI32shl:
		c.emit(
			&OperationShl{Type: SignLessInt32},
		)
	case wasm.OptCodeI32shrs:
		c.emit(
			&OperationShr{Type: SignFulInt32},
		)
	case wasm.OptCodeI32shru:
		c.emit(
			&OperationShr{Type: SignFulUint32},
		)
	case wasm.OptCodeI32rotl:
		c.emit(
			&OperationRotl{Type: SignLessInt32},
		)
	case wasm.OptCodeI32rotr:
		c.emit(
			&OperationRotr{Type: SignLessInt32},
		)
	case wasm.OptCodeI64clz:
		c.emit(
			&OperationClz{Type: SignLessInt64},
		)
	case wasm.OptCodeI64ctz:
		c.emit(
			&OperationCtz{Type: SignLessInt64},
		)
	case wasm.OptCodeI64popcnt:
		c.emit(
			&OperationPopcnt{Type: SignLessInt64},
		)
	case wasm.OptCodeI64add:
		c.emit(
			&OperationAdd{Type: SignLessTypeI64},
		)
	case wasm.OptCodeI64sub:
		c.emit(
			&OperationSub{Type: SignLessTypeI64},
		)
	case wasm.OptCodeI64mul:
		c.emit(
			&OperationMul{Type: SignLessTypeI64},
		)
	case wasm.OptCodeI64divs:
		c.emit(
			&OperationDiv{Type: SignFulTypeInt64},
		)
	case wasm.OptCodeI64divu:
		c.emit(
			&OperationDiv{Type: SignFulTypeUint64},
		)
	case wasm.OptCodeI64rems:
		c.emit(
			&OperationRem{Type: SignFulInt64},
		)
	case wasm.OptCodeI64remu:
		c.emit(
			&OperationRem{Type: SignFulUint64},
		)
	case wasm.OptCodeI64and:
		c.emit(
			&OperationAnd{Type: SignLessInt64},
		)
	case wasm.OptCodeI64or:
		c.emit(
			&OperationOr{Type: SignLessInt64},
		)
	case wasm.OptCodeI64xor:
		c.emit(
			&OperationXor{Type: SignLessInt64},
		)
	case wasm.OptCodeI64shl:
		c.emit(
			&OperationShl{Type: SignLessInt64},
		)
	case wasm.OptCodeI64shrs:
		c.emit(
			&OperationShr{Type: SignFulInt64},
		)
	case wasm.OptCodeI64shru:
		c.emit(
			&OperationShr{Type: SignFulUint64},
		)
	case wasm.OptCodeI64rotl:
		c.emit(
			&OperationRotl{Type: SignLessInt64},
		)
	case wasm.OptCodeI64rotr:
		c.emit(
			&OperationRotr{Type: SignLessInt64},
		)
	case wasm.OptCodeF32abs:
		c.emit(
			&OperationAbs{Type: Float32},
		)
	case wasm.OptCodeF32neg:
		c.emit(
			&OperationNeg{Type: Float32},
		)
	case wasm.OptCodeF32ceil:
		c.emit(
			&OperationCeil{Type: Float32},
		)
	case wasm.OptCodeF32floor:
		c.emit(
			&OperationFloor{Type: Float32},
		)
	case wasm.OptCodeF32trunc:
		c.emit(
			&OperationTrunc{Type: Float32},
		)
	case wasm.OptCodeF32nearest:
		c.emit(
			&OperationNearest{Type: Float32},
		)
	case wasm.OptCodeF32sqrt:
		c.emit(
			&OperationSqrt{Type: Float32},
		)
	case wasm.OptCodeF32add:
		c.emit(
			&OperationAdd{Type: SignLessTypeF32},
		)
	case wasm.OptCodeF32sub:
		c.emit(
			&OperationSub{Type: SignLessTypeF32},
		)
	case wasm.OptCodeF32mul:
		c.emit(
			&OperationMul{Type: SignLessTypeF32},
		)
	case wasm.OptCodeF32div:
		c.emit(
			&OperationDiv{Type: SignFulTypeFloat32},
		)
	case wasm.OptCodeF32min:
		c.emit(
			&OperationMin{Type: Float32},
		)
	case wasm.OptCodeF32max:
		c.emit(
			&OperationMax{Type: Float32},
		)
	case wasm.OptCodeF32copysign:
		c.emit(
			&OperationCopysign{Type: Float32},
		)
	case wasm.OptCodeF64abs:
		c.emit(
			&OperationAbs{Type: Float64},
		)
	case wasm.OptCodeF64neg:
		c.emit(
			&OperationNeg{Type: Float64},
		)
	case wasm.OptCodeF64ceil:
		c.emit(
			&OperationCeil{Type: Float64},
		)
	case wasm.OptCodeF64floor:
		c.emit(
			&OperationFloor{Type: Float64},
		)
	case wasm.OptCodeF64trunc:
		c.emit(
			&OperationTrunc{Type: Float64},
		)
	case wasm.OptCodeF64nearest:
		c.emit(
			&OperationNearest{Type: Float64},
		)
	case wasm.OptCodeF64sqrt:
		c.emit(
			&OperationSqrt{Type: Float64},
		)
	case wasm.OptCodeF64add:
		c.emit(
			&OperationAdd{Type: SignLessTypeF64},
		)
	case wasm.OptCodeF64sub:
		c.emit(
			&OperationSub{Type: SignLessTypeF64},
		)
	case wasm.OptCodeF64mul:
		c.emit(
			&OperationMul{Type: SignLessTypeF64},
		)
	case wasm.OptCodeF64div:
		c.emit(
			&OperationDiv{Type: SignFulTypeFloat64},
		)
	case wasm.OptCodeF64min:
		c.emit(
			&OperationMin{Type: Float64},
		)
	case wasm.OptCodeF64max:
		c.emit(
			&OperationMax{Type: Float64},
		)
	case wasm.OptCodeF64copysign:
		c.emit(
			&OperationCopysign{Type: Float64},
		)
	case wasm.OptCodeI32wrapI64:
		c.emit(
			&OperationI32WrapFromI64{},
		)
	case wasm.OptCodeI32truncf32s:
		c.emit(
			&OperationITruncFromF{InputType: Float32, OutputType: SignFulInt32},
		)
	case wasm.OptCodeI32truncf32u:
		c.emit(
			&OperationITruncFromF{InputType: Float32, OutputType: SignFulUint32},
		)
	case wasm.OptCodeI32truncf64s:
		c.emit(
			&OperationITruncFromF{InputType: Float64, OutputType: SignFulInt32},
		)
	case wasm.OptCodeI32truncf64u:
		c.emit(
			&OperationITruncFromF{InputType: Float64, OutputType: SignFulUint32},
		)
	case wasm.OptCodeI64Extendi32s:
		c.emit(
			&OperationExtend{Signed: true},
		)
	case wasm.OptCodeI64Extendi32u:
		c.emit(
			&OperationExtend{Signed: false},
		)
	case wasm.OptCodeI64TruncF32s:
		c.emit(
			&OperationITruncFromF{InputType: Float32, OutputType: SignFulInt64},
		)
	case wasm.OptCodeI64TruncF32u:
		c.emit(
			&OperationITruncFromF{InputType: Float32, OutputType: SignFulUint64},
		)
	case wasm.OptCodeI64Truncf64s:
		c.emit(
			&OperationITruncFromF{InputType: Float64, OutputType: SignFulInt64},
		)
	case wasm.OptCodeI64Truncf64u:
		c.emit(
			&OperationITruncFromF{InputType: Float64, OutputType: SignFulUint64},
		)
	case wasm.OptCodeF32Converti32s:
		c.emit(
			&OperationFConvertFromI{InputType: SignFulInt32, OutputType: Float32},
		)
	case wasm.OptCodeF32ConvertI32u:
		c.emit(
			&OperationFConvertFromI{InputType: SignFulUint32, OutputType: Float32},
		)
	case wasm.OptCodeF32Converti64s:
		c.emit(
			&OperationFConvertFromI{InputType: SignFulInt64, OutputType: Float32},
		)
	case wasm.OptCodeF32Converti64u:
		c.emit(
			&OperationFConvertFromI{InputType: SignFulUint64, OutputType: Float32},
		)
	case wasm.OptCodeF32Demotef64:
		c.emit(
			&OperationF32DemoteFromF64{},
		)
	case wasm.OptCodeF64Converti32s:
		c.emit(
			&OperationFConvertFromI{InputType: SignFulInt32, OutputType: Float64},
		)
	case wasm.OptCodeF64Converti32u:
		c.emit(
			&OperationFConvertFromI{InputType: SignFulUint32, OutputType: Float64},
		)
	case wasm.OptCodeF64Converti64s:
		c.emit(
			&OperationFConvertFromI{InputType: SignFulInt64, OutputType: Float64},
		)
	case wasm.OptCodeF64Converti64u:
		c.emit(
			&OperationFConvertFromI{InputType: SignFulUint64, OutputType: Float64},
		)
	case wasm.OptCodeF64Promotef32:
		c.emit(
			&OperationF64PromoteFromF32{},
		)
	case wasm.OptCodeI32Reinterpretf32:
		c.emit(
			&OperationI32ReinterpretFromF32{},
		)
	case wasm.OptCodeI64reinterpretf64:
		c.emit(
			&OperationI64ReinterpretFromF64{},
		)
	case wasm.OptCodeF32reinterpreti32:
		c.emit(
			&OperationF32ReinterpretFromI32{},
		)
	case wasm.OptCodeF64reinterpreti64:
		c.emit(
			&OperationF64ReinterpretFromI64{},
		)
	default:
		return fmt.Errorf("unsupported instruction in wazeroir: 0x%x", op)
	}

	// Move the program counter to point to the next instruction.
	c.pc++
	return nil
}

func (c *compiler) nextID() (id uint32) {
	id = c.currentID + 1
	c.currentID++
	return
}

func (c *compiler) applyToStack(optCode wasm.OptCode) (*uint32, error) {
	var index uint32
	var ptr *uint32
	switch optCode {
	case
		// These are the optcodes that is coupled with "index"　immediate
		// and it DOES affect the signature of optcode.
		wasm.OptCodeCall,
		wasm.OptCodeCallIndirect,
		wasm.OptCodeLocalGet,
		wasm.OptCodeLocalSet,
		wasm.OptCodeLocalTee,
		wasm.OptCodeGlobalGet,
		wasm.OptCodeGlobalSet:
		// Assumes that we are at the optcode now so skip it before read immediates.
		v, num, err := leb128.DecodeUint32(bytes.NewBuffer(c.f.Body[c.pc+1:]))
		if err != nil {
			return nil, fmt.Errorf("reading immediates: %w", err)
		}
		c.pc += num
		index = v
		ptr = &index
	default:
		// Note that other optcodes are free of index
		// as it doesn't affect the signature of opt code.
		// In other words, the "index" argument of wasmOptcodeSignature
		// is ignored there.
	}

	if c.unreachableState.on {
		return ptr, nil
	}

	// Retrieve the signature of the optcode.
	s, err := wasmOptcodeSignature(c.f, optCode, index)
	if err != nil {
		return nil, err
	}

	// Manipulate the stack according to the signtature.
	// Note that the following algorithm assumes that
	// the unkown type is unique in the signature,
	// and is determined by the actual type on the stack.
	// The determined type is stored in this typeParam.
	var typeParam *SignLessType
	for i := range s.in {
		want := s.in[len(s.in)-1-i]
		actual := c.stackPop()
		if want == SignLessTypeUnknown && typeParam != nil {
			want = *typeParam
		} else if want == SignLessTypeUnknown {
			want = actual
			typeParam = &actual
		}
		if want != actual {
			return nil, fmt.Errorf("input signature mismatch: want %s but got %s", want, actual)
		}
	}

	for _, target := range s.out {
		if target == SignLessTypeUnknown && typeParam == nil {
			return nil, fmt.Errorf("cannot determine type of unknown result")
		} else if target == SignLessTypeUnknown {
			c.stackPush(*typeParam)
		} else {
			c.stackPush(target)
		}
	}

	return ptr, nil
}

func (c *compiler) stackPop() (ret SignLessType) {
	// No need to check stack bound
	// as we can assume that all the operations
	// are valid thanks to analyzeFunction
	// at module validation phase.
	ret = c.stack[len(c.stack)-1]
	c.stack = c.stack[:len(c.stack)-1]
	return
}

func (c *compiler) stackPush(t SignLessType) {
	c.stack = append(c.stack, t)
}

// Emit the operatiosn into the result.
func (c *compiler) emit(ops ...Operation) {
	if !c.unreachableState.on {
		for _, op := range ops {
			switch o := op.(type) {
			case *OperationDrop:
				// If the drop range is nil,
				// we could remove such operatinos.
				// That happens when drop operation is unnecessary.
				// i.e. when there's no need to adjust stack before jmp.
				if o.Range == nil {
					continue
				}
			}
			c.result = append(c.result, op)
			if buildoptions.IsDebugMode {
				fmt.Printf("emitting ")
				formatOperation(os.Stdout, op)
			}
		}
	}
}

// Emit const expression with default values of the given type.
func (c *compiler) emitDefaultValue(t wasm.ValueType) {
	switch t {
	case wasm.ValueTypeI32:
		c.stackPush(SignLessTypeI32)
		c.emit(&OperationConstI32{Value: 0})
	case wasm.ValueTypeI64:
		c.stackPush(SignLessTypeI64)
		c.emit(&OperationConstI64{Value: 0})
	case wasm.ValueTypeF32:
		c.stackPush(SignLessTypeF32)
		c.emit(&OperationConstF32{Value: 0})
	case wasm.ValueTypeF64:
		c.stackPush(SignLessTypeF64)
		c.emit(&OperationConstF64{Value: 0})
	}
}

// Returns the "depth" (starting from top of the stack)
// of the n-th local.
func (c *compiler) localDepth(n uint32) int {
	return int(len(c.stack)) - 1 - int(n)
}

// Returns the range (starting from top of the stack) that spans across
// the stack. The range is supposed to be dropped from the stack when
// the given frame exists.
func (c *compiler) getFrameDropRange(frame *controlFrame) *InclusiveRange {
	start := len(frame.returns)
	var end int
	if frame.kind == controlFrameKindFunction {
		// On the function return, we eliminate all the contents on the stack
		// including locals (existing below of frame.originalStackLen)
		end = len(c.stack) - 1
	} else {
		end = len(c.stack) - 1 - frame.originalStackLen
	}
	if start <= end {
		return &InclusiveRange{Start: start, End: end}
	}
	return nil
}

func (c *compiler) readMemoryImmediate(tag string) (*MemoryImmediate, error) {
	r := bytes.NewBuffer(c.f.Body[c.pc+1:])
	alignment, num, err := leb128.DecodeUint32(r)
	if err != nil {
		return nil, fmt.Errorf("reading alignment for %s: %w", tag, err)
	}
	c.pc += num
	offset, num, err := leb128.DecodeUint32(r)
	if err != nil {
		return nil, fmt.Errorf("reading offset for %s: %w", tag, err)
	}
	c.pc += num
	return &MemoryImmediate{Offest: offset, Alignment: alignment}, nil
}
