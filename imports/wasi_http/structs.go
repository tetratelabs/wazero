package wasi_http

import (
	"context"
	"log"

	"github.com/tetratelabs/wazero/api"
)

var newFields = newHostFunc("new-fields", newFieldsFn, []api.ValueType{i32, i32}, "a", "b")

func newFieldsFn(_ context.Context, mod api.Module, params []uint64) int32 {
	return 0
}

func allocateWriteString(ctx context.Context, m api.Module, s string) uint32 {
	ptr, err := Malloc(ctx, m, uint32(len(s)))
	if err != nil {
		log.Fatalf(err.Error())
	}
	m.Memory().Write(ptr, []byte(s))
	return ptr
}

var fieldsEntries = newHostMethod("fields-entries", fieldsEntriesFn, []api.ValueType{i32, i32}, "a", "b")

func fieldsEntriesFn(ctx context.Context, mod api.Module, params []uint64) {
	headers := ResponseHeaders()
	l := uint32(len(headers))
	// 8 bytes per string/string
	ptr, err := Malloc(ctx, mod, l*16)
	if err != nil {
		log.Fatalf(err.Error())
	}
	data := []byte{}
	data = le.AppendUint32(data, ptr)
	data = le.AppendUint32(data, l)
	// write result
	mod.Memory().Write(uint32(params[1]), data)

	// ok now allocate and write the strings.
	data = []byte{}
	for k, v := range headers {
		data = le.AppendUint32(data, allocateWriteString(ctx, mod, k))
		data = le.AppendUint32(data, uint32(len(k)))
		data = le.AppendUint32(data, allocateWriteString(ctx, mod, v[0]))
		data = le.AppendUint32(data, uint32(len(v[0])))
	}
	mod.Memory().Write(ptr, data)
}
