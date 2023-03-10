# How do compiler functions work?

WebAssembly runtimes let you call functions defined in wasm. How this works in
wazero is different depending on your `RuntimeConfig`.

* `RuntimeConfigCompiler` compiles machine code from your wasm, and jumps to
  that when invoking a function.
* `RuntimeConfigInterpreter` does not generate code. It interprets wasm and
  executes go statements that correspond to WebAssembly instructions.

How the compiler works precisely is a large topic, and discussed at length on
this page. For more general information on architecture, etc., please refer to
[Docs](..).

## Engines

Our [Docs](..) introduce the "engine" concept of wazero. More precisely, there
are three types of engines, `Engine`, `ModuleEngine` and `CallEngine`. Each has
a different scope and role:

- `Engine` has the same lifetime as `Runtime`. This compiles a `CompiledModule`
  into machine code, which is both cached and memory-mapped as an executable.
- `ModuleEngine` is a virtual machine with the same lifetime as its [Module][api-module].
  Notably, this binds each [function instance][spec-function-instance] to
  corresponding machine code owned by its `Engine`.
- `CallEngine` corresponds to an exported [api.Function][api-function] in a
  [Module][api-module]. This implements `Function.Call(...)` by invoking
  machine code corresponding to a function instance in `ModuleEngine` and
  managing the [call stack][call-stack] representing the invocation.

Here is a diagram showing the relationships of these engines:

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

Go source can be compiled to invoke native library functions using CGO.
However, [CGO is not GO][cgo-not-go]. To call native functions in pure Go, we
need a different approach with unique constraints.

For example, the generated machine code must not manipulate Goroutine
(or system) stack. Otherwise, the Go runtime gets corrupted, which results in
fatal execution errors. This means we cannot[^1] call Go functions (host
functions) directly from machine code (compiled from wasm). This is routinely
needed in WebAssembly, as system calls such as WASI are defined in Go, but
invoked from Wasm.

To handle this, we employ a "trampoline" strategy. Let's explain this with an
example. `clock_time_get`, is a host function defined in Go, called from
machine code compiled from guest wasm. Let's say the wasm function is named
`getTime`.

When `getTime` calls  `clock_time_get`, it actually exits execution first.
wazero then calls the Go function mapped to `clock_time_get` like a usual Go
program. Finally, wazero transfers control back to machine code again, resuming
execution after the `clock_time_get` call instruction.

TODO: add a diagram

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
[api-function]: https://pkg.go.dev/github.com/tetratelabs/wazero@v1.0.0-rc.1/api#Function
[api-module]: https://pkg.go.dev/github.com/tetratelabs/wazero@v1.0.0-rc.1/api#Module
[spec-function-instance]: https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#function-instances%E2%91%A0
[spec-trap]: https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#trap
[spec-unreachable]: https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#syntax-instr-control
[compiler-rationale]: https://github.com/tetratelabs/wazero/blob/v1.0.0-rc.1/internal/engine/compiler/RATIONALE.md
[signal-handler-discussion]: https://gophers.slack.com/archives/C1C1YSQBT/p1675992411241409
[cgo-not-go]: https://www.youtube.com/watch?v=PAAkCSZUG1c&t=757s

[^1]: it's technically possible to call it directly, but that would come with performing "stack switching" in the native code.
  It's almost the same as what wazero does: exiting the execution of machine code, then call the target Go function (using the caller of machine code as a "trampoline").
