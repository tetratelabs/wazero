package wazerotest

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
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
	exitStatus uint64

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

	once                        sync.Once
	exportedFunctions           map[string]api.Function
	exportedFunctionDefinitions map[string]api.FunctionDefinition
	exportedGlobals             map[string]api.Global
	exportedMemoryDefinitions   map[string]api.MemoryDefinition
}

func (m *Module) String() string {
	return "module[" + m.ModuleName + "]"
}

func (m *Module) Name() string {
	return m.ModuleName
}

func (m *Module) Memory() api.Memory {
	if m.ExportMemory != nil {
		m.once.Do(m.initialize)
		return m.ExportMemory
	}
	return nil
}

func (m *Module) ExportedFunction(name string) api.Function {
	m.once.Do(m.initialize)
	return m.exportedFunctions[name]
}

func (m *Module) ExportedFunctionDefinitions() map[string]api.FunctionDefinition {
	m.once.Do(m.initialize)
	return m.exportedFunctionDefinitions
}

func (m *Module) ExportedMemory(name string) api.Memory {
	if m.ExportMemory != nil && name == "memory" {
		m.once.Do(m.initialize)
		return m.ExportMemory
	}
	return nil
}

func (m *Module) ExportedMemoryDefinitions() map[string]api.MemoryDefinition {
	m.once.Do(m.initialize)
	return m.exportedMemoryDefinitions
}

func (m *Module) ExportedGlobal(name string) api.Global {
	m.once.Do(m.initialize)
	return m.exportedGlobals[name]
}

func (m *Module) NumFunction() int {
	return len(m.Functions)
}

func (m *Module) Function(i int) api.Function {
	m.once.Do(m.initialize)
	return m.Functions[i]
}

func (m *Module) NumGlobal() int {
	return len(m.Globals)
}

func (m *Module) Global(i int) api.Global {
	m.once.Do(m.initialize)
	return m.Globals[i]
}

func (m *Module) Close(ctx context.Context) error {
	return m.CloseWithExitCode(ctx, 0)
}

func (m *Module) CloseWithExitCode(ctx context.Context, exitCode uint32) error {
	atomic.CompareAndSwapUint64(&m.exitStatus, 0, exitStatusMarker|uint64(exitCode))
	return nil
}

func (m *Module) ExitStatus() (exitCode uint32, exited bool) {
	exitStatus := atomic.LoadUint64(&m.exitStatus)
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
		for i := range stack {
			stack[i] = 0
		}
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

const PageSize = 65536

var exportedMemoryName = [1]string{"memory"}

func (m *Memory) Definition() api.MemoryDefinition {
	return memoryDefinition{memory: m}
}

func (m *Memory) Size() uint32 {
	return uint32(len(m.Bytes))
}

func (m *Memory) Grow(deltaPages uint32) (previousPages uint32, ok bool) {
	previousPages = uint32(len(m.Bytes) / PageSize)
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
		return exportedMemoryName[:]
	}
	return nil
}

func (def memoryDefinition) Min() uint32 {
	return def.memory.Min
}

func (def memoryDefinition) Max() (uint32, bool) {
	return def.memory.Max, def.memory.Max != 0
}

// StackFrame represents a frame on the call stack.
type StackFrame struct {
	Function api.Function
	Params   []uint64
	Results  []uint64
}

type stackIterator struct {
	stack []StackFrame
	fndef []api.FunctionDefinition
	index int
}

func (si *stackIterator) Next() bool {
	si.index++
	return si.index < len(si.stack)
}

func (si *stackIterator) FunctionDefinition() api.FunctionDefinition {
	return si.fndef[si.index]
}

func (si *stackIterator) Parameters() []uint64 {
	return si.stack[si.index].Params
}

func (si *stackIterator) reset() {
	si.index = 0
}

func newStackIterator(stack []StackFrame) *stackIterator {
	si := &stackIterator{
		stack: stack,
		fndef: make([]api.FunctionDefinition, len(stack)),
	}
	// The size of functionDefinition is only one pointer which should allow
	// the compiler to optimize the conversion to api.FunctionDefinition; but
	// the presence of internal.WazeroOnlyType, despite being defined as an
	// empty struct, forces a heap allocation that we amortize by caching the
	// result.
	for i, frame := range stack {
		si.fndef[i] = frame.Function.Definition()
	}
	return si
}

var (
	_ experimental.InternalModule = (*Module)(nil)

	_ api.Module   = (*Module)(nil)
	_ api.Function = (*Function)(nil)
	_ api.Global   = (*Global)(nil)
)

// BenchmarkFunctionListener implements a benchmark for function listeners.
//
// The benchmark calls Before and After methods repeatedly using the provided
// module an stack frames to invoke the methods.
//
// The stack frame is a representation of the call stack that the Before method
// will be invoked with. The top of the stack is stored at index zero. The stack
// must contain at least one frame or the benchmark will fail.
func BenchmarkFunctionListener(b *testing.B, module *Module, stack []StackFrame, listener experimental.FunctionListener) {
	if len(stack) == 0 {
		b.Error("cannot benchmark function listener with an empty stack")
		return
	}

	functionDefinition := stack[0].Function.Definition()
	functionParams := stack[0].Params
	functionResults := stack[0].Results
	stackIterator := newStackIterator(stack)
	ctx := context.Background()

	for i := 0; i < b.N; i++ {
		stackIterator.reset()
		callContext := listener.Before(ctx, module, functionDefinition, functionParams, stackIterator)
		listener.After(callContext, module, functionDefinition, nil, functionResults)
	}
}
