package wasi_streams

import (
	"context"
	"log"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_http"
)

var streamsRead = newHostMethod("read", streamReadFn, []api.ValueType{i32, i64, i32}, "a", "b", "c")

func streamReadFn(ctx context.Context, mod api.Module, params []uint64) {
	data, err := wasi_http.ResponseBody()
	if err != nil {
		log.Fatalf(err.Error())
	}
	ptr_len := uint32(len(data))
	data = append(data, 0)
	ptr, err := wasi_http.Malloc(ctx, mod, ptr_len)
	if err != nil {
		log.Fatalf(err.Error())
	}
	mod.Memory().Write(ptr, data)

	data = []byte{}
	// 0 == is_ok, 1 == is_err
	data = le.AppendUint32(data, 0)
	data = le.AppendUint32(data, ptr)
	data = le.AppendUint32(data, ptr_len)
	// No more data to read.
	data = le.AppendUint32(data, 0)
	mod.Memory().Write(uint32(params[2]), data)
}

var dropInputStream = newHostMethod("drop-input-stream", dropInputStreamFn, []api.ValueType{i32}, "a")

func dropInputStreamFn(_ context.Context, mod api.Module, params []uint64) {
	// pass
}

var writeStream = newHostMethod("write", writeStreamFn, []api.ValueType{i32, i32, i32, i32}, "a", "b", "c", "d")

func writeStreamFn(_ context.Context, mod api.Module, params []uint64) {
	// pass
}
