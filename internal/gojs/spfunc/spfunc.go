package spfunc

import (
	"errors"
	"fmt"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/wasm"
)

const debugMode = false

func MustCallFromSP(expectSP bool, proxied *wasm.HostFunc) *wasm.ProxyFunc {
	if ret, err := callFromSP(expectSP, proxied); err != nil {
		panic(err)
	} else {
		return ret
	}
}

// callFromSP generates code to call a function with the provided signature.
// The returned function has a single api.ValueTypeI32 parameter of SP. Each
// parameter is read at 8 byte offsets after that, and each result is written
// at 8 byte offsets after the parameters.
//
// # Parameters
//
//   - expectSP: true if a constructor or method invocation. The last result is
//     an updated SP value (i32), which affects the result memory offsets.
func callFromSP(expectSP bool, proxied *wasm.HostFunc) (*wasm.ProxyFunc, error) {
	params := proxied.ParamTypes
	results := proxied.ResultTypes
	if (8+len(params)+len(results))*8 > 255 {
		return nil, errors.New("TODO: memory offset larger than one byte")
	}

	if debugMode {
		fmt.Printf("(func $%s.proxy (param $%s %s)", proxied.Name, "sp", wasm.ValueTypeName(wasm.ValueTypeI32))
	}

	var localTypes []api.ValueType

	resultSpOffset := 8 + len(params)*8
	resultSPIndex := byte(0)
	if len(results) > 0 {
		if debugMode {
			fmt.Printf(" (local %s %s)", wasm.ValueTypeName(wasm.ValueTypeI32), wasm.ValueTypeName(wasm.ValueTypeI64))
		}
		localTypes = append(localTypes, api.ValueTypeI32, api.ValueTypeI64)
		if expectSP {
			if debugMode {
				fmt.Printf(" (local %s)", wasm.ValueTypeName(wasm.ValueTypeI32))
			}
			resultSPIndex = 3
			resultSpOffset += 8
			localTypes = append(localTypes, api.ValueTypeI32)
		}
	}

	// Load all parameters onto the stack.
	var code []byte
	for i, t := range params {
		if debugMode {
			fmt.Printf("\n;; param[%d]=%s\n", i, wasm.ValueTypeName(t))
		}

		// First, add the memory offset to load onto the stack.
		offset := 8 + int(i*8)
		code = compileAddOffsetToSP(code, 0, offset)

		// Next, load stack parameter $i from memory at that offset.
		switch t {
		case api.ValueTypeI32:
			if debugMode {
				fmt.Println(wasm.OpcodeI32LoadName)
			}
			code = append(code, wasm.OpcodeI32Load, 0x2, 0x0) // alignment=2 (natural alignment) staticOffset=0
		case api.ValueTypeI64:
			if debugMode {
				fmt.Println(wasm.OpcodeI64LoadName)
			}
			code = append(code, wasm.OpcodeI64Load, 0x3, 0x0) // alignment=3 (natural alignment) staticOffset=0
		default:
			panic(errors.New("TODO: param " + api.ValueTypeName(t)))
		}
	}

	// Now that all parameters are on the stack, call the function
	callFuncPos := len(code) + 1
	if debugMode {
		fmt.Printf("\n%s 0\n", wasm.OpcodeCallName)
	}

	// Call index zero is a placeholder as it is replaced later.
	code = append(code, wasm.OpcodeCall, 0)

	// The stack may now have results. Iterate backwards.
	i := len(results) - 1
	if expectSP {
		if debugMode {
			fmt.Printf("%s %d ;; refresh SP\n", wasm.OpcodeLocalSetName, resultSPIndex)
		}
		code = append(code, wasm.OpcodeLocalSet, resultSPIndex)
		i--
	}
	for ; i >= 0; i-- {
		// pop current result from stack
		t := results[i]
		if debugMode {
			fmt.Printf("\n;; result[%d]=%s\n", i, wasm.ValueTypeName(t))
		}

		var typeIndex byte
		switch t {
		case api.ValueTypeI32:
			typeIndex = 1
		case api.ValueTypeI64:
			typeIndex = 2
		default:
			panic(errors.New("TODO: result " + api.ValueTypeName(t)))
		}

		if debugMode {
			fmt.Printf("%s %d ;; next result\n", wasm.OpcodeLocalSetName, typeIndex)
		}
		code = append(code, wasm.OpcodeLocalSet, typeIndex)

		offset := resultSpOffset + i*8
		code = compileAddOffsetToSP(code, resultSPIndex, offset)

		if debugMode {
			fmt.Printf("%s %d ;; store next result\n", wasm.OpcodeLocalGetName, typeIndex)
		}
		code = append(code, wasm.OpcodeLocalGet, typeIndex)

		switch t {
		case api.ValueTypeI32:
			if debugMode {
				fmt.Println(wasm.OpcodeI32StoreName)
			}
			code = append(code, wasm.OpcodeI32Store, 0x2, 0x0) // alignment=2 (natural alignment) staticOffset=0
		case api.ValueTypeI64:
			if debugMode {
				fmt.Println(wasm.OpcodeI64StoreName)
			}
			code = append(code, wasm.OpcodeI64Store, 0x3, 0x0) // alignment=3 (natural alignment) staticOffset=0
		default:
			panic(errors.New("TODO: result " + api.ValueTypeName(t)))
		}

	}
	if debugMode {
		fmt.Println("\n)")
	}
	code = append(code, wasm.OpcodeEnd)
	return &wasm.ProxyFunc{
		Proxy: &wasm.HostFunc{
			ExportNames: proxied.ExportNames,
			Name:        proxied.Name + ".proxy",
			ParamTypes:  []api.ValueType{api.ValueTypeI32},
			ParamNames:  []string{"sp"},
			Code:        &wasm.Code{IsHostFunction: true, LocalTypes: localTypes, Body: code},
		},
		Proxied: &wasm.HostFunc{
			Name:        proxied.Name,
			ParamTypes:  proxied.ParamTypes,
			ResultTypes: proxied.ResultTypes,
			ParamNames:  proxied.ParamNames,
			Code:        proxied.Code,
		},
		CallBodyPos: callFuncPos,
	}, nil
}

func compileAddOffsetToSP(code []byte, spLocalIndex byte, offset int) []byte {
	if debugMode {
		fmt.Printf("%s %d ;; SP\n", wasm.OpcodeLocalGetName, spLocalIndex)
		fmt.Printf("%s %d ;; offset\n", wasm.OpcodeI32ConstName, offset)
		fmt.Printf("%s\n", wasm.OpcodeI32AddName)
	}
	code = append(code, wasm.OpcodeLocalGet, spLocalIndex)
	// See /RATIONALE.md we can't tell the signed interpretation of a constant, so default to signed.
	code = append(code, wasm.OpcodeI32Const)
	code = append(code, leb128.EncodeInt32(int32(offset))...)
	code = append(code, wasm.OpcodeI32Add)
	return code
}
