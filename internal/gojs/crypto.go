package gojs

import (
	"context"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/gojs/goos"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// jsCrypto gets random values.
//
// It has only one invocation pattern:
//
//	jsCrypto.Call("getRandomValues", a /* uint8Array */)
//
// This is defined as `Get("crypto")` in rand_js.go init
var jsCrypto = newJsVal(goos.RefJsCrypto, "crypto").
	addFunction("getRandomValues", &getRandomValues{})

type getRandomValues struct{}

// invoke implements jsFn.invoke
func (*getRandomValues) invoke(_ context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	randSource := mod.(*wasm.CallContext).Sys.RandSource()

	r := args[0].(*byteArray)
	n, err := randSource.Read(r.slice)
	return uint32(n), err
}
