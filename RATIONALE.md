# Notable rationale of wazero

## Project structure
wazero uses internal packages extensively to balance API compatability desires for end users with the need to safely
share internals between compilers.

End-user packages include `wazero`,with `Config` structs, and `api`, with shared types. Everything else is internal.

### Internal packages
Most code in wazero is internal, and it is acknowledged that this prevents external implementation of facets such as
compilers or decoding. It also prevents splitting this code into separate repositories, resulting in a larger monorepo.
This also adds work as more code needs to be centrally reviewed.

However, the alternative is neither secure nor viable. To allow external implementation would require exporting symbols
public, such as the `CodeSection`, which can easily create bugs. Moreover, there's a high drift risk for any attempt at
external implementations, compounded not just by wazero's code organization, but also the fast moving Wasm and WASI
specifications.

For example, implementing a compiler correctly requires expertise in Wasm, Golang and assembly. This requires deep
insight into how internals are meant to be structured and the various tiers of testing required for `wazero` to result
in a high quality experience. Even if someone had these skills, supporting external code would introduce variables which
are constants in the central one. Supporting an external codebase is harder on the project team, and could starve time
from the already large burden on the central codebase.

The tradeoffs of internal packages are a larger codebase and responsibility to implement all standard features. It also
implies thinking about extension more as forking is not viable for reasons above also. The primary mitigation of these
realities are friendly OSS licensing, high rigor and a collaborative spirit which aim to make contribution in the shared
codebase productive.

### Avoiding cyclic dependencies
wazero shares constants and interfaces with internal code by a sharing pattern described below:
* shared interfaces and constants go in one package under root: `api`.
* user APIs and structs depend on `api` and go into the root package `wazero`.
  * Ex. `InstantiateModule` -> `/wasm.go` depends on the type `api.Module`.
* implementation code can also depend on `api` in a corresponding package under `/internal`.
  * Ex  package `wasm` -> `/internal/wasm/*.go` and can depend on the type `api.Module`.

The above guarantees no cyclic dependencies at the cost of having to re-define symbols that exist in both packages.
For example, if `wasm.Store` is a type the user needs access to, it is narrowed by a cover type in the `wazero`:

```go
type Runtime struct {
	s *wasm.Store
}
```

This is not as bad as it sounds as mutations are only available via configuration. This means exported functions are
limited to only a few functions.

### Avoiding security bugs

In order to avoid security flaws such as code insertion, nothing in the public API is permitted to write directly to any
mutable symbol in the internal package. For example, the package `api` is shared with internal code. To ensure
immutability, the `api` package cannot contain any mutable public symbol, such as a slice or a struct with an exported
field.

In practice, this means shared functionality like memory mutation need to be implemented by interfaces.

Ex. `api.Memory` protects access by exposing functions like `WriteFloat64Le` instead of exporting a buffer (`[]byte`).
Ex. There is no exported symbol for the `[]byte` representing the `CodeSection`

Besides security, this practice prevents other bugs and allows centralization of validation logic such as decoding Wasm.

## Why does InstantiateModule call "_start" by default?
We formerly had functions like `StartWASICommand` that would verify preconditions and start WASI's "_start" command.
However, this caused confusion because both many languages compiled a WASI dependency, and many did so inconsistently.

That said, if "_start" isn't called, it causes issues in TinyGo, as it needs this in order to implement panic. To deal
with this a different way, we have a configuration to call any start functions that exist, which defaults to "_start".

## Runtime == Engine+Store
wazero defines a single user-type which combines the specification concept of `Store` with the unspecified `Engine`
which manages them.

### Why not multi-store?
Multi-store isn't supported as the extra tier complicates lifecycle and locking. Moreover, in practice it is unusual for
there to be an engine that has multiple stores which have multiple modules. More often, it is the case that there is
either 1 engine with 1 store and multiple modules, or 1 engine with many stores, each having 1 non-host module. In worst
case, a user can use multiple runtimes until "multi-store" is better understood.

If later, we have demand for multiple stores, that can be accomplished by overload. Ex. `Runtime.InstantiateInStore` or
`Runtime.Store(name) Store`.

## wazeroir
wazero's intermediate representation (IR) is called `wazeroir`. Compiling into an IR provides us a faster interpreter
and a building block for a future JIT compilation engine. Both of these help answer demands for a more performant
runtime vs interpreting Wasm directly (the `naivevm` interpreter).

### Intermediate Representation (IR) design
`wazeroir`'s initial design borrowed heavily from the defunct `microwasm` format (a.k.a. LightbeamIR). Notably,
`wazeroir` doesn't have block operations: this simplifies the implementation.

Note: `microwasm` was never specified formally, and only exists in a historical codebase of wasmtime:
https://github.com/bytecodealliance/wasmtime/blob/v0.29.0/crates/lightbeam/src/microwasm.rs

## WASI

Unfortunately, (WASI Snapshot Preview 1)[https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md] is not formally defined enough, and has APIs with ambiguous semantics.
This section describes how Wazero interprets and implements the semantics of several WASI APIs that may be interpreted differently by different wasm runtimes.
Those APIs may affect the portability of a WASI application.

### Why aren't WASI rules enforced?

The [snapshot-01](https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md) version of WASI has a
number of rules for a "command module", but only the memory export rule is enforced. If a "_start" function exists, it
is enforced to be the correct signature and succeed, but the export itself isn't enforced. It follows that this means
exports are not required to be contained to a "_start" function invocation. Finally, the "__indirect_function_table"
export is also not enforced.

The reason for the exceptions are that implementations aren't following the rules. For example, TinyGo doesn't export
"__indirect_function_table", so crashing on this would make wazero unable to run TinyGo modules. Similarly, modules
loaded by wapc-go don't always define a "_start" function. Since "snapshot-01" is not a proper version, and certainly
not a W3C recommendation, there's no sense in breaking users over matters like this.

### Why is I/O configuration not coupled to WASI?

WebAssembly System Interfaces (WASI) is a formalization of a practice that can be done anyway: Define a host function to
access a system interface, such as writing to STDOUT. WASI stalled at snapshot-01 and as of early 2022, is being
rewritten entirely.

This instability implies a need to transition between WASI specs, which places wazero in a position that requires
decoupling. For example, if code uses two different functions to call `fd_write`, the underlying configuration must be
centralized and decoupled. Otherwise, calls using the same file descriptor number will end up writing to different
places.

In short, wazero defined system configuration in `ModuleConfig`, not a WASI type. This allows end-users to switch from
one spec to another with minimal impact. This has other helpful benefits, as centralized resources are simpler to close
coherently (ex via `Module.Close`).

### Background on `ModuleConfig` design
WebAssembly 1.0 (20191205) specifies some aspects to control isolation between modules ([sandboxing](https://en.wikipedia.org/wiki/Sandbox_(computer_security))).
For example, `wasm.Memory` has size constraints and each instance of it is isolated from each other. While `wasm.Memory`
can be shared, by exporting it, it is not exported by default. In fact a WebAssembly Module (Wasm) has no memory by
default.

While memory is defined in WebAssembly 1.0 (20191205), many aspects are not. Let's use an example of `exec.Cmd` as for
example, a WebAssembly System Interfaces (WASI) command is implemented as a module with a `_start` function, and in many
ways acts similar to a process with a `main` function.

To capture "hello world" written to the console (stdout a.k.a. file descriptor 1) in `exec.Cmd`, you would set the
`Stdout` field accordingly, perhaps to a buffer. In WebAssembly 1.0 (20191205), the only way to perform something like
this is via a host function (ex `ModuleBuilder.ExportFunction`) and internally copy memory corresponding to that string
to a buffer.

WASI implements system interfaces with host functions. Concretely, to write to console, a WASI command `Module` imports
"fd_write" from "wasi_snapshot_preview1" and calls it with the `fd` parameter set to 1 (STDOUT).

The [snapshot-01](https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md) version of WASI has no
means to declare configuration, although its function definitions imply configuration for example if fd 1 should exist,
and if so where should it write. Moreover, snapshot-01 was last updated in late 2020 and the specification is being
completely rewritten as of early 2022. This means WASI as defined by "snapshot-01" will not clarify aspects like which
file descriptors are required. While it is possible a subsequent version may, it is too early to tell as no version of
WASI has reached a stage near W3C recommendation. Even if it did, module authors are not required to only use WASI to
write to console, as they can define their own host functions, such as they did before WASI existed.

wazero aims to serve Go developers as a primary function, and help them transition between WASI specifications. In
order to do this, we have to allow top-level configuration. To ensure isolation by default, `ModuleConfig` has WithXXX
that override defaults to no-op or empty. One `ModuleConfig` instance is used regardless of how many times the same WASI
functions are imported. The nil defaults allow safe concurrency in these situations, as well lower the cost when they
are never used. Finally, a one-to-one mapping with `Module` allows the module to close the `ModuleConfig` instead of
confusing users with another API to close.

Naming, defaults and validation rules of aspects like `STDIN` and `Environ` are intentionally similar to other Go
libraries such as `exec.Cmd` or `syscall.SetEnv`, and differences called out where helpful. For example, there's no goal
to emulate any operating system primitive specific to Windows (such as a 'c:\' drive). Moreover, certain defaults
working with real system calls are neither relevant nor safe to inherit: For example, `exec.Cmd` defaults to read STDIN
from a real file descriptor ("/dev/null"). Defaulting to this, vs reading `io.EOF`, would be unsafe as it can exhaust
file descriptors if resources aren't managed properly. In other words, blind copying of defaults isn't wise as it can
violate isolation or endanger the embedding process. In summary, we try to be similar to normal Go code, but often need
act differently and document `ModuleConfig` is more about emulating, not necessarily performing real system calls.

### FdPrestatDirName

`FdPrestatDirName` is a WASI function to return the path of the pre-opened directory of a file descriptor.
It has the following three parameters, and the third `pathLen` has ambiguous semantics.

- `fd` - a file descriptor
- `path` - the offset for the result path
- `pathLen` - In wazero, `FdPrestatDirName` writes the result path string to `path` offset for the exact length of `pathLen`.

Wasmer considers `pathLen` to be the maximum length instead of the exact length that should be written.
See https://github.com/wasmerio/wasmer/blob/3463c51268ed551933392a4063bd4f8e7498b0f6/lib/wasi/src/syscalls/mod.rs#L764

The semantics in wazero follows that of wasmtime.
See https://github.com/bytecodealliance/wasmtime/blob/2ca01ae9478f199337cf743a6ab543e8c3f3b238/crates/wasi-common/src/snapshots/preview_1.rs#L578-L582

Their semantics match when `pathLen` == the length of `path`, so in practice this difference won't matter match.

## Implementation limitations

WebAssembly 1.0 (20191205) specification allows runtimes to [limit certain aspects of Wasm module or execution](https://www.w3.org/TR/2019/REC-2019/REC-wasm-core-1-20191205/#a2-implementation-limitations).

wazero limitations are imposed pragmatically and described below.

### Number of functions in a module

The possible number of function instances in [a module](https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#module-instances%E2%91%A0) is not specified in the WebAssembly specifications since [`funcaddr`](https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#syntax-funcaddr) corresponding to a function instance in a store can be arbitrary number.
wazero limits the maximum function instances to 2^27 as even that number would occupy 1GB in function pointers.

That is because not only we _believe_ that all use cases are fine with the limitation, but also we have no way to test wazero runtimes under these unusual circumstances.

### Number of function types in a store

There's no limitation on the number of function types in [a store](https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#store%E2%91%A0) according to the spec. In wazero implementation, we assign each function type to a unique ID, and choose to use `uint32` to represent the IDs.
Therefore the maximum number of function types a store can have is limited to 2^27 as even that number would occupy 512MB just to reference the function types.

This is due to the same reason for the limitation on the number of functions above.

### Number of values on the stack in a function

While the the spec does not clarify a limitation of function stack values, wazero limits this to 2^27 = 134,217,728.
The reason is that we internally represent all the values as 64-bit integers regardless of its types (including f32, f64), and 2^27 values means
1 GiB = (2^30). 1 GiB is the reasonable for most applications [as we see a Goroutine has 250 MB as a limit on the stack for 32-bit arch](https://github.com/golang/go/blob/f296b7a6f045325a230f77e9bda1470b1270f817/src/runtime/proc.go#L120), considering that WebAssembly is (currently) 32-bit environment.

All the functions are statically analyzed at module instantiation phase, and if a function can potentially reach this limit, an error is returned.

### Number of globals in a module

Theoretically, a module can declare globals (including imports) up to 2^32 times. However, wazero limits this to  2^27(134,217,728) per module.
That is because internally we store globals in a slice with pointer types (meaning 8 bytes on 64-bit platforms), and therefore 2^27 globals
means that we have 1 GiB size of slice which seems large enough for most applications.

## JIT engine implementation

See [wasm/jit/RATIONALE.md](internal/wasm/jit/RATIONALE.md).

## Golang patterns

### Hammer tests
Code that uses concurrency primitives, such as locks or atomics, should include "hammer tests", which run large loops
inside a bounded amount of goroutines, run by half that many `GOMAXPROCS`. These are named consistently "hammer", so
they are easy to find. The name inherits from some existing tests in [golang/go](https://github.com/golang/go/search?q=hammer&type=code).

Here is an annotated description of the key pieces of a hammer test:
1. `P` declares the count of goroutines to use, defaulting to 8 or 4 if `testing.Short`.
   * Half this amount are the cores used, and 4 is less than a modern laptop's CPU. This allows multiple "hammer" tests to run in parallel.
2. `N` declares the scale of work (loop) per goroutine, defaulting to value that finishes in ~0.1s on a modern laptop.
   * When in doubt, try 1000 or 100 if `testing.Short`
   * Remember, there are multiple hammer tests and CI nodes are slow. Slower tests hurt feedback loops.
3. `defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(P/2))` makes goroutines switch cores, testing visibility of shared data.
4. To ensure goroutines execute at the same time, block them with `sync.WaitGroup`, initialized to `Add(P)`.
   * `sync.WaitGroup` internally uses `runtime_Semacquire` not available in any other library.
   * `sync.WaitGroup.Add` with a negative value can unblock many goroutines at the same time, e.g. without a for loop.
5. Track goroutines progress via `finished := make(chan int)` where each goroutine in `P` defers `finished <- 1`.
   1. Tests use `require.XXX`, so `recover()` into `t.Fail` in a `defer` function before `finished <- 1`.
      * This makes it easier to spot larger concurrency problems as you see each failure, not just the first.
   2. After the `defer` function, await unblocked, then run the stateful function `N` times in a normal loop.
      * This loop should trigger shared state problems as locks or atomics are contended by `P` goroutines.
6. After all `P` goroutines launch, atomically release all of them with `WaitGroup.Add(-P)`.
7. Block the runner on goroutine completion, by (`<-finished`) for each `P`.
8. When all goroutines complete, `return` if `t.Failed()`, otherwise perform follow-up state checks.

This is implemented in wazero in [hammer.go](internal/testing/hammer/hammer.go)

### Lock-free, cross-goroutine observations of updates

How to achieve cross-goroutine reads of a variable are not explicitly defined in https://go.dev/ref/mem. wazero uses
atomics to implement this following unofficial practice. For example, a `Close` operation can be guarded to happen only
once via compare-and-swap (CAS) against a zero value. When we use this pattern, we consistently use atomics to both
read and update the same numeric field.

In lieu of formal documentation, we infer this pattern works from other sources (besides tests):
 * `sync.WaitGroup` by definition must support calling `Add` from other goroutines. Internally, it uses atomics.
 * rsc in golang/go#5045 writes "atomics guarantee sequential consistency among the atomic variables".

See https://github.com/golang/go/blob/011fd002457da0823da5f06b099fcf6e21444b00/src/sync/waitgroup.go#L64
See https://github.com/golang/go/issues/5045#issuecomment-252730563
See https://www.youtube.com/watch?v=VmrEG-3bWyM
