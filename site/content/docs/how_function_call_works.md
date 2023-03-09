# How function call works

This document explains how wazero performs function call to an exported Wasm function in wazero at the higher level,
and what kind of role "engines" play in that regard. For more general information on architecture, etc, please refer
to [Docs](..).

Note that in this document, we focus on Compiler Runtime, not Interpreter for simplicity.

## Engines

As you can find in the [Doc](..), there's a concept called "engine" in wazero's code base. More precisely,
there are three types of engines, `Engine`, `ModuleEngine` and `CallEngine`. Each of them has a different scope and role:

- `Engine` has the same lifetime as `Runtinme`,
and in charge of compiling a [valid][valid] Wasm module into the native machine code, memory-mapping it as an executable,
and caching it in-memory and optionally [in files][file-cache]. It is used to create `ModuleEngine` for a given instantiated [Module][api-module].
- `ModuleEngine` has the same lifetime as the instantiated [Module][api-module]. It binds each [function instance][spec-function-instance] in a `Module` to
  the correct machine code executable created by `Engine`, and prepares the "virtual machine environment". `ModuleEngine` is used to
  create a `CallEngine` for a given exported function in the [Module][api-module].
- `CallEngine` has the same lifetime as the [api.Function][api-function], and corresponds to an exported function in a `Module`.
  It holds the data structure grouping the exported function instance and its corresponding machine code executable.
  This implements the [api.Function][api-function]'s `Call(...)` method, and has the unique [call stack][call-stack] which
  is used during the invocation of the corresponding function.

In short, each has the following scope:
- `Engine`: compile of a module.
- `ModuleEngine`: instantiation of a compiled module.
- `CallEngine`: invocation of an exported function.

The following diagram illustrates the relationship among these concepts:

```goat
      .-----------> Instantiated module                                 Exported Function
     /1:N                   |                                                  |
    /                       |                                                  v
   |     +----------+       v        +----------------+                  +------------+
   |     |  Engine  |--------------->|  ModuleEngine  |----------------->| CallEngine |
   |     +----------+                +----------------+                  +------------+
   |          |                               |                            |      |
   .          |                               |                            |      |
 foo.wasm --->|        .--------------------->|          '-----------------+      |
              |       /                       |          |                        |
              v      .                        v          v                        v
      +--------------+      +-----------------------------------+            +----------+
      | Machine Code |      |[(func_instance, machine_code),...]|            |Call Stack|
      +--------------+      +-----------------------------------+            +----------+
                                               ^                                  ^
                                               |                                  |
                                               |                                  |
                                               +----------------------------------+
                                                               |
                                                               |
                                                               |
                                                        Function.Call()
```

## The behavior of machine code

Due to wazero being CGO-less pure Go runtime, there's several constraints imposed on what we can do
within runtime-generated machine code execution in Go.

For example, the generated machine code must not manipulate Gorutine stack (or system stack) in general.
Otherwise, the Go runtime gets corrupted, and your program results in fatal error. One consequence is that
we cannot[^1] call Go functions (host functions) directly from machine code. Therefore, we employ the "trampoline" strategy to invoke
the imported Go functions: Instead of directly calling a Go function from the machine code, we exit the execution, and go back
to the caller (=usual Go function) of the machine code. Then, call the target Go function just like we do in Go programs,
and get results. Finally, we transfer the control to the machine code again, and resume the execution after the call instruction against the host function.

Another example is that, [we cannot safely modify the signal handler][signal-handler-discussion] of Go runtime.
This means that we cannot insert the custom signal handling logic at runtime. Therefore, in wazero, we always exit the execution of
machine code in case of [Wasm runtime traps][spec-trap] (e.g. [unreachable][spec-unreachable]) instead of using
signals unlike normal JIT compilers. And then, we raise the normal panic of Go with the proper status code.

For more details, see [RATIONALE.md][compiler-rationale].

## Journey of a function call

To better illustrate "how it works" of the previous section, let's consider the following TinyGo code:

```go
//go:import host tell_age
func host_tell_age(age int32) {}

// maybePanic panics when doPanic is true, no-op otherwise.
func maybePanic(doPanic bool) { if doPanic { panic("this is panic") } }

//export run
func run () {
	host_tell_age(15)
	maybePanic(false)
	maybePanic(true)
}
```

and the following is the diagram on what would happen when invoking the `run` function in wazero:

```goat
      |                                Go              |         Machine Code
      |                                                      (compiled from Wasm)
      |                                                |
      v
      |                   `Moudle.Export("run").Call()`|
      |                                |
      v                                v               |
      |                       +----------------+                +--------+
      |                       |func exec_native|-------|------> |func run|
      v                       +----------------+                +--------+
      |                                                |       /
      |         Go func call  +----------------+              /age=15
      v           .-----------|func exec_native|<------|-----. status=call_host_fn(name=tell_age)
      |          /   age=15   +----------------+     exit
      |         v                                      |
      v   +-------------+     +----------------+                +--------+
      |   |func tell_age|---->|func exec_native|-------|------> |func run|-----.
      |   +-------------+     +----------------+   continue     +--------+      \ doPanic=false
      v                                                |                         v
      |                                                         +--------+    +---------------+
      |                                                |        |func run|<---|func maybePanic|
      v                                                         +--------+    +---------------+
      |                                                |            |.
      |                                                             +-----------.
      |                                                |                        | doPanic=true
      v                                                                         v
      |                      +----------------+        |        exit          +---------------+
      |                      |func exec_native|<------------------------------|func maybePanic|
      v                      +----------------+        | status=unreachable   +---------------+
      |                              |
      |                              |                 |
      v                panic(WasmRuntimeErrUnreachable)
      |                                                |
      |
      v                                                |
```

where we start our journey from `Moudle.Export("run").Call()`. Internally, this creates a CallEngine, and start the invocation.
Then, we begin the execution of the function jumping into the machine code. When making a function call against hosst Go functions,
we exit the execution and return the control back to the `exec_native` function wit the proper parameters. Then, it executes the
target Go function on behalf of the machine code. (This is why we called it "trampoline" above). After executing a host function,
we continue the execution, and so on. You might notice that unction calls between Wasm functions are made fully in machine code world.
Finally, in the face of Wasm runtime errors, we exit the machine code execution with the proper status,
and return the control back to `exec_native` function, just like host function calls, and it will cause the regular Go panic with the
corresponding error.


[call-stack]: https://en.wikipedia.org/wiki/Call_stack
[valid]: https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#validation%E2%91%A1
[file-cache]: https://pkg.go.dev/github.com/tetratelabs/wazero@v1.0.0-rc.1#NewCompilationCacheWithDir
[api-function]: https://pkg.go.dev/github.com/tetratelabs/wazero@v1.0.0-rc.1/api#Function
[api-module]: https://pkg.go.dev/github.com/tetratelabs/wazero@v1.0.0-rc.1/api#Module
[spec-function-instance]: https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#function-instances%E2%91%A0
[spec-trap]: https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#trap
[spec-unreachable]: https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#syntax-instr-control
[compiler-rationale]: https://github.com/tetratelabs/wazero/blob/v1.0.0-rc.1/internal/engine/compiler/RATIONALE.md
[signal-handler-discussion]: https://gophers.slack.com/archives/C1C1YSQBT/p1675992411241409

[^1]: it's technically possible to call it directly, but that would come with performing "stack switching" in the native code.
  It's almost the same as what wazero does: exiting the execution of machine code, then call the target Go function (using the caller of machine code as a "trampoline").
