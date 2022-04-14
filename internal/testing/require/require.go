// Package require includes test assertions that fail the test immediately. This is like to testify, but without a
// dependency.
//
// Note: Assertions here are internal and are free to be customized to only support valid WebAssembly types, or to
// reduce code in tests that only require certain types.
package require

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// TODO: implement, test and document each function without using testify

func Contains(t *testing.T, s interface{}, contains interface{}, msgAndArgs ...interface{}) {
	require.Contains(t, s, contains, msgAndArgs)
}

func Empty(t *testing.T, object interface{}, msgAndArgs ...interface{}) {
	require.Empty(t, object, msgAndArgs)
}

func Equal(t *testing.T, expected interface{}, actual interface{}, msgAndArgs ...interface{}) {
	require.Equal(t, expected, actual, msgAndArgs)
}

func EqualError(t *testing.T, theError error, errString string, msgAndArgs ...interface{}) {
	require.EqualError(t, theError, errString, msgAndArgs)
}

func Error(t *testing.T, err error, msgAndArgs ...interface{}) {
	require.Error(t, err, msgAndArgs)
}

func ErrorIs(t *testing.T, err error, target error, msgAndArgs ...interface{}) {
	require.ErrorIs(t, err, target, msgAndArgs)
}

func False(t *testing.T, value bool, msgAndArgs ...interface{}) {
	require.False(t, value, msgAndArgs)
}

func Len(t *testing.T, object interface{}, length int, msgAndArgs ...interface{}) {
	require.Len(t, object, length, msgAndArgs)
}

func Nil(t *testing.T, object interface{}, msgAndArgs ...interface{}) {
	require.Nil(t, object, msgAndArgs)
}

func NoError(t *testing.T, err error, msgAndArgs ...interface{}) {
	require.NoError(t, err, msgAndArgs)
}

func NotContains(t *testing.T, s interface{}, contains interface{}, msgAndArgs ...interface{}) {
	require.NotContains(t, s, contains, msgAndArgs)
}

func NotEmpty(t *testing.T, object interface{}, msgAndArgs ...interface{}) {
	require.NotEmpty(t, object, msgAndArgs)
}

func NotEqual(t *testing.T, expected interface{}, actual interface{}, msgAndArgs ...interface{}) {
	require.NotEqual(t, expected, actual, msgAndArgs)
}

func NotNil(t *testing.T, object interface{}, msgAndArgs ...interface{}) {
	require.NotNil(t, object, msgAndArgs)
}

func NotSame(t *testing.T, expected interface{}, actual interface{}, msgAndArgs ...interface{}) {
	require.NotSame(t, expected, actual, msgAndArgs)
}

// CapturePanic returns an error recovered from a panic. If the panic was not an error, this converts it to one.
func CapturePanic(panics func()) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			if e, ok := recovered.(error); ok {
				err = e
			} else {
				err = fmt.Errorf("%v", recovered)
			}
		}
	}()
	panics()
	return
}

func Same(t *testing.T, expected interface{}, actual interface{}, msgAndArgs ...interface{}) {
	require.Same(t, expected, actual, msgAndArgs)
}

func True(t *testing.T, value bool, msgAndArgs ...interface{}) {
	require.True(t, value, msgAndArgs)
}

func Zero(t *testing.T, i interface{}, msgAndArgs ...interface{}) {
	require.Zero(t, i, msgAndArgs)
}
