package wasi_http

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/tetratelabs/wazero/api"
)

var newOutgoingRequest = newHostFunc("new-outgoing-request", newOutgoingRequestFn,
	[]api.ValueType{i32, i32, i32, i32, i32, i32, i32, i32, i32, i32, i32, i32, i32, i32},
	"a", "b", "c", "d", "e", "f", "g", "h", "j", "k", "l", "m", "n", "o")

type Request struct {
	Method    string
	Path      string
	Query     string
	Scheme    string
	Authority string
	Body      io.Reader
}

func (r Request) Url() string {
	return fmt.Sprintf("%s://%s%s%s", r.Scheme, r.Authority, r.Path, r.Query)
}

var request = Request{}

func MakeRequest() (*http.Response, error) {
	r, err := http.NewRequest(request.Method, request.Url(), request.Body)
	if err != nil {
		return nil, err
	}

	return http.DefaultClient.Do(r)
}

func RequestBody(body []byte) {
	request.Body = bytes.NewBuffer(body)
}

func newOutgoingRequestFn(_ context.Context, mod api.Module, params []uint64) int32 {
	switch params[0] {
	case 0:
		request.Method = "GET"
	case 1:
		request.Method = "HEAD"
	case 2:
		request.Method = "POST"
	case 3:
		request.Method = "PUT"
	case 4:
		request.Method = "DELETE"
	case 5:
		request.Method = "CONNECT"
	case 6:
		request.Method = "OPTIONS"
	case 7:
		request.Method = "TRACE"
	case 8:
		request.Method = "PATCH"
	default:
		log.Fatalf("Unknown method: %d", params[0])
	}

	path_ptr := params[3]
	path_len := params[4]
	path, ok := mod.Memory().Read(uint32(path_ptr), uint32(path_len))
	if !ok {
		return 0
	}
	request.Path = string(path)

	query_ptr := params[5]
	query_len := params[6]
	query, ok := mod.Memory().Read(uint32(query_ptr), uint32(query_len))
	if !ok {
		return 0
	}
	request.Query = string(query)

	scheme_is_some := params[7]
	request.Scheme = "https"
	if scheme_is_some == 1 {
		if params[8] == 0 {
			request.Scheme = "http"
		}
	}

	authority_ptr := params[11]
	authority_len := params[12]
	authority, ok := mod.Memory().Read(uint32(authority_ptr), uint32(authority_len))
	if !ok {
		return 0
	}
	request.Authority = string(authority)
	log.Printf("%s %s\n", request.Method, request.Url())
	return 2
}

var dropOutgoingRequest = newHostMethod("drop-outgoing-request", dropOutgoingRequestFn, []api.ValueType{i32}, "a")

func dropOutgoingRequestFn(_ context.Context, mod api.Module, params []uint64) {
	// pass
}

var outgoingRequestWrite = newHostMethod("outgoing-request-write", outgoingRequestWriteFn, []api.ValueType{i32, i32}, "a", "b")

func outgoingRequestWriteFn(_ context.Context, mod api.Module, params []uint64) {

}
