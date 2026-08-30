// Package wasmdebug contains utilities used to give consistent search keys between stack traces and error messages.
// Note: This is named wasmdebug to avoid conflicts with the normal go module.
// Note: This only imports "api" as importing "wasm" would create a cyclic dependency.
package wasmdebug

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/wasmruntime"
	"github.com/tetratelabs/wazero/sys"
)

// FuncName returns the naming convention of "moduleName.funcName".
//
//   - moduleName is the possibly empty name the module was instantiated with.
//   - funcName is the name in the Custom Name section.
//   - funcIdx is the position in the function index, prefixed with
//     imported functions.
//
// Note: "moduleName.$funcIdx" is used when the funcName is empty, as commonly
// the case in TinyGo.
func FuncName(moduleName, funcName string, funcIdx uint32) string {
	var ret strings.Builder

	// Start module.function
	ret.WriteString(moduleName)
	ret.WriteByte('.')
	if funcName == "" {
		ret.WriteByte('$')
		ret.WriteString(strconv.Itoa(int(funcIdx)))
	} else {
		ret.WriteString(funcName)
	}

	return ret.String()
}

// signature returns a formatted signature similar to how it is defined in Go.
//
// * paramTypes should be from wasm.FunctionType
// * resultTypes should be from wasm.FunctionType
// TODO: add paramNames
func signature(funcName string, paramTypes []api.ValueType, resultTypes []api.ValueType) string {
	var ret strings.Builder
	ret.WriteString(funcName)

	// Start params
	ret.WriteByte('(')
	paramCount := len(paramTypes)
	switch paramCount {
	case 0:
	case 1:
		ret.WriteString(api.ValueTypeName(paramTypes[0]))
	default:
		ret.WriteString(api.ValueTypeName(paramTypes[0]))
		for _, vt := range paramTypes[1:] {
			ret.WriteByte(',')
			ret.WriteString(api.ValueTypeName(vt))
		}
	}
	ret.WriteByte(')')

	// Start results
	resultCount := len(resultTypes)
	switch resultCount {
	case 0:
	case 1:
		ret.WriteByte(' ')
		ret.WriteString(api.ValueTypeName(resultTypes[0]))
	default: // As this is used for errors, don't panic if there are multiple returns, even if that's invalid!
		ret.WriteByte(' ')
		ret.WriteByte('(')
		ret.WriteString(api.ValueTypeName(resultTypes[0]))
		for _, vt := range resultTypes[1:] {
			ret.WriteByte(',')
			ret.WriteString(api.ValueTypeName(vt))
		}
		ret.WriteByte(')')
	}

	return ret.String()
}

// ErrorBuilder helps build consistent errors, particularly adding a WASM stack trace.
//
// AddFrame should be called beginning at the frame that panicked until no more frames exist. Once done, call Format.
type ErrorBuilder interface {
	// AddFrame adds the next frame.
	//
	// * funcName should be from FuncName
	// * paramTypes should be from wasm.FunctionType
	// * resultTypes should be from wasm.FunctionType
	// * sources is the source code information for this frame and can be empty.
	//
	// Note: paramTypes and resultTypes are present because signature misunderstanding, mismatch or overflow are common.
	AddFrame(funcName string, paramTypes, resultTypes []api.ValueType, sources []string)

	// StartSection begins a group of frames printed under its own header, for frames that
	// are not part of the stack the error happened on. The frame budget starts over, so a
	// long main trace cannot crowd a section out.
	//
	// header is printed followed by a colon, e.g. "originally thrown at".
	StartSection(header string)

	// FromRecovered returns an error with the wasm stack trace appended to it.
	FromRecovered(recovered interface{}) error
}

func NewErrorBuilder() ErrorBuilder {
	return &stackTrace{}
}

type stackTrace struct {
	// frameCount is the number of stack frame currently pushed into the section being built.
	frameCount int
	// lines contains the stack trace and possibly the inlined source code information.
	lines []string
	// sections contains the groups started with StartSection, in order. Frames go to the
	// last one once there is one, so lines holds the main trace alone.
	sections []traceSection
}

// traceSection is a group of frames printed under its own header. See StartSection.
type traceSection struct {
	header string
	lines  []string
}

// GoRuntimeErrorTracePrefix is the prefix coming before the Go runtime stack trace included in the face of runtime.Error.
// This is exported for testing purpose.
const GoRuntimeErrorTracePrefix = "Go runtime stack trace:"

func (s *stackTrace) FromRecovered(recovered interface{}) error {
	if false {
		debug.PrintStack()
	}

	if exitErr, ok := recovered.(*sys.ExitError); ok { // Don't wrap an exit error!
		return exitErr
	}

	stack := s.trace()

	// If the error was internal, don't mention it was recovered.
	if wasmErr, ok := recovered.(*wasmruntime.Error); ok {
		return fmt.Errorf("wasm error: %w\nwasm stack trace:\n\t%s", wasmErr, stack)
	}

	// If we have a runtime.Error, something severe happened which should include the stack trace. This could be
	// a nil pointer from wazero or a user-defined function from HostModuleBuilder.
	if runtimeErr, ok := recovered.(runtime.Error); ok {
		return fmt.Errorf("%w (recovered by wazero)\nwasm stack trace:\n\t%s\n\n%s\n%s",
			runtimeErr, stack, GoRuntimeErrorTracePrefix, debug.Stack())
	}

	// At this point we expect the error was from a function defined by HostModuleBuilder that intentionally called panic.
	if runtimeErr, ok := recovered.(error); ok { // e.g. panic(errors.New("whoops"))
		return fmt.Errorf("%w (recovered by wazero)\nwasm stack trace:\n\t%s", runtimeErr, stack)
	} else { // e.g. panic("whoops")
		return fmt.Errorf("%v (recovered by wazero)\nwasm stack trace:\n\t%s", recovered, stack)
	}
}

// MaxFrames is the maximum number of frames to include in the stack trace.
const MaxFrames = 30

// ExceptionOriginSection is the StartSection header under which an uncaught exception
// reports where it was first thrown, when that differs from where it was last thrown.
const ExceptionOriginSection = "originally thrown at"

// trace renders the main trace, then each section under its own header. The caller supplies
// the main trace's header, so it is joined the way it always was.
func (s *stackTrace) trace() string {
	trace := strings.Join(s.lines, "\n\t")
	for i := range s.sections {
		sec := &s.sections[i]
		if len(sec.lines) == 0 {
			continue
		}
		trace += "\n" + sec.header + ":\n\t" + strings.Join(sec.lines, "\n\t")
	}
	return trace
}

// StartSection implements ErrorBuilder.StartSection
func (s *stackTrace) StartSection(header string) {
	s.sections = append(s.sections, traceSection{header: header})
	s.frameCount = 0
}

// appendLine adds a line to whichever section is being built.
func (s *stackTrace) appendLine(line string) {
	if n := len(s.sections); n > 0 {
		s.sections[n-1].lines = append(s.sections[n-1].lines, line)
		return
	}
	s.lines = append(s.lines, line)
}

// AddFrame implements ErrorBuilder.AddFrame
func (s *stackTrace) AddFrame(funcName string, paramTypes, resultTypes []api.ValueType, sources []string) {
	if s.frameCount == MaxFrames {
		return
	}
	s.frameCount++
	sig := signature(funcName, paramTypes, resultTypes)
	s.appendLine(sig)
	for _, source := range sources {
		s.appendLine("\t" + source)
	}
	if s.frameCount == MaxFrames {
		s.appendLine("... maybe followed by omitted frames")
	}
}
