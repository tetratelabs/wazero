package gojs

import (
	"context"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/gojs/goos"
)

var (
	// jsDateConstructor returns jsDate.
	//
	// This is defined as `Get("Date")` in zoneinfo_js.go time.initLocal
	jsDateConstructor = newJsVal(goos.RefJsDateConstructor, "Date")

	// jsDate is used inline in zoneinfo_js.go for time.initLocal.
	// `.Call("getTimezoneOffset").Int()` returns a timezone offset.
	jsDate = newJsVal(goos.RefJsDate, "jsDate").
		addFunction("getTimezoneOffset", &getTimezoneOffset{})
)

type getTimezoneOffset struct{}

// invoke implements jsFn.invoke
func (*getTimezoneOffset) invoke(context.Context, api.Module, ...interface{}) (interface{}, error) {
	return uint32(0), nil // UTC
}
