package logging

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/wasi_snapshot_preview1"
)

// NewLoggingListenerFactory implements FunctionListenerFactory to log all
// functions that have a name to the writer.
func NewLoggingListenerFactory(writer io.Writer) experimental.FunctionListenerFactory {
	return &loggingListenerFactory{writer}
}

type loggingListenerFactory struct{ writer io.Writer }

// NewListener implements the same method as documented on
// experimental.FunctionListener.
func (f *loggingListenerFactory) NewListener(fnd api.FunctionDefinition) experimental.FunctionListener {
	return &loggingListener{writer: f.writer, fnd: fnd, isWasi: fnd.ModuleName() == "wasi_snapshot_preview1"}
}

// nestLevelKey holds state between logger.Before and loggingListener.After to ensure
// call depth is reflected.
type nestLevelKey struct{}

// loggingListener implements experimental.FunctionListener to log entrance and exit
// of each function call.
type loggingListener struct {
	writer io.Writer
	fnd    api.FunctionDefinition
	isWasi bool
}

// Before logs to stdout the module and function name, prefixed with '-->' and
// indented based on the call nesting level.
func (l *loggingListener) Before(ctx context.Context, _ api.FunctionDefinition, vals []uint64) context.Context {
	nestLevel, _ := ctx.Value(nestLevelKey{}).(int)

	l.writeIndented(true, nil, vals, nestLevel+1)

	// Increase the next nesting level.
	return context.WithValue(ctx, nestLevelKey{}, nestLevel+1)
}

// After logs to stdout the module and function name, prefixed with '<--' and
// indented based on the call nesting level.
func (l *loggingListener) After(ctx context.Context, _ api.FunctionDefinition, err error, vals []uint64) {
	// Note: We use the nest level directly even though it is the "next" nesting level.
	// This works because our indent of zero nesting is one tab.
	l.writeIndented(false, err, vals, ctx.Value(nestLevelKey{}).(int))
}

// writeIndented writes an indented message like this: "-->\t\t\t$indentLevel$funcName\n"
func (l *loggingListener) writeIndented(before bool, err error, vals []uint64, indentLevel int) {
	var message strings.Builder
	for i := 1; i < indentLevel; i++ {
		message.WriteByte('\t')
	}
	if before {
		if l.fnd.GoFunc() != nil {
			message.WriteString("==> ")
		} else {
			message.WriteString("--> ")
		}
		l.writeFuncEnter(&message, vals)
	} else { // after
		if l.fnd.GoFunc() != nil {
			message.WriteString("<== ")
		} else {
			message.WriteString("<-- ")
		}
		l.writeFuncExit(&message, err, vals)
	}
	message.WriteByte('\n')

	_, _ = l.writer.Write([]byte(message.String()))
}

func (l *loggingListener) writeFuncEnter(message *strings.Builder, vals []uint64) {
	valLen := len(vals)
	message.WriteString(l.fnd.DebugName())
	message.WriteByte('(')
	switch valLen {
	case 0:
	default:
		i := l.writeParam(message, 0, vals)
		for i < valLen {
			message.WriteByte(',')
			i = l.writeParam(message, i, vals)
		}
	}
	message.WriteByte(')')
}

func (l *loggingListener) writeFuncExit(message *strings.Builder, err error, vals []uint64) {
	if err != nil {
		message.WriteString("error: ")
		message.WriteString(err.Error())
		return
	} else if l.isWasi {
		message.WriteString(wasi_snapshot_preview1.ErrnoName(uint32(vals[0])))
		return
	}
	valLen := len(vals)
	message.WriteByte('(')
	switch valLen {
	case 0:
	default:
		i := l.writeResult(message, 0, vals)
		for i < valLen {
			message.WriteByte(',')
			i = l.writeResult(message, i, vals)
		}
	}
	message.WriteByte(')')
}

func (l *loggingListener) writeResult(message *strings.Builder, i int, vals []uint64) int {
	return l.writeVal(message, l.fnd.ResultTypes()[i], i, vals)
}

func (l *loggingListener) writeParam(message *strings.Builder, i int, vals []uint64) int {
	if len(l.fnd.ParamNames()) > 0 {
		message.WriteString(l.fnd.ParamNames()[i])
		message.WriteByte('=')
	}
	return l.writeVal(message, l.fnd.ParamTypes()[i], i, vals)
}

func (l *loggingListener) writeVal(message *strings.Builder, t api.ValueType, i int, vals []uint64) int {
	v := vals[i]
	i++
	switch t {
	case api.ValueTypeI32:
		message.WriteString(strconv.FormatUint(uint64(uint32(v)), 10))
	case api.ValueTypeI64:
		message.WriteString(strconv.FormatUint(v, 10))
	case api.ValueTypeF32:
		message.WriteString(strconv.FormatFloat(float64(api.DecodeF32(v)), 'g', -1, 32))
	case api.ValueTypeF64:
		message.WriteString(strconv.FormatFloat(api.DecodeF64(v), 'g', -1, 64))
	case 0x7b: // wasm.ValueTypeV128
		message.WriteString(fmt.Sprintf("%016x%016x", v, vals[i])) // fixed-width hex
		i++
	case api.ValueTypeExternref, 0x70: // wasm.ValueTypeFuncref
		message.WriteString(fmt.Sprintf("%016x", v)) // fixed-width hex
	}
	return i
}
