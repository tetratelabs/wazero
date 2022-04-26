## Function listener

This example shows how to define a function listener to trace function calls. Function listeners are currently
only implemented for interpreter mode.

### Background

As WebAssembly has become a target bytecode for many different languages and runtimes, we end up with binaries
that encode the semantics and nuances for various frontends such as TinyGo and Rust, all in the single large
binary. One line of code in a frontend language may result in working through many functions, possibly through
host functions exposed with WASI. [print-trace.go](print-trace.go) shows how to use a FunctionListenerFactory to
listen to all function invocations in the program. This can be used to find details about the execution of a
wasm program, which can otherwise be a blackbox cobbled together by a frontend compiler.
