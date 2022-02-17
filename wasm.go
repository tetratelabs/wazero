package wazero

import (
	internalwasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/binary"
	"github.com/tetratelabs/wazero/internal/wasm/text"
)

// DecodeModule parses the configured source into a Module. This function returns when the source is exhausted or
// an error occurs. The result can be initialized for use via Store.Instantiate.
//
// Here's a description of the return values:
// * result is the module parsed or nil on error
// * err is a FormatError invoking the parser, dangling block comments or unexpected characters.
// See binary.DecodeModule and text.DecodeModule
type DecodeModule func(source []byte) (result *Module, err error)

var DecodeModuleBinary DecodeModule = func(source []byte) (*Module, error) {
	return decodeModule(binary.DecodeModule, source)
}

var DecodeModuleText DecodeModule = func(source []byte) (*Module, error) {
	return decodeModule(text.DecodeModule, source)
}

func decodeModule(decoder internalwasm.DecodeModule, source []byte) (*Module, error) {
	m, err := decoder(source)
	if err != nil {
		return nil, err
	}
	var name string
	if m.NameSection != nil {
		name = m.NameSection.ModuleName
	}
	return &Module{m: m, name: name}, nil
}

// EncodeModule encodes the given module into a byte slice depending on the format of the implementation.
// See binary.EncodeModule
type EncodeModule func(m *Module) []byte

var EncodeModuleBinary EncodeModule = func(m *Module) []byte {
	return encodeModule(binary.EncodeModule, m)
}

func encodeModule(encoder internalwasm.EncodeModule, m *Module) []byte {
	return encoder(m.m)
}

type Module struct {
	m    *internalwasm.Module
	name string
}

// Name defaults to what's decoded from the custom name section and can be overridden WithName.
// See https://www.w3.org/TR/wasm-core-1/#name-section%E2%91%A0
func (m *Module) Name() string {
	return m.name
}

// WithName overwrites Name
func (m *Module) WithName(name string) *Module {
	m.name = name
	return m
}
