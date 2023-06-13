package wasi_http

import (
	"context"
	"io/ioutil"
	"net/http"

	"github.com/tetratelabs/wazero/api"
)

var response *http.Response

func SetResponse(r *http.Response) {
	response = r
}

func ResponseBody() ([]byte, error) {
	return ioutil.ReadAll(response.Body)
}

func ResponseHeaders() http.Header {
	return response.Header
}

var dropIncomingResponse = newHostMethod("drop-incoming-response", dropIncomingResponseFn, []api.ValueType{i32}, "a")

func dropIncomingResponseFn(_ context.Context, mod api.Module, params []uint64) {
	// pass
}

var incomingResponseStatus = newHostFunc("incoming-response-status", incomingResponseStatusFn, []api.ValueType{i32}, "a")

func incomingResponseStatusFn(_ context.Context, mod api.Module, params []uint64) int32 {
	return int32(response.StatusCode)
}

var incomingResponseHeaders = newHostFunc("incoming-response-headers", incomingResponseHeadersFn, []api.ValueType{i32}, "a")

func incomingResponseHeadersFn(_ context.Context, mod api.Module, params []uint64) int32 {
	return 1
}

var incomingResponseConsume = newHostMethod("incoming-response-consume", incomingResponseConsumeFn, []api.ValueType{i32, i32}, "a", "b")

func incomingResponseConsumeFn(_ context.Context, mod api.Module, params []uint64) {
	data := []byte{}
	// 0 == ok, 1 == is_err
	data = le.AppendUint32(data, 0)
	// This is the stream number
	data = le.AppendUint32(data, 1)
	mod.Memory().Write(uint32(params[1]), data)
}

var futureResponseGet = newHostMethod("future-incoming-response-get", futureResponseGetFn, []api.ValueType{i32, i32}, "a", "b")

func futureResponseGetFn(_ context.Context, mod api.Module, params []uint64) {
	data := []byte{}
	// 1 == is_some, 0 == none
	data = le.AppendUint32(data, 1)
	// 0 == ok, 1 == is_err, consistency ftw!
	data = le.AppendUint32(data, 0)
	// Copy the future into the actual
	data = le.AppendUint32(data, uint32(params[0]))
	mod.Memory().Write(uint32(params[1]), data)
}
