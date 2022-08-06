package gojs

import (
	"context"

	"github.com/tetratelabs/wazero/api"
)

var (
	// jsDateConstructor returns jsDate.
	//
	// This is defined as `Get("Date")` in zoneinfo_js.go time.initLocal
	jsDateConstructor = newJsVal(refJsDateConstructor, "Date")

	// jsDate is used inline in zoneinfo_js.go for time.initLocal.
	// `.Call("getTimezoneOffset").Int()` returns a timezone offset.
	jsDate = newJsVal(refJsDate, "jsDate").
		addFunction("getTimezoneOffset", &getTimezoneOffset{})
)

type getTimezoneOffset struct{}

// invoke implements jsFn.invoke
func (*getTimezoneOffset) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	return uint32(0), nil // UTC
}
