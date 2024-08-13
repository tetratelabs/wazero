package wazerotest

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/internalapi"
	"github.com/tetratelabs/wazero/sys"
)

const (
	exitStatusMarker = 1 << 63
)

// Module is an implementation of the api.Module interface, it represents a
// WebAssembly module.
type Module struct {
	internalapi.WazeroOnlyType

	// The module name that will be returned by calling the Name method.
	ModuleName string

	// The list of functions of the module. Functions with a non-empty export
	// names will be exported by the module.
	Functions []*Function

	// The list of globals of the module. Global
	Globals []*Global

	// The program memory. If non-nil, the memory is automatically exported as
	// "memory".
	ExportMemory *Memory

	exitStatus atomic.Uint64

	once                        sync.Once
	exportedFunctions           map[string]api.Function
	exportedFunctionDefinitions map[string]api.FunctionDefinition
	exportedGlobals             map[string]api.Global
	exportedMemoryDefinitions   map[string]api.MemoryDefinition
}

// NewModule constructs a Module object with the given memory and function list.
func NewModule(memory *Memory, functions ...*Function) *Module {
	return &Module{Functions: functions, ExportMemory: memory}
}

// String implements fmt.Stringer.
func (m *Module) String() string {
	return "module[" + m.ModuleName + "]"
}

// Name implements the same method as documented on api.Module.
func (m *Module) Name() string {
	return m.ModuleName
}

// Memory implements the same method as documented on api.Module.
func (m *Module) Memory() api.Memory {
	if m.ExportMemory != nil {
		m.once.Do(m.initialize)
		return m.ExportMemory
	}
	return nil
}

// ExportedFunction implements the same method as documented on api.Module.
func (m *Module) ExportedFunction(name string) api.Function {
	m.once.Do(m.initialize)
	return m.exportedFunctions[name]
}

// ExportedFunctionDefinitions implements the same method as documented on api.Module.
func (m *Module) ExportedFunctionDefinitions() map[string]api.FunctionDefinition {
	m.once.Do(m.initialize)
	return m.exportedFunctionDefinitions
}

// ExportedMemory implements the same method as documented on api.Module.
func (m *Module) ExportedMemory(name string) api.Memory {
	if m.ExportMemory != nil && name == "memory" {
		m.once.Do(m.initialize)
		return m.ExportMemory
	}
	return nil
}

// ExportedMemoryDefinitions implements the same method as documented on api.Module.
func (m *Module) ExportedMemoryDefinitions() map[string]api.MemoryDefinition {
	m.once.Do(m.initialize)
	return m.exportedMemoryDefinitions
}

// ExportedGlobal implements the same method as documented on api.Module.
func (m *Module) ExportedGlobal(name string) api.Global {
	m.once.Do(m.initialize)
	return m.exportedGlobals[name]
}

// Close implements the same method as documented on api.Closer.
func (m *Module) Close(ctx context.Context) error {
	return m.CloseWithExitCode(ctx, 0)
}

// CloseWithExitCode implements the same method as documented on api.Closer.
func (m *Module) CloseWithExitCode(ctx context.Context, exitCode uint32) error {
	m.exitStatus.CompareAndSwap(0, exitStatusMarker|uint64(exitCode))
	return nil
}

// IsClosed implements the same method as documented on api.Module.
func (m *Module) IsClosed() bool {
	_, exited := m.ExitStatus()
	return exited
}

// NumGlobal implements the same method as documented on experimental.InternalModule.
func (m *Module) NumGlobal() int {
	return len(m.Globals)
}

// Global implements the same method as documented on experimental.InternalModule.
func (m *Module) Global(i int) api.Global {
	m.once.Do(m.initialize)
	return m.Globals[i]
}

// Below are undocumented extensions

func (m *Module) NumFunction() int {
	return len(m.Functions)
}

func (m *Module) Function(i int) api.Function {
	m.once.Do(m.initialize)
	return m.Functions[i]
}

func (m *Module) ExitStatus() (exitCode uint32, exited bool) {
	exitStatus := m.exitStatus.Load()
	return uint32(exitStatus), exitStatus != 0
}

func (m *Module) initialize() {
	m.exportedFunctions = make(map[string]api.Function)
	m.exportedFunctionDefinitions = make(map[string]api.FunctionDefinition)
	m.exportedGlobals = make(map[string]api.Global)
	m.exportedMemoryDefinitions = make(map[string]api.MemoryDefinition)

	for index, function := range m.Functions {
		for _, exportName := range function.ExportNames {
			m.exportedFunctions[exportName] = function
			m.exportedFunctionDefinitions[exportName] = function.Definition()
		}
		function.module = m
		function.index = index
	}

	for _, global := range m.Globals {
		for _, exportName := range global.ExportNames {
			m.exportedGlobals[exportName] = global
		}
	}

	if m.ExportMemory != nil {
		m.ExportMemory.module = m
		m.exportedMemoryDefinitions["memory"] = m.ExportMemory.Definition()
	}
}

// Global is an implementation of the api.Global interface, it represents a
// global in a WebAssembly module.
type Global struct {
	internalapi.WazeroOnlyType

	// Type of the global value, used to interpret bits of the Value field.
	ValueType api.ValueType

	// Value of the global packed in a 64 bits field.
	Value uint64

	// List of names that the globla is exported as.
	ExportNames []string
}

func (g *Global) String() string {
	switch g.ValueType {
	case api.ValueTypeI32:
		return strconv.FormatInt(int64(api.DecodeI32(g.Value)), 10)
	case api.ValueTypeI64:
		return strconv.FormatInt(int64(g.Value), 10)
	case api.ValueTypeF32:
		return strconv.FormatFloat(float64(api.DecodeF32(g.Value)), 'g', -1, 32)
	case api.ValueTypeF64:
		return strconv.FormatFloat(api.DecodeF64(g.Value), 'g', -1, 64)
	default:
		return "0x" + strconv.FormatUint(g.Value, 16)
	}
}

func (g *Global) Type() api.ValueType {
	return g.ValueType
}

func (g *Global) Get() uint64 {
	return g.Value
}

func GlobalI32(value int32, export ...string) *Global {
	return &Global{ValueType: api.ValueTypeI32, Value: api.EncodeI32(value), ExportNames: export}
}

func GlobalI64(value int64, export ...string) *Global {
	return &Global{ValueType: api.ValueTypeI64, Value: api.EncodeI64(value), ExportNames: export}
}

func GlobalF32(value float32, export ...string) *Global {
	return &Global{ValueType: api.ValueTypeF32, Value: api.EncodeF32(value), ExportNames: export}
}

func GlobalF64(value float64, export ...string) *Global {
	return &Global{ValueType: api.ValueTypeF64, Value: api.EncodeF64(value), ExportNames: export}
}

// Function is an implementation of the api.Function interface, it represents
// a function in a WebAssembly module.
//
// Until accessed through a Module's method, the function definition's
// ModuleName method returns an empty string and its Index method returns 0.
type Function struct {
	internalapi.WazeroOnlyType

	// GoModuleFunction may be set to a non-nil value to allow calling of the
	// function via Call or CallWithStack.
	//
	// It is the user's responsibility to ensure that the signature of this
	// implementation matches the ParamTypes and ResultTypes fields.
	GoModuleFunction api.GoModuleFunction

	// Type lists representing the function signature. Those fields should be
	// set for the function to be properly constructed. The Function's Call
	// and CallWithStack methods will error if those fields are nil.
	ParamTypes  []api.ValueType
	ResultTypes []api.ValueType

	// Sets of names associated with the function. It is valid to leave those
	// names empty, they are only used for debugging purposes.
	FunctionName string
	DebugName    string
	ParamNames   []string
	ResultNames  []string
	ExportNames  []string

	// Lazily initialized when accessed through the module.
	module *Module
	index  int
}

// NewFunction constructs a Function object from a Go function.
//
// The function fn must accept at least two arguments of type context.Context
// and api.Module. Any other arguments and return values must be of type uint32,
// uint64, int32, int64, float32, or float64. The call panics if fn is not a Go
// functionn or has an unsupported signature.
func NewFunction(fn any) *Function {
	functionType := reflect.TypeOf(fn)
	functionValue := reflect.ValueOf(fn)

	paramTypes := make([]api.ValueType, functionType.NumIn()-2)
	paramFuncs := make([]func(uint64) reflect.Value, len(paramTypes))

	resultTypes := make([]api.ValueType, functionType.NumOut())
	resultFuncs := make([]func(reflect.Value) uint64, len(resultTypes))

	for i := range paramTypes {
		var paramType api.ValueType
		var paramFunc func(uint64) reflect.Value

		switch functionType.In(i + 2).Kind() {
		case reflect.Uint32:
			paramType = api.ValueTypeI32
			paramFunc = func(v uint64) reflect.Value { return reflect.ValueOf(api.DecodeU32(v)) }
		case reflect.Uint64:
			paramType = api.ValueTypeI64
			paramFunc = func(v uint64) reflect.Value { return reflect.ValueOf(v) }
		case reflect.Int32:
			paramType = api.ValueTypeI32
			paramFunc = func(v uint64) reflect.Value { return reflect.ValueOf(api.DecodeI32(v)) }
		case reflect.Int64:
			paramType = api.ValueTypeI64
			paramFunc = func(v uint64) reflect.Value { return reflect.ValueOf(int64(v)) }
		case reflect.Float32:
			paramType = api.ValueTypeF32
			paramFunc = func(v uint64) reflect.Value { return reflect.ValueOf(api.DecodeF32(v)) }
		case reflect.Float64:
			paramType = api.ValueTypeF64
			paramFunc = func(v uint64) reflect.Value { return reflect.ValueOf(api.DecodeF64(v)) }
		default:
			panic("cannot construct wasm function from go function of type " + functionType.String())
		}

		paramTypes[i] = paramType
		paramFuncs[i] = paramFunc
	}

	for i := range resultTypes {
		var resultType api.ValueType
		var resultFunc func(reflect.Value) uint64

		switch functionType.Out(i).Kind() {
		case reflect.Uint32:
			resultType = api.ValueTypeI32
			resultFunc = func(v reflect.Value) uint64 { return v.Uint() }
		case reflect.Uint64:
			resultType = api.ValueTypeI64
			resultFunc = func(v reflect.Value) uint64 { return v.Uint() }
		case reflect.Int32:
			resultType = api.ValueTypeI32
			resultFunc = func(v reflect.Value) uint64 { return api.EncodeI32(int32(v.Int())) }
		case reflect.Int64:
			resultType = api.ValueTypeI64
			resultFunc = func(v reflect.Value) uint64 { return api.EncodeI64(v.Int()) }
		case reflect.Float32:
			resultType = api.ValueTypeF32
			resultFunc = func(v reflect.Value) uint64 { return api.EncodeF32(float32(v.Float())) }
		case reflect.Float64:
			resultType = api.ValueTypeF64
			resultFunc = func(v reflect.Value) uint64 { return api.EncodeF64(v.Float()) }
		default:
			panic("cannot construct wasm function from go function of type " + functionType.String())
		}

		resultTypes[i] = resultType
		resultFuncs[i] = resultFunc
	}

	return &Function{
		GoModuleFunction: api.GoModuleFunc(func(ctx context.Context, mod api.Module, stack []uint64) {
			in := make([]reflect.Value, 2+len(paramFuncs))
			in[0] = reflect.ValueOf(ctx)
			in[1] = reflect.ValueOf(mod)
			for i, param := range paramFuncs {
				in[i+2] = param(stack[i])
			}
			out := functionValue.Call(in)
			for i, result := range resultFuncs {
				stack[i] = result(out[i])
			}
		}),
		ParamTypes:  paramTypes,
		ResultTypes: resultTypes,
	}
}

var (
	errMissingFunctionSignature      = errors.New("missing function signature")
	errMissingFunctionModule         = errors.New("missing function module")
	errMissingFunctionImplementation = errors.New("missing function implementation")
)

func (f *Function) Definition() api.FunctionDefinition {
	return functionDefinition{function: f}
}

func (f *Function) Call(ctx context.Context, params ...uint64) ([]uint64, error) {
	stackLen := len(f.ParamTypes)
	if stackLen < len(f.ResultTypes) {
		stackLen = len(f.ResultTypes)
	}
	stack := make([]uint64, stackLen)
	copy(stack, params)
	err := f.CallWithStack(ctx, stack)
	if err != nil {
		clear(stack)
	}
	return stack[:len(f.ResultTypes)], err
}

func (f *Function) CallWithStack(ctx context.Context, stack []uint64) error {
	if f.ParamTypes == nil || f.ResultTypes == nil {
		return errMissingFunctionSignature
	}
	if f.GoModuleFunction == nil {
		return errMissingFunctionImplementation
	}
	if f.module == nil {
		return errMissingFunctionModule
	}
	if exitCode, exited := f.module.ExitStatus(); exited {
		return sys.NewExitError(exitCode)
	}
	f.GoModuleFunction.Call(ctx, f.module, stack)
	return nil
}

type functionDefinition struct {
	internalapi.WazeroOnlyType
	function *Function
}

func (def functionDefinition) Name() string {
	return def.function.FunctionName
}

func (def functionDefinition) DebugName() string {
	if def.function.DebugName != "" {
		return def.function.DebugName
	}
	return fmt.Sprintf("%s.$%d", def.ModuleName(), def.Index())
}

func (def functionDefinition) GoFunction() any {
	return def.function.GoModuleFunction
}

func (def functionDefinition) ParamTypes() []api.ValueType {
	return def.function.ParamTypes
}

func (def functionDefinition) ParamNames() []string {
	return def.function.ParamNames
}

func (def functionDefinition) ResultTypes() []api.ValueType {
	return def.function.ResultTypes
}

func (def functionDefinition) ResultNames() []string {
	return def.function.ResultNames
}

func (def functionDefinition) ModuleName() string {
	if def.function.module != nil {
		return def.function.module.ModuleName
	}
	return ""
}

func (def functionDefinition) Index() uint32 {
	return uint32(def.function.index)
}

func (def functionDefinition) Import() (moduleName, name string, isImport bool) {
	return
}

func (def functionDefinition) ExportNames() []string {
	return def.function.ExportNames
}

// Memory is an implementation of the api.Memory interface, representing the
// memory of a WebAssembly module.
type Memory struct {
	internalapi.WazeroOnlyType

	// Byte slices holding the memory pages.
	//
	// It is the user's repsonsibility to ensure that the length of this byte
	// slice is a multiple of the page size.
	Bytes []byte

	// Min and max number of memory pages which may be held in this memory.
	//
	// Leaving Max to zero means no upper bound.
	Min uint32
	Max uint32

	// Lazily initialized when accessed through the module.
	module *Module
}

// NewMemory constructs a Memory object with a buffer of the given size, aligned
// to the closest multiple of the page size.
func NewMemory(size int) *Memory {
	numPages := (size + (PageSize - 1)) / PageSize
	return &Memory{
		Bytes: make([]byte, numPages*PageSize),
		Min:   uint32(numPages),
	}
}

// NewFixedMemory constructs a Memory object of the given size. The returned
// memory is configured with a max limit to prevent growing beyond its initial
// size.
func NewFixedMemory(size int) *Memory {
	memory := NewMemory(size)
	memory.Max = memory.Min
	return memory
}

// The PageSize constant defines the size of WebAssembly memory pages in bytes.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#page-size
const PageSize = 65536

func (m *Memory) Definition() api.MemoryDefinition {
	return memoryDefinition{memory: m}
}

func (m *Memory) Size() uint32 {
	return uint32(len(m.Bytes))
}

func (m *Memory) Pages() uint32 {
	return uint32(len(m.Bytes) / PageSize)
}

func (m *Memory) Grow(deltaPages uint32) (previousPages uint32, ok bool) {
	previousPages = m.Pages()
	numPages := previousPages + deltaPages
	if m.Max != 0 && numPages > m.Max {
		return previousPages, false
	}
	bytes := make([]byte, PageSize*numPages)
	copy(bytes, m.Bytes)
	m.Bytes = bytes
	return previousPages, true
}

func (m *Memory) ReadByte(offset uint32) (byte, bool) {
	if m.isOutOfRange(offset, 1) {
		return 0, false
	}
	return m.Bytes[offset], true
}

func (m *Memory) ReadUint16Le(offset uint32) (uint16, bool) {
	if m.isOutOfRange(offset, 2) {
		return 0, false
	}
	return binary.LittleEndian.Uint16(m.Bytes[offset:]), true
}

func (m *Memory) ReadUint32Le(offset uint32) (uint32, bool) {
	if m.isOutOfRange(offset, 4) {
		return 0, false
	}
	return binary.LittleEndian.Uint32(m.Bytes[offset:]), true
}

func (m *Memory) ReadUint64Le(offset uint32) (uint64, bool) {
	if m.isOutOfRange(offset, 8) {
		return 0, false
	}
	return binary.LittleEndian.Uint64(m.Bytes[offset:]), true
}

func (m *Memory) ReadFloat32Le(offset uint32) (float32, bool) {
	v, ok := m.ReadUint32Le(offset)
	return math.Float32frombits(v), ok
}

func (m *Memory) ReadFloat64Le(offset uint32) (float64, bool) {
	v, ok := m.ReadUint64Le(offset)
	return math.Float64frombits(v), ok
}

func (m *Memory) Read(offset, length uint32) ([]byte, bool) {
	if m.isOutOfRange(offset, length) {
		return nil, false
	}
	return m.Bytes[offset : offset+length : offset+length], true
}

func (m *Memory) WriteByte(offset uint32, value byte) bool {
	if m.isOutOfRange(offset, 1) {
		return false
	}
	m.Bytes[offset] = value
	return true
}

func (m *Memory) WriteUint16Le(offset uint32, value uint16) bool {
	if m.isOutOfRange(offset, 2) {
		return false
	}
	binary.LittleEndian.PutUint16(m.Bytes[offset:], value)
	return true
}

func (m *Memory) WriteUint32Le(offset uint32, value uint32) bool {
	if m.isOutOfRange(offset, 4) {
		return false
	}
	binary.LittleEndian.PutUint32(m.Bytes[offset:], value)
	return true
}

func (m *Memory) WriteUint64Le(offset uint32, value uint64) bool {
	if m.isOutOfRange(offset, 4) {
		return false
	}
	binary.LittleEndian.PutUint64(m.Bytes[offset:], value)
	return true
}

func (m *Memory) WriteFloat32Le(offset uint32, value float32) bool {
	return m.WriteUint32Le(offset, math.Float32bits(value))
}

func (m *Memory) WriteFloat64Le(offset uint32, value float64) bool {
	return m.WriteUint64Le(offset, math.Float64bits(value))
}

func (m *Memory) Write(offset uint32, value []byte) bool {
	if m.isOutOfRange(offset, uint32(len(value))) {
		return false
	}
	copy(m.Bytes[offset:], value)
	return true
}

func (m *Memory) WriteString(offset uint32, value string) bool {
	if m.isOutOfRange(offset, uint32(len(value))) {
		return false
	}
	copy(m.Bytes[offset:], value)
	return true
}

func (m *Memory) isOutOfRange(offset, length uint32) bool {
	size := m.Size()
	return offset >= size || length > size || offset > (size-length)
}

type memoryDefinition struct {
	internalapi.WazeroOnlyType
	memory *Memory
}

func (def memoryDefinition) ModuleName() string {
	if def.memory.module != nil {
		return def.memory.module.ModuleName
	}
	return ""
}

func (def memoryDefinition) Index() uint32 {
	return 0
}

func (def memoryDefinition) Import() (moduleName, name string, isImport bool) {
	return
}

func (def memoryDefinition) ExportNames() []string {
	if def.memory.module != nil {
		return []string{"memory"}
	}
	return nil
}

func (def memoryDefinition) Min() uint32 {
	return def.memory.Min
}

func (def memoryDefinition) Max() (uint32, bool) {
	return def.memory.Max, def.memory.Max != 0
}

var (
	_ api.Module   = (*Module)(nil)
	_ api.Function = (*Function)(nil)
	_ api.Global   = (*Global)(nil)
)
