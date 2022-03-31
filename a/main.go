package main

import (
	"fmt"

	"github.com/tetratelabs/wazero"
)

// main is a simple WebAssembly program to showcase the packaging simplicity of wazero.
// This works because https://github.com/tetratelabs/wazero doesn't require CGO.
//
// Given a go.mod like so:
//
//	module test
//	go 1.18
//
// And a Dockerfile like so:
//
//	FROM scratch
//	COPY wazero /wazero
//	ENTRYPOINT ["/wazero"]
//
// Build like this:
//
//	go get github.com/tetratelabs/wazero@main
//	GOOS=linux go build -o wazero main.go
//
// Then, package like this:
//
//	docker build --platform linux/amd64 -t wazero .
//
// Finally, you end up with about 5MB even with an embedded JIT compiler!
//
//	docker run --rm wazero 1 2
//	docker history wazero:latest
func main() {
	source := []byte(`(module $test
    (func $addInt
        (param $value_1 i32) (param $value_2 i32)
        (result i32)
        local.get 0
        local.get 1
        i32.add
    )
    (export "AddInt" (func $addInt))
)`)

	// Instantiate the module and return its exported functions
	module, _ := wazero.NewRuntime().InstantiateModuleFromSource(source)

	// Discover 1 + 2 = 3
	fmt.Println(module.ExportedFunction("AddInt").Call(nil, 1, 2))
}
