// Package require includes test assertions that fail the test immediately. This is like to testify, but without a
// dependency.
//
// Note: Assertions here are internal and are free to be customized to only support valid WebAssembly types, or to
// reduce code in tests that only require certain types.
package require

import (
	"bytes"
	"errors"
	"fmt"
	"path"
	"reflect"
	"runtime"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/tetratelabs/wazero/experimental/sys"
)

// TestingT is an interface wrapper of functions used in TestingT
type TestingT interface {
	Fatal(args ...interface{})
}

type EqualTo interface {
	EqualTo(that interface{}) bool
}

// TODO: implement, test and document each function without using testify

// Contains fails if `s` does not contain `substr` using strings.Contains.
//
//   - formatWithArgs are optional. When the first is a string that contains '%', it is treated like fmt.Sprintf.
func Contains(t TestingT, s, substr string, formatWithArgs ...interface{}) {
	if !strings.Contains(s, substr) {
		fail(t, fmt.Sprintf("expected %q to contain %q", s, substr), "", formatWithArgs...)
	}
}

// Equal fails if the actual value is not equal to the expected.
//
//   - formatWithArgs are optional. When the first is a string that contains '%', it is treated like fmt.Sprintf.
func Equal(t TestingT, expected, actual interface{}, formatWithArgs ...interface{}) {
	if expected == nil {
		Nil(t, actual)
		return
	}
	if equal(expected, actual) {
		return
	}
	_, expectString := expected.(string)
	if actual == nil {
		if expectString {
			fail(t, fmt.Sprintf("expected %q, but was nil", expected), "", formatWithArgs...)
		} else {
			fail(t, fmt.Sprintf("expected %#v, but was nil", expected), "", formatWithArgs...)
		}
		return
	}

	// Include the type name if the actual wasn't the same
	et, at := reflect.ValueOf(expected).Type(), reflect.ValueOf(actual).Type()
	if et != at {
		if expectString {
			fail(t, fmt.Sprintf("expected %q, but was %s(%v)", expected, at, actual), "", formatWithArgs...)
		} else {
			fail(t, fmt.Sprintf("expected %s(%v), but was %s(%v)", et, expected, at, actual), "", formatWithArgs...)
		}
		return
	}

	// Inline the comparison if the types are likely small:
	if expectString {
		// Don't use %q as it escapes newlines!
		fail(t, fmt.Sprintf("expected \"%s\", but was \"%s\"", expected, actual), "", formatWithArgs...)
		return
	} else if et.Kind() < reflect.Array {
		fail(t, fmt.Sprintf("expected %v, but was %v", expected, actual), "", formatWithArgs...)
		return
	} else if et.Kind() == reflect.Func {
		// compare funcs by string pointer
		expected := fmt.Sprintf("%v", expected)
		actual := fmt.Sprintf("%v", actual)
		if expected != actual {
			fail(t, fmt.Sprintf("expected %s, but was %s", expected, actual), "", formatWithArgs...)
		}
		return
	} else if eq, ok := actual.(EqualTo); ok {
		if !eq.EqualTo(expected) {
			fail(t, fmt.Sprintf("expected %v, but was %v", expected, actual), "", formatWithArgs...)
		}
	}

	// If we have the same type, and it isn't a string, but the expected and actual values on a different line.
	// This allows easier comparison without using a diff library.
	fail(t, "unexpected value", fmt.Sprintf("expected:\n\t%#v\nwas:\n\t%#v\n", expected, actual), formatWithArgs...)
}

// equal speculatively tries to cast the inputs as byte arrays and falls back to reflection.
func equal(expected, actual interface{}) bool {
	if b1, ok := expected.([]byte); !ok {
		return reflect.DeepEqual(expected, actual)
	} else if b2, ok := actual.([]byte); ok {
		return bytes.Equal(b1, b2)
	}
	return false
}

// EqualError fails if the error is nil or its `Error()` value is not equal to
// the expected string.
//
//   - formatWithArgs are optional. When the first is a string that contains '%', it is treated like fmt.Sprintf.
func EqualError(t TestingT, err error, expected string, formatWithArgs ...interface{}) {
	if err == nil {
		fail(t, "expected an error, but was nil", "", formatWithArgs...)
		return
	}
	actual := err.Error()
	if actual != expected {
		fail(t, fmt.Sprintf("expected error \"%s\", but was \"%s\"", expected, actual), "", formatWithArgs...)
	}
}

// Error fails if the err is nil.
//
//   - formatWithArgs are optional. When the first is a string that contains '%', it is treated like fmt.Sprintf.
func Error(t TestingT, err error, formatWithArgs ...interface{}) {
	if err == nil {
		fail(t, "expected an error, but was nil", "", formatWithArgs...)
	}
}

// EqualErrno should be used for functions that return sys.Errno or nil.
func EqualErrno(t TestingT, expected sys.Errno, err error, formatWithArgs ...interface{}) {
	if err == nil {
		fail(t, "expected a sys.Errno, but was nil", "", formatWithArgs...)
		return
	}
	if se, ok := err.(sys.Errno); !ok {
		fail(t, fmt.Sprintf("expected %v to be a sys.Errno", err), "", formatWithArgs...)
	} else if se != expected {
		fail(t, fmt.Sprintf("expected Errno %#[1]v(%[1]s), but was %#[2]v(%[2]s)", expected, err), "", formatWithArgs...)
	}
}

// ErrorIs fails if the err is nil or errors.Is fails against the expected.
//
//   - formatWithArgs are optional. When the first is a string that contains '%', it is treated like fmt.Sprintf.
func ErrorIs(t TestingT, err, target error, formatWithArgs ...interface{}) {
	if err == nil {
		fail(t, "expected an error, but was nil", "", formatWithArgs...)
		return
	}
	if !errors.Is(err, target) {
		fail(t, fmt.Sprintf("expected errors.Is(%v, %v), but it wasn't", err, target), "", formatWithArgs...)
	}
}

// False fails if the actual value was true.
//
//   - formatWithArgs are optional. When the first is a string that contains '%', it is treated like fmt.Sprintf.
func False(t TestingT, actual bool, formatWithArgs ...interface{}) {
	if actual {
		fail(t, "expected false, but was true", "", formatWithArgs...)
	}
}

// Nil fails if the object is not nil.
//
//   - formatWithArgs are optional. When the first is a string that contains '%', it is treated like fmt.Sprintf.
func Nil(t TestingT, object interface{}, formatWithArgs ...interface{}) {
	if !isNil(object) {
		fail(t, fmt.Sprintf("expected nil, but was %v", object), "", formatWithArgs...)
	}
}

// NoError fails if the err is not nil.
//
//   - formatWithArgs are optional. When the first is a string that contains '%', it is treated like fmt.Sprintf.
func NoError(t TestingT, err error, formatWithArgs ...interface{}) {
	if err != nil {
		fail(t, fmt.Sprintf("expected no error, but was %v", err), "", formatWithArgs...)
	}
}

// NotEqual fails if the actual value is equal to the expected.
//
//   - formatWithArgs are optional. When the first is a string that contains '%', it is treated like fmt.Sprintf.
func NotEqual(t TestingT, expected, actual interface{}, formatWithArgs ...interface{}) {
	if !equal(expected, actual) {
		return
	}
	_, expectString := expected.(string)
	if expectString {
		fail(t, fmt.Sprintf("expected to not equal %q", actual), "", formatWithArgs...)
		return
	}
	fail(t, fmt.Sprintf("expected to not equal %#v", actual), "", formatWithArgs...)
}

// NotNil fails if the object is nil.
//
//   - formatWithArgs are optional. When the first is a string that contains '%', it is treated like fmt.Sprintf.
func NotNil(t TestingT, object interface{}, formatWithArgs ...interface{}) {
	if isNil(object) {
		fail(t, "expected to not be nil", "", formatWithArgs...)
	}
}

// isNil is less efficient for the sake of less code vs tracking all the nil types in Go.
func isNil(object interface{}) (isNil bool) {
	if object == nil {
		return true
	}

	v := reflect.ValueOf(object)

	defer func() {
		if recovered := recover(); recovered != nil {
			// ignore problems using isNil on a type that can't be nil
			isNil = false
		}
	}()

	isNil = v.IsNil()
	return
}

// NotSame fails if the inputs point to the same object.
//
//   - formatWithArgs are optional. When the first is a string that contains '%', it is treated like fmt.Sprintf.
func NotSame(t TestingT, expected, actual interface{}, formatWithArgs ...interface{}) {
	if equalsPointer(expected, actual) {
		fail(t, fmt.Sprintf("expected %v to point to a different object", actual), "", formatWithArgs...)
		return
	}
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

// Same fails if the inputs don't point to the same object.
//
//   - formatWithArgs are optional. When the first is a string that contains '%', it is treated like fmt.Sprintf.
func Same(t TestingT, expected, actual interface{}, formatWithArgs ...interface{}) {
	if !equalsPointer(expected, actual) {
		fail(t, fmt.Sprintf("expected %v to point to the same object as %v", actual, expected), "", formatWithArgs...)
		return
	}
}

func equalsPointer(expected, actual interface{}) bool {
	expectedV := reflect.ValueOf(expected)
	if expectedV.Kind() != reflect.Ptr {
		panic("BUG: expected was not a pointer")
	}
	actualV := reflect.ValueOf(actual)
	if actualV.Kind() != reflect.Ptr {
		panic("BUG: actual was not a pointer")
	}

	if t1, t2 := reflect.TypeOf(expectedV), reflect.TypeOf(actualV); t1 != t2 {
		return false
	} else {
		return expected == actual
	}
}

// True fails if the actual value wasn't.
//
//   - formatWithArgs are optional. When the first is a string that contains '%', it is treated like fmt.Sprintf.
func True(t TestingT, actual bool, formatWithArgs ...interface{}) {
	if !actual {
		fail(t, "expected true, but was false", "", formatWithArgs...)
	}
}

// Zero fails if the actual value wasn't.
//
//   - formatWithArgs are optional. When the first is a string that contains '%', it is treated like fmt.Sprintf.
//
// Note: This isn't precise to numeric types, but we don't care as being more precise is more code and tests.
func Zero(t TestingT, i interface{}, formatWithArgs ...interface{}) {
	if i == nil {
		fail(t, "expected zero, but was nil", "", formatWithArgs...)
	}
	zero := reflect.Zero(reflect.TypeOf(i))
	if i != zero.Interface() {
		fail(t, fmt.Sprintf("expected zero, but was %v", i), "", formatWithArgs...)
	}
}

// fail tries to treat the formatWithArgs as fmt.Sprintf parameters or joins on space.
func fail(t TestingT, m1, m2 string, formatWithArgs ...interface{}) {
	var failure string
	if len(formatWithArgs) > 0 {
		if s, ok := formatWithArgs[0].(string); ok && strings.Contains(s, "%") {
			failure = fmt.Sprintf(m1+": "+s, formatWithArgs[1:]...)
		} else {
			var builder strings.Builder
			builder.WriteString(fmt.Sprintf("%s: %v", m1, formatWithArgs[0]))
			for _, v := range formatWithArgs[1:] {
				builder.WriteByte(' ')
				builder.WriteString(fmt.Sprintf("%v", v))
			}
			failure = builder.String()
		}
	} else {
		failure = m1
	}
	if m2 != "" {
		failure = failure + "\n" + m2
	}

	// Don't write the failStack in our own package!
	if fs := failStack(); len(fs) > 0 {
		t.Fatal(failure + "\n" + strings.Join(fs, "\n"))
	} else {
		t.Fatal(failure)
	}
}

// failStack returns the stack leading to the failure, without test infrastructure.
//
// Note: This is similar to assert.CallerInfo in testify
// Note: This is untested because it is a lot of work to do that. The rationale to punt is this is a test-only internal
// type which returns optional info. Someone can add tests, but they'd need to do that as an integration test in a
// different package with something stable line-number-wise.
func failStack() (fs []string) {
	for i := 0; ; i++ {
		pc, file, line, ok := runtime.Caller(i)
		if !ok {
			break // don't loop forever on a bug
		}

		f := runtime.FuncForPC(pc)
		if f == nil {
			break // don't loop forever on a bug
		}
		name := f.Name()

		if name == "testing.tRunner" {
			break // Don't add the runner from src/testing/testing.go
		}

		// Ensure we don't add functions in the require package to the failure stack.
		dir := path.Dir(file)
		if path.Base(dir) != "require" {
			fs = append(fs, fmt.Sprintf("%s:%d", file, line))
		}

		// Stop the stack when we get to a test. Strip off any leading package name first!
		if dot := strings.Index(name, "."); dot > 0 {
			if isTest(name[dot+1:]) {
				return
			}
		}
	}
	return
}

var testPrefixes = []string{"Test", "Benchmark", "Example"}

// isTest is similar to load.isTest in Go's src/cmd/go/internal/load/test.go
func isTest(name string) bool {
	for _, prefix := range testPrefixes {
		if !strings.HasPrefix(name, prefix) {
			return false
		}
		if len(name) == len(prefix) { // "Test" is ok
			return true
		}
		if r, _ := utf8.DecodeRuneInString(name[len(prefix):]); !unicode.IsLower(r) {
			return true
		}
	}
	return false
}
