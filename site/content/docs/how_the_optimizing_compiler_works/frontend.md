+++
title = "How the Optimizing Compiler Works: Front-End"
layout = "single"
+++

In this section we will discuss the phases in the front-end of the optimizing compiler:

- [Translation to SSA](#translation-to-ssa)
- [Optimization](#optimization)
- [Block Layout](#block-layout)

Every section includes an explanation of the phase; the subsection **Code**
will include high-level pointers to functions and packages; the subsection **Debug Flags**
indicates the flags that can be used to enable advanced logging of the phase.

## Translation to SSA

We mentioned earlier that wazero uses an internal representation called an "SSA"
form or "Static Single-Assignment" form, but we never explained what that is.

In short terms, every program, or, in our case, every Wasm function, can be
translated in a control-flow graph. The control-flow graph is a directed graph where
each node is a sequence of statements that do not contain a control flow instruction,
called a **basic block**. Instead, control-flow instructions are translated into edges.

For instance, take the following implementation of the `abs` function:

```wasm
(module
  (func (;0;) (param i32) (result i32)
     (if (result i32) (i32.lt_s (local.get 0) (i32.const 0))
        (then
            (i32.sub (i32.const 0) (local.get 0)))
        (else
            (local.get 0))
     )
  )
  (export "f" (func 0))
)
```

This is translated to the following block diagram:

```goat {width="100%" height="500"}
               +---------------------------------------------+
               |blk0: (exec_ctx:i64, module_ctx:i64, v2:i32) |
               |    v3:i32 = Iconst_32 0x0                   |
               |    v4:i32 = Icmp lt_s, v2, v3               |
               |    Brz v4, blk2                             |
               |    Jump blk1                                |
               +---------------------------------------------+
                                      |
                                      |
                      +---`(v4 != 0)`-+-`(v4 == 0)`---+
                      |                               |
                      v                               v
        +---------------------------+   +---------------------------+
        |blk1: () <-- (blk0)        |   |blk2: () <-- (blk0)        |
        |    v6:i32 = Iconst_32 0x0 |   |    Jump blk3, v2          |
        |    v7:i32 = Isub v6, v2   |   |                           |
        |    Jump blk3, v7          |   |                           |
        +---------------------------+   +---------------------------+
                      |                               |
                      |                               |
                      +-`{v5 := v7}`--+--`{v5 := v2}`-+
                                      |
                                      v
                      +------------------------------+
                      |blk3: (v5:i32) <-- (blk1,blk2)|
                      |    Jump blk_ret, v5          |
                      +------------------------------+
                                      |
                                 {return v5}
                                      |
                                      v
```

We use the ["block argument" variant of SSA][ssa-blocks], which is also the same
representation [used in LLVM's MLIR][llvm-mlir]. In this variant, each block
takes a list of arguments. Each block ends with a branching instruction (Branch, Return,
Jump, etc...) with an optional list of arguments; these arguments are assigned
to the target block's arguments like a function.

Consider the first block `blk0`.

```
blk0: (exec_ctx:i64, module_ctx:i64, v2:i32)
    v3:i32 = Iconst_32 0x0
    v4:i32 = Icmp lt_s, v2, v3
    Brz v4, blk2
    Jump blk1
```

You will notice that, compared to the original function, it takes two extra
parameters (`exec_ctx` and `module_ctx`):

1. `exec_ctx` is a pointer to `wazevo.executionContext`. This is used to exit the execution
   in the face of traps or for host function calls.
2. `module_ctx`: pointer to `wazevo.moduleContextOpaque`. This is used, among other things,
   to access memory.

It then takes one parameter `v2`, corresponding to the function parameter, and
it defines two variables `v3`, `v4`. `v3` is the constant 0, `v4` is the result of
comparing `v2` to `v3` using the `i32.lt_s` instruction. Then, it branches to
`blk2` if `v4` is zero, otherwise it jumps to `blk1`.

You might also have noticed that the instructions do not correspond strictly to
the original Wasm opcodes. This is because, similarly to the wazero IR used by
the old compiler, this is a custom IR.

You will also notice that, _on the right-hand side of the assignments_ of any statement,
no name occurs _twice_: this is why this form is called **single-assignment**.

Finally, notice how `blk1` and `blk2` end with a jump to the last block `blk3`.

```
blk1: ()
    ...
	Jump blk3, v7

blk2: ()
	Jump blk3, v2

blk3: (v5:i32)
    ...
```

`blk3` takes an argument `v5`: `blk1` jumps to `bl3` with `v7` and `blk2` jumps
to `blk3` with `v2`, meaning `v5` is effectively a rename of `v5` or `v7`,
depending on the originating block. If you are familiar with the traditional
representation of an SSA form, you will recognize that the role of block
arguments is equivalent to the role of the *Phi (Φ) function*, a special
function that returns a different value depending on the incoming edge; e.g., in
this case: `v5 := Φ(v7, v2)`.

### Code

The relevant APIs can be found under sub-package `ssa` and `frontend`.
In the code, the terms *lower* or *lowering* are often used to indicate a mapping or a translation,
because such transformations usually correspond to targeting a lower abstraction level.

- Basic Blocks are represented by the type `ssa.Block`.
- The SSA form is constructed using an `ssa.Builder`. The `ssa.Builder` is instantiated
  in the context of `wasm.Engine.CompileModule()`, more specifically in the method
  `frontend.Compiler.LowerToSSA()`.
- The mapping between Wasm opcodes and the IR happens in `frontend/lower.go`,
  more specifically in the method `frontend.Compiler.lowerCurrentOpcode()`.
- Because they are semantically equivalent, in the code, basic block parameters
  are sometimes referred to as "Phi values".

#### Instructions and Values

An `ssa.Instruction` is a single instruction in the SSA form. Each instruction might
consume zero or more `ssa.Value`s, and it usually produces a single `ssa.Value`; some
instructions may not produce any value (for instance, a `Jump` instruction).
An `ssa.Value` is an abstraction that represents a typed name binding, and it is used
to represent the result of an instruction, or the input to an instruction.

For instance:

```
blk1: () <-- (blk0)
    v6:i32 = Iconst_32 0x0
    v7:i32 = Isub v6, v2
    Jump blk3, v7
```

`Iconst_32` takes no input value and produce value `v6`; `Isub` takes two input values (`v6`, `v2`)
and produces value `v7`; `Jump` takes one input value (`v7`) and produces no value. All
such values have the `i32` type. The wazero SSA's type system (`ssa.Type`) allows the following types:

- `i32`: 32-bit integer
- `i64`: 64-bit integer
- `f32`: 32-bit floating point
- `f64`: 64-bit floating point
- `v128`: 128-bit SIMD vector

For simplicity, we don't have a dedicated type for pointers. Instead, we use the `i64`
type to represent pointer values since we only support 64-bit architectures,
unlike traditional compilers such as LLVM.

Values and instructions are both allocated from pools to minimize memory allocations.

### Debug Flags

- `wazevoapi.PrintSSA` dumps the SSA form to the console.
- `wazevoapi.FrontEndLoggingEnabled` dumps progress of the translation between Wasm
  opcodes and SSA instructions to the console.

## Optimization

The SSA form makes it easier to perform a number of optimizations. For instance,
we can perform constant propagation, dead code elimination, and common
subexpression elimination. These optimizations either act upon the instructions
within a basic block, or they act upon the control-flow graph as a whole.

On a high, level, consider the following basic block, derived from the previous
example:

```
blk0: (exec_ctx:i64, module_ctx:i64)
    v2:i32 = Iconst_32 -5
    v3:i32 = Iconst_32  0
    v4:i32 = Icmp lt_s, v2, v3
    Brz v4, blk2
    Jump blk1
```

It is pretty easy to see that the comparison in `v4` can be replaced by a
constant `1`, because the comparison is between two constant values (-5, 0).
Therefore, the block can be rewritten as such:

```
blk0: (exec_ctx:i64, module_ctx:i64)
    v4:i32 = Iconst_32 1
    Brz v4, blk2
    Jump blk1
```

However, we can now also see that the branch is always taken, and that the block
`blk2` is never executed, so even the branch instruction and the constant
definition `v4` can be removed:

```
blk0: (exec_ctx:i64, module_ctx:i64)
    Jump blk1
```

This is a simple example of constant propagation and dead code elimination
occurring within a basic block. However, now  `blk2` is unreachable, because
there is no other edge in the edge that points to it; thus it can be removed
from the control-flow graph. This is an example of dead-code elimination that
occurs at the control-flow graph level.

In practice, because WebAssembly is a compilation target, these simple
optimizations are often unnecessary. The optimization passes implemented in
wazero are also work-in-progress and, at the time of writing, further work is
expected to implement more advanced optimizations.

### Code

Optimization passes are implemented by `ssa.Builder.RunPasses()`. An optimization
pass is just a function that takes a ssa builder as a parameter.

Passes iterate over the basic blocks, and, for each basic block, they iterate
over the instructions. Each pass may mutate the basic block by modifying the instructions
it contains, or it might change the entire shape of the control-flow graph (e.g. by removing
blocks).

Currently, there are two dead-code elimination passes:

- `passDeadBlockEliminationOpt` acting at the block-level.
- `passDeadCodeEliminationOpt` acting at instruction-level.

Notably, `passDeadCodeEliminationOpt` also assigns an `InstructionGroupID` to each
instruction. This is used to determine whether a sequence of instructions can be
replaced by a single machine instruction during the back-end phase. For more details,
see also the relevant documentation in `ssa/instructions.go`

There are also simple constant folding passes such as `passNopInstElimination`, which
folds and delete instructions that are essentially no-ops (e.g. shifting by a 0 amount).

### Debug Flags

`wazevoapi.PrintOptimizedSSA` dumps the SSA form to the console after optimization.


## Block Layout

As we have seen earlier, the SSA form instructions are contained within basic
blocks, and the basic blocks are connected by edges of the control-flow graph.
However, machine code is not laid out in a graph, but it is just a linear
sequence of instructions.

Thus, the last step of the front-end is to lay out the basic blocks in a linear
sequence. Because each basic block, by design, ends with a control-flow
instruction, one of the goals of the block layout phase is to maximize the number of
**fall-through opportunities**. A fall-through opportunity occurs when a block ends
with a jump instruction whose target is exactly the next block in the
sequence. In order to maximize the number of fall-through opportunities, the
block layout phase might reorder the basic blocks in the control-flow graph,
and transform the control-flow instructions. For instance, it might _invert_
some branching conditions.

The end goal is to effectively minimize the number of jumps and branches in
the machine code that will be generated later.


### Critical Edges

Special attention must be taken when a basic block has multiple predecessors,
i.e., when it has multiple incoming edges. In particular, an edge between two
basic blocks is called a **critical edge** when, at the same time:
- the predecessor has multiple successors **and**
- the successor has multiple predecessors.

For instance, in the example below the edge between `BB0` and `BB3`
is a critical edge.

```goat { width="300" }
┌───────┐    ┌───────┐
│  BB0  │━┓  │  BB1  │
└───────┘ ┃  └───────┘
    │     ┃      │
    ▼     ┃      ▼
┌───────┐ ┃  ┌───────┐
│  BB2  │ ┗━▶│  BB3  │
└───────┘    └───────┘
```

In these cases the critical edge is split by introducing a new basic block,
called a **trampoline**, where the critical edge was.

```goat  { width="300" }
┌───────┐            ┌───────┐
│  BB0  │──────┐     │  BB1  │
└───────┘      ▼     └───────┘
    │    ┌──────────┐    │
    │    │trampoline│    │
    ▼    └──────────┘    ▼
┌───────┐      │     ┌───────┐
│  BB2  │      └────▶│  BB3  │
└───────┘            └───────┘
```

For more details on critical edges read more at

- https://en.wikipedia.org/wiki/Control-flow_graph
- https://nickdesaulniers.github.io/blog/2023/01/27/critical-edge-splitting/

### Example

At the end of the block layout phase, the laid out SSA for the `abs` function
looks as follows:

```
blk0: (exec_ctx:i64, module_ctx:i64, v2:i32)
	v3:i32 = Iconst_32 0x0
	v4:i32 = Icmp lt_s, v2, v3
	Brz v4, blk2
	Jump fallthrough

blk1: () <-- (blk0)
	v6:i32 = Iconst_32 0x0
	v7:i32 = Isub v6, v2
	Jump blk3, v7

blk2: () <-- (blk0)
	Jump fallthrough, v2

blk3: (v5:i32) <-- (blk1,blk2)
	Jump blk_ret, v5
```

### Code

`passLayoutBlocks` implements the block layout phase.

### Debug Flags

- `wazevoapi.PrintBlockLaidOutSSA` dumps the SSA form to the console after block layout.
- `wazevoapi.SSALoggingEnabled` logs the transformations that are applied during this phase,
  such as inverting branching conditions or splitting critical edges.

<hr>

* Previous Section: [How the Optimizing Compiler Works](../)
* Next Section: [Back-End](../backend/)

[ssa-blocks]: https://en.wikipedia.org/wiki/Static_single-assignment_form#Block_arguments
[llvm-mlir]: https://mlir.llvm.org/docs/Rationale/Rationale/#block-arguments-vs-phi-nodes
