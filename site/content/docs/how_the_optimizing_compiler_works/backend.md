+++
title = "How the Optimizing Compiler Works: Back-End"
layout = "single"
+++

In this section we will discuss the phases in the back-end of the optimizing
compiler:

- [Instruction Selection](#instruction-selection)
- [Register Allocation](#register-allocation)
- [Finalization and Encoding](#finalization-and-encoding)

Each section will include a brief explanation of the phase, references to the
code that implements the phase, and a description of the debug flags that can
be used to inspect that phase.  Please notice that, since the implementation of
the back-end is architecture-specific, the code might be different for each
architecture.

### Code

The higher-level entry-point to the back-end is the
`backend.Compiler.Compile(context.Context)` method.  This method executes, in
turn, the following methods in the same type:

- `backend.Compiler.Lower()` (instruction selection)
- `backend.Compiler.RegAlloc()` (register allocation)
- `backend.Compiler.Finalize(context.Context)` (finalization and encoding)

## Instruction Selection

The instruction selection phase is responsible for mapping the higher-level SSA
instructions to arch-specific instructions. Each SSA instruction is translated
to one or more machine instructions.

Each target architecture comes with a different number of registers, some of
them are general purpose, others might be specific to certain instructions. In
general, we can expect to have a set of registers for integer computations,
another set for floating point computations, a set for vector (SIMD)
computations, and some specific special-purpose registers (e.g. stack pointers,
program counters, status flags, etc.)

In addition, some registers might be reserved by the Go runtime or the
Operating System for specific purposes, so they should be handled with special
care.

At this point in the compilation process we do not want to deal with all that.
Instead, we assume that we have a potentially infinite number of *virtual
registers* of each type at our disposal. The next phase, the register
allocation phase, will map these virtual registers to the actual registers of
the target architecture.

### Operands and Addressing Modes

As a rule of thumb, we want to map each `ssa.Value` to a virtual register, and
then use that virtual register as one of the arguments of the machine
instruction that we will generate. However, usually instructions are able to
address more than just registers: an *operand* might be able to represent a
memory address, or an immediate value (i.e. a constant value that is encoded as
part of the instruction itself).

For these reasons, instead of mapping each `ssa.Value` to a virtual register
(`regalloc.VReg`), we map each `ssa.Value` to an architecture-specific
`operand` type.

During lowering of an `ssa.Instruction`, for each `ssa.Value` that is used as
an argument of the instruction, in the simplest case, the `operand` might be
mapped to a virtual register, in other cases, the `operand` might be mapped to
a memory address, or an immediate value. Sometimes this makes it possible to
replace several SSA instructions with a single machine instruction, by folding
the addressing mode into the instruction itself.

For instance, consider the following SSA instructions:

```
    v4:i32 = Const 0x9
    v6:i32 = Load v5, 0x4
    v7:i32 = Iadd v6, v4
```

In the `amd64` architecture, the `add` instruction adds the second operand to
the first operand, and assigns the result to the second operand. So assuming
that `r4`, `v5`, `v6`, and `v7` are mapped respectively to the virtual
registers `r4?`, `r5?`, `r6?`, and `r7?`, the lowering of the `Iadd`
instruction on `amd64` might look like this:

```asm
    ;; AT&T syntax
    add $4(%r5?), %r4? ;; add the value at memory address [`r5?` + 4] to `r4?`
    mov %r4?, %r7?     ;; move the result from `r4?` to `r7?`
```

Notice how the load from memory has been folded into an operand of the `add`
instruction. This transformation is possible when the value produced by the
instruction being folded is not referenced by other instructions and the
instructions belong to the same `InstructionGroupID` (see [Front-End:
Optimization](../frontend/#optimization)).

### Example

At the end of the instruction selection phase, the basic blocks of our `abs`
function will look as follows (for `arm64`):

```asm
L1 (SSA Block: blk0):
	mov x130?, x2
	subs wzr, w130?, #0x0
	b.ge L2
L3 (SSA Block: blk1):
	mov x136?, xzr
	sub w134?, w136?, w130?
	mov x135?, x134?
	b L4
L2 (SSA Block: blk2):
	mov x135?, x130?
L4 (SSA Block: blk3):
	mov x0, x135?
	ret
```

Notice the introduction of the new identifiers `L1`, `L3`, `L2`, and `L4`.
These are labels that are used to mark the beginning of each basic block, and
they are the target for branching instructions such as `b` and `b.ge`.

### Code

`backend.Machine` is the interface to the backend. It has a methods to
translate (lower) the IR to machine code.  Again, as seen earlier in the
front-end, the term *lowering* is used to indicate translation from a
higher-level representation to a lower-level representation.

`backend.Machine.LowerInstr(*ssa.Instruction)` is the method that translates an
SSA instruction to machine code.  Machine-specific implementations of this
method can be found in package `backend/isa/<arch>` where `<arch>` is either
`amd64` or `arm64`.

### Debug Flags

`wazevoapi.PrintSSAToBackendIRLowering` prints the basic blocks with the
lowered arch-specific instructions.

## Register Allocation

The register allocation phase is responsible for mapping the potentially
infinite number of virtual registers to the real registers of the target
architecture. Because the number of real registers is limited, the register
allocation phase might need to "spill" some of the virtual registers to memory;
that is, it might store their content, and then load them back into a register
when they are needed.

For a given function `f` the register allocation procedure
`regalloc.Allocator.DoAllocation(f)` is implemented in sub-phases:

- `livenessAnalysis(f)` collects the "liveness" information for each virtual
  register. The algorithm is described in [Chapter 9.2 of The SSA
Book][ssa-book].

- `alloc(f)` allocates registers for the given function. The algorithm is
  derived from [the Go compiler's
allocator][go-regalloc]

At the end of the allocation procedure, we also record the set of registers
that are **clobbered** by the body of the function. A register is clobbered
if its value is overwritten by the function, and it is not saved by the
callee. This information is used in the finalization phase to determine which
registers need to be saved in the prologue and restored in the epilogue.
to register allocation in a textbook meaning, but it is a necessary step
for the finalization phase.

### Liveness Analysis

Intuitively, a variable or name binding can be considered _live_ at a certain
point in a program, if its value will be used in the future.

For instance:

```
1| int f(int x) {
2|   int y = 2 + x;
3|   int z = x + y;
4|   return z;
5| }
```

Variable `x` and `y` are both live at line 4, because they are used in the
expression `x + y` on line 3; variable `z` is live at line 4, because it is
used in the return statement.  However, variables `x` and `y` can be considered
_not_ live at line 4 because they are not used anywhere after line 3.

Statically, _liveness_ can be approximated by following paths backwards on the
control-flow graph, connecting the uses of a given variable to its definitions
(or its *unique* definition, assuming SSA form).

In practice, while liveness is a property of each name binding at any point in
the program, it is enough to keep track of liveness at the boundaries of basic
blocks:

- the _live-in_ set for a given basic block is the set of all bindings that are
  live at the entry of that block.
- the _live-out_ set for a given basic block is the set of all bindings that
  are live at the exit of that block. A binding is live at the exit of a block
if it is live at the entry of a successor.

Because the CFG is a connected graph, it is enough to keep track of either
live-in or live-out sets, and then propagate the liveness information backward
or forward, respectively. In our case, we keep track of live-in sets per block;
live-outs are derived from live-ins of the successor blocks when a block is
allocated.

### Allocation

We implemented a variant of the linear scan register allocation algorithm
described in [the Go compiler's allocator][go-regalloc].

Each basic block is allocated registers in a linear scan order, and the
allocation state is propagated from a given basic block to its successors.
Then, each block continues allocation from that initial state.

#### Merge States

Special care has to be taken when a block has multiple predecessors. We call
this *fixing merge states*: for instance, consider the following:

```goat { width="30%" }
 .---.     .---.
| BB0 |   | BB1 |
 '-+-'     '-+-'
   +----+----+
        |
        v
      .---.
     | BB2 |
      '---'
```

if the live-out set of a given block `BB0` is different from the live-out set
of a given block `BB1` and both are predecessors of a block `BB2`, then we need
to adjust `BB0` and `BB1` to ensure consistency with `BB2`. In practice,
abstract values in `BB0` and `BB1` might be passed to `BB2` either via registers
or via stack; fixing merge states ensures that registers and stack are used
consistently to pass values across the involved states.

#### Spilling

If the register allocator cannot find a free register for a given virtual
(live) register, it needs to "spill" the value to the stack to get a free
register, *i.e.,* stash it temporarily to stack.  When that virtual register is
reused later, we will have to insert instructions to reload the value into a
real register.

While the procedure proceeds with allocation, the procedure also records all
the virtual registers that transition to the "spilled" state, and inserts the
reload instructions when those registers are reused later.

The spill instructions are actually inserted at the end of the register
allocation, after all the allocations and the merge states have been fixed. At
this point, all the other potential sources of instability have been resolved,
and we know where all the reloads happen.

We insert the spills in the block that is the lowest common ancestor of all the
blocks that reload the value.

#### Clobbered Registers

At the end of the allocation procedure, the `determineCalleeSavedRealRegs(f)`
method iterates over the set of the allocated registers and compares them
to a set of architecture-specific set `CalleeSavedRegisters`. If a register
has been allocated, and it is present in this set, the register is marked as
"clobbered", i.e., we now know that the register allocator will overwrite
that value. Thus, these values will have to be spilled in the prologue.

#### References

Register allocation is a complex problem, possibly the most complicated
part of the backend. The following references were used to implement the
algorithm:

- https://web.stanford.edu/class/archive/cs/cs143/cs143.1128/lectures/17/Slides17.pdf
- https://en.wikipedia.org/wiki/Chaitin%27s_algorithm
- https://llvm.org/ProjectsWithLLVM/2004-Fall-CS426-LS.pdf
- https://pfalcon.github.io/ssabook/latest/book-full.pdf: Chapter 9. for liveness analysis.
- https://github.com/golang/go/blob/release-branch.go1.21/src/cmd/compile/internal/ssa/regalloc.go

We suggest to refer to them to dive deeper in the topic.

### Example

At the end of the register allocation phase, the basic blocks of our `abs`
function look as follows (for `arm64`):

```asm
L1 (SSA Block: blk0):
	mov x2, x2
	subs wzr, w2, #0x0
	b.ge L2
L3 (SSA Block: blk1):
	mov x8, xzr
	sub w8, w8, w2
	mov x8, x8
	b L4
L2 (SSA Block: blk2):
	mov x8, x2
L4 (SSA Block: blk3):
	mov x0, x8
	ret
```

Notice how the virtual registers have been all replaced by real registers, i.e.
no register identifier is suffixed with `?`. This example is quite simple, and
it does not require any spill.

### Code

The algorithm (`regalloc/regalloc.go`) can work on any ISA by implementing the
interfaces in `regalloc/api.go`.

Essentially:

- each architecture exposes iteration over basic blocks of a function
  (`regalloc.Function` interface)
- each arch-specific basic block exposes iteration over instructions
  (`regalloc.Block` interface)
- each arch-specific instruction exposes the set of registers it defines and
  uses  (`regalloc.Instr` interface)

By defining these interfaces, the register allocation algorithm can assign real
registers to virtual registers without dealing specifically with the target
architecture.

In practice, each interface is usually implemented by instantiating a common
generic struct that comes already with an implementation of all or most of the
required methods.  For instance,`regalloc.Function`is implemented by
`backend.RegAllocFunction[*arm64.instruction, *arm64.machine]`.

`backend/isa/<arch>/abi.go` (where `<arch>` is either `arm64` or `amd64`)
contains the instantiation of the `regalloc.RegisterInfo` struct, which
declares, among others
- the set of registers that are available for allocation, excluding, for
  instance, those that might be reserved by the runtime or the OS
(`AllocatableRegisters`)
- the registers that might be saved by the callee to the stack
  (`CalleeSavedRegisters`)

### Debug Flags

- `wazevoapi.RegAllocLoggingEnabled` logs detailed logging of the register
  allocation procedure.
- `wazevoapi.PrintRegisterAllocated` prints the basic blocks with the register
  allocation result.

## Finalization and Encoding

At the end of the register allocation phase, we have enough information to
finally generate machine code (_encoding_). We are only missing the prologue
and epilogue of the function.

### Prologue and Epilogue

As usual, the **prologue** is executed before the main body of the function,
and the **epilogue** is executed at the return. The prologue is responsible for
setting up the stack frame, and the epilogue is responsible for cleaning up the
stack frame and returning control to the caller.

Generally, this means, at the very least:
- saving the return address
- a base pointer to the stack; or, equivalently, the height of the stack at the
  beginning of the function

For instance, on `amd64`, `RBP` is the base pointer, `RSP` is the stack
pointer:

```goat {width="100%" height="250"}
                (high address)                     (high address)
    RBP ----> +-----------------+                +-----------------+
              |      `...`      |                |      `...`      |
              |      ret Y      |                |      ret Y      |
              |      `...`      |                |      `...`      |
              |      ret 0      |                |      ret 0      |
              |      arg X      |                |      arg X      |
              |      `...`      |     ====>      |      `...`      |
              |      arg 1      |                |      arg 1      |
              |      arg 0      |                |      arg 0      |
              |   Return Addr   |                |   Return Addr   |
    RSP ----> +-----------------+                |    Caller_RBP   |
                 (low address)                   +-----------------+ <----- RSP, RBP
```

While, on `arm64`, there is only a stack pointer `SP`:


```goat {width="100%" height="300"}
            (high address)                    (high address)
  SP ---> +-----------------+               +------------------+ <----+
          |      `...`      |               |      `...`       |      |
          |      ret Y      |               |      ret Y       |      |
          |      `...`      |               |      `...`       |      |
          |      ret 0      |               |      ret 0       |      |
          |      arg X      |               |      arg X       |      |  size_of_arg_ret.
          |      `...`      |     ====>     |      `...`       |      |
          |      arg 1      |               |      arg 1       |      |
          |      arg 0      |               |      arg 0       | <----+
          +-----------------+               |  size_of_arg_ret |
                                            |  return address  |
                                            +------------------+ <---- SP
             (low address)                     (low address)
```

However, the prologue and epilogue might also be responsible for saving and
restoring the state of registers that might be overwritten by the function
("clobbered"); and, if spilling occurs, prologue and epilogue are also
responsible for reserving and releasing the space for the spilled values.

For clarity, we make a distinction between the space reserved for the clobbered
registers and the space reserved for the spilled values:

- Spill slots are used to temporarily store the values that needs spilling as
  determined by the register allocator. This section must have a fix height,
but its contents will change over time, as registers are being spilled and
reloaded.
- Clobbered registers are, similarly, determined by the register allocator, but
  they are stashed in the prologue and then restored in the epilogue.

The procedure happens after the register allocation phase because at
this point we have collected enough information to know how much space we need
to reserve, and which registers are clobbered.

Regardless of the architecture, after allocating this space, the stack will
look as follows:

```goat {height="350"}
    (high address)
  +-----------------+
  |      `...`      |
  |      ret Y      |
  |      `...`      |
  |      ret 0      |
  |      arg X      |
  |      `...`      |
  |      arg 1      |
  |      arg 0      |
  | (arch-specific) |
  +-----------------+
  |    clobbered M  |
  |   ............  |
  |    clobbered 1  |
  |    clobbered 0  |
  |   spill slot N  |
  |   ............  |
  |   spill slot 0  |
  +-----------------+
     (low address)
```

Note: the prologue might also introduce a check of the stack bounds. If there
is no sufficient space to allocate the stack frame, the function will exit the
execution and will try to grow it from the Go runtime.

The epilogue simply reverses the operations of the prologue.

### Other Post-RegAlloc Logic

The `backend.Machine.PostRegAlloc` method is invoked after the register
allocation procedure; while its main role is to define the prologue and
epilogue of the function, it also serves as a hook to perform other,
arch-specific duty, that has to happen after the register allocation phase.

For instance, on `amd64`, the constraints for some instructions are hard to
express in a meaningful way for the register allocation procedure (for
instance, the `div` instruction implicitly use registers `rdx`, `rax`).
Instead, they are lowered with ad-hoc logic as part of the implementation
`backend.Machine.PostRegAlloc` method.

### Encoding

The final stage of the backend encodes the machine instructions into bytes and
writes them to the target buffer. Before proceeding with the encoding, relative
addresses in branching instructions or addressing modes are resolved.

The procedure encodes the instructions in the order they appear in the
function.

### Code

- The prologue and epilogue are set up as part of the
  `backend.Machine.PostRegAlloc` method.
- The encoding is done by the `backend.Machine.Encode` method.

### Debug Flags

- `wazevoapi.PrintFinalizedMachineCode` prints the assembly code of the
  function after the finalization phase.
- `wazevoapi.printMachineCodeHexPerFunctionUnmodified` prints a hex
  representation of the function generated code as it is.
- `wazevoapi.PrintMachineCodeHexPerFunctionDisassemblable` prints a hex
  representation of the function generated code that can be disassembled.

The reason for the distinction between the last two flags is that the generated
code in some cases might not be disassemblable.
`PrintMachineCodeHexPerFunctionDisassemblable` flag prints a hex encoding of
the generated code that can be disassembled, but cannot be executed.

<hr>

* Previous Section: [Front-End](../frontend/)
* Next Section: [Appendix: Trampolines](../appendix/)

[ssa-book]: https://pfalcon.github.io/ssabook/latest/book-full.pdf
[go-regalloc]: https://github.com/golang/go/blob/release-branch.go1.21/src/cmd/compile/internal/ssa/regalloc.go
