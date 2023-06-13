package default_http

import (
	"context"
	"log"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_http"
)

var request = newHostFunc("request", requestFn,
	[]api.ValueType{i32, i32, i32, i32, i32, i32, i32, i32, i32, i32, i32, i32, i32, i32},
	"a", "b", "c", "d", "e", "f", "g", "h", "j", "k", "l", "m", "n", "o")

func requestFn(_ context.Context, mod api.Module, params []uint64) int32 {
	return 0
}

var handle = newHostFunc("handle", handleFn,
	[]api.ValueType{i32, i32, i32, i32, i32, i32, i32, i32},
	"a", "b", "c", "d", "e", "f", "g", "h")

func handleFn(_ context.Context, mod api.Module, params []uint64) int32 {
	r, err := wasi_http.MakeRequest()
	if err != nil {
		log.Printf(err.Error())
		return 0
	}
	wasi_http.SetResponse(r)
	return 1
}
