package require

import (
	"errors"
	"fmt"
	"io"
	"testing"
)

func TestCapturePanic(t *testing.T) {
	tests := []struct {
		name        string
		panics      func()
		expectedErr string
	}{
		{
			name:        "doesn't panic",
			panics:      func() {},
			expectedErr: "",
		},
		{
			name:        "panics with error",
			panics:      func() { panic(errors.New("error")) },
			expectedErr: "error",
		},
		{
			name:        "panics with string",
			panics:      func() { panic("crash") },
			expectedErr: "crash",
		},
		{
			name:        "panics with object",
			panics:      func() { panic(struct{}{}) },
			expectedErr: "{}",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			captured := CapturePanic(tc.panics)
			if tc.expectedErr == "" {
				if captured != nil {
					t.Fatalf("expected no error, but found %v", captured)
				}
			} else {
				if captured.Error() != tc.expectedErr {
					t.Fatalf("expected %s, but found %s", tc.expectedErr, captured.Error())
				}
			}
		})
	}
}

func TestFail(t *testing.T) {
	tests := []struct {
		name           string
		formatWithArgs []interface{}
		expectedLog    string
	}{
		{
			name:        "message no formatWithArgs",
			expectedLog: "failed",
		},
		{
			name:           "message formatWithArgs =: string",
			formatWithArgs: []interface{}{"because"},
			expectedLog:    "failed: because",
		},
		{
			name:           "message formatWithArgs = [number]",
			formatWithArgs: []interface{}{1},
			expectedLog:    "failed: 1",
		},
		{
			name:           "message formatWithArgs = [struct]",
			formatWithArgs: []interface{}{struct{}{}},
			expectedLog:    "failed: {}",
		},
		{
			name:           "message formatWithArgs = [string, string]",
			formatWithArgs: []interface{}{"because", "this"},
			expectedLog:    "failed: because this",
		},
		{
			name:           "message formatWithArgs = [format, string]",
			formatWithArgs: []interface{}{"because %s", "this"},
			expectedLog:    "failed: because this",
		},
		{
			name:           "message formatWithArgs = [format, struct]",
			formatWithArgs: []interface{}{"because %s", struct{}{}},
			expectedLog:    "failed: because {}",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			m := &mockT{t: t}
			fail(m, "failed", "", tc.formatWithArgs...)
			m.require(tc.expectedLog)
		})
	}
}

type testStruct struct {
	name string
}

func TestRequire(t *testing.T) {
	zero := uint64(0)
	struct1 := &testStruct{"hello"}
	struct2 := &testStruct{"hello"}

	tests := []struct {
		name        string
		require     func(TestingT)
		expectedLog string
	}{
		{
			name: "Contains passes on contains",
			require: func(t TestingT) {
				Contains(t, "hello cat", "cat")
			},
		},
		{
			name: "Contains fails on empty",
			require: func(t TestingT) {
				Contains(t, "", "dog")
			},
			expectedLog: `expected "" to contain "dog"`,
		},
		{
			name: "Contains fails on not contains",
			require: func(t TestingT) {
				Contains(t, "hello cat", "dog")
			},
			expectedLog: `expected "hello cat" to contain "dog"`,
		},
		{
			name: "Contains fails on not contains with format",
			require: func(t TestingT) {
				Contains(t, "hello cat", "dog", "pay me %d", 5)
			},
			expectedLog: `expected "hello cat" to contain "dog": pay me 5`,
		},
		{
			name: "Equal passes on equal: string",
			require: func(t TestingT) {
				Equal(t, "wazero", "wazero")
			},
		},
		{
			name: "Equal passes on equal: []byte",
			require: func(t TestingT) {
				Equal(t, []byte{1, 2, 3, 4}, []byte{1, 2, 3, 4})
			},
		},
		{
			name: "Equal passes on equal: struct",
			require: func(t TestingT) {
				Equal(t, &testStruct{name: "takeshi"}, &testStruct{name: "takeshi"})
			},
		},
		{
			name: "Equal fails on nil: string",
			require: func(t TestingT) {
				Equal(t, "wazero", nil)
			},
			expectedLog: `expected "wazero", but was nil`,
		},
		{
			name: "Equal fails on nil: []byte",
			require: func(t TestingT) {
				Equal(t, []byte{1, 2, 3, 4}, nil)
			},
			expectedLog: `expected []byte{0x1, 0x2, 0x3, 0x4}, but was nil`,
		},
		{
			name: "Equal fails on nil: struct",
			require: func(t TestingT) {
				Equal(t, &testStruct{name: "takeshi"}, nil)
			},
			expectedLog: `expected &require.testStruct{name:"takeshi"}, but was nil`,
		},

		{
			name: "Equal fails on not same type: string",
			require: func(t TestingT) {
				Equal(t, "wazero", uint32(1))
			},
			expectedLog: `expected "wazero", but was uint32(1)`,
		},
		{
			name: "Equal fails on not same type: []byte",
			require: func(t TestingT) {
				Equal(t, []byte{1, 2, 3, 4}, "wazero")
			},
			expectedLog: `expected []uint8([1 2 3 4]), but was string(wazero)`,
		},
		{
			name: "Equal fails on not same type: struct",
			require: func(t TestingT) {
				Equal(t, &testStruct{name: "takeshi"}, "wazero")
			},
			expectedLog: `expected *require.testStruct(&{takeshi}), but was string(wazero)`,
		},
		{
			name: "Equal fails on not equal: string",
			require: func(t TestingT) {
				Equal(t, "wazero", "walero")
			},
			expectedLog: `expected "wazero", but was "walero"`,
		},
		{
			name: "Equal fails on not equal: uint64", // ensure we don't use multi-line output!
			require: func(t TestingT) {
				Equal(t, uint64(12), uint64(13))
			},
			expectedLog: `expected 12, but was 13`,
		},
		{
			name: "Equal fails on not equal: []byte",
			require: func(t TestingT) {
				Equal(t, []byte{1, 2, 3, 4}, []byte{1, 2, 4})
			},
			expectedLog: `unexpected value
expected:
	[]byte{0x1, 0x2, 0x3, 0x4}
was:
	[]byte{0x1, 0x2, 0x4}
`,
		},
		{
			name: "Equal fails on not equal: struct",
			require: func(t TestingT) {
				Equal(t, &testStruct{name: "takeshi"}, &testStruct{name: "adrian"})
			},
			expectedLog: `unexpected value
expected:
	&require.testStruct{name:"takeshi"}
was:
	&require.testStruct{name:"adrian"}
`,
		},
		{
			name: "Equal fails on not equal: struct with format",
			require: func(t TestingT) {
				Equal(t, &testStruct{name: "takeshi"}, &testStruct{name: "adrian"}, "pay me %d", 5)
			},
			expectedLog: `unexpected value: pay me 5
expected:
	&require.testStruct{name:"takeshi"}
was:
	&require.testStruct{name:"adrian"}
`,
		},
		{
			name: "EqualError passes on equal",
			require: func(t TestingT) {
				EqualError(t, io.EOF, io.EOF.Error())
			},
		},
		{
			name: "EqualError fails on nil",
			require: func(t TestingT) {
				EqualError(t, nil, "crash")
			},
			expectedLog: "expected an error, but was nil",
		},
		{
			name: "EqualError fails on not equal",
			require: func(t TestingT) {
				EqualError(t, io.EOF, "crash")
			},
			expectedLog: `expected error "crash", but was "EOF"`,
		},
		{
			name: "EqualError fails on not equal with format",
			require: func(t TestingT) {
				EqualError(t, io.EOF, "crash", "pay me %d", 5)
			},
			expectedLog: `expected error "crash", but was "EOF": pay me 5`,
		},
		{
			name: "Error passes on not nil",
			require: func(t TestingT) {
				Error(t, io.EOF)
			},
		},
		{
			name: "Error fails on nil",
			require: func(t TestingT) {
				Error(t, nil)
			},
			expectedLog: "expected an error, but was nil",
		},
		{
			name: "Error fails on nil with format",
			require: func(t TestingT) {
				Error(t, nil, "pay me %d", 5)
			},
			expectedLog: `expected an error, but was nil: pay me 5`,
		},
		{
			name: "ErrorIs passes on same",
			require: func(t TestingT) {
				ErrorIs(t, io.EOF, io.EOF)
			},
		},
		{
			name: "ErrorIs passes on wrapped",
			require: func(t TestingT) {
				ErrorIs(t, fmt.Errorf("cause: %w", io.EOF), io.EOF)
			},
		},
		{
			name: "ErrorIs fails on not equal",
			require: func(t TestingT) {
				ErrorIs(t, io.EOF, io.ErrUnexpectedEOF)
			},
			expectedLog: "expected errors.Is(EOF, unexpected EOF), but it wasn't",
		},
		{
			name: "ErrorIs fails on not equal with format",
			require: func(t TestingT) {
				ErrorIs(t, io.EOF, io.ErrUnexpectedEOF, "pay me %d", 5)
			},
			expectedLog: `expected errors.Is(EOF, unexpected EOF), but it wasn't: pay me 5`,
		},
		{
			name: "Nil passes on nil",
			require: func(t TestingT) {
				Nil(t, nil)
			},
		},
		{
			name: "Nil fails on not nil",
			require: func(t TestingT) {
				Nil(t, io.EOF)
			},
			expectedLog: "expected nil, but was EOF",
		},
		{
			name: "Nil fails on not nil with format",
			require: func(t TestingT) {
				Nil(t, io.EOF, "pay me %d", 5)
			},
			expectedLog: `expected nil, but was EOF: pay me 5`,
		},
		{
			name: "NoError passes on nil",
			require: func(t TestingT) {
				NoError(t, nil)
			},
		},
		{
			name: "NoError fails on not nil",
			require: func(t TestingT) {
				NoError(t, io.EOF)
			},
			expectedLog: "expected no error, but was EOF",
		},
		{
			name: "NoError fails on not nil with format",
			require: func(t TestingT) {
				NoError(t, io.EOF, "pay me %d", 5)
			},
			expectedLog: `expected no error, but was EOF: pay me 5`,
		},
		{
			name: "NotNil passes on not nil",
			require: func(t TestingT) {
				NotNil(t, io.EOF)
			},
		},
		{
			name: "NotNil fails on nil",
			require: func(t TestingT) {
				NotNil(t, nil)
			},
			expectedLog: "expected to not be nil",
		},
		{
			name: "NotNil fails on nil with format",
			require: func(t TestingT) {
				NotNil(t, nil, "pay me %d", 5)
			},
			expectedLog: `expected to not be nil: pay me 5`,
		},
		{
			name: "False passes on false",
			require: func(t TestingT) {
				False(t, false)
			},
		},
		{
			name: "False fails on true",
			require: func(t TestingT) {
				False(t, true)
			},
			expectedLog: "expected false, but was true",
		},
		{
			name: "False fails on true with format",
			require: func(t TestingT) {
				False(t, true, "pay me %d", 5)
			},
			expectedLog: "expected false, but was true: pay me 5",
		},
		{
			name: "Equal passes on equal: string",
			require: func(t TestingT) {
				Equal(t, "wazero", "wazero")
			},
		},
		{
			name: "Equal passes on equal: []byte",
			require: func(t TestingT) {
				Equal(t, []byte{1, 2, 3, 4}, []byte{1, 2, 3, 4})
			},
		},
		{
			name: "Equal passes on equal: struct",
			require: func(t TestingT) {
				Equal(t, &testStruct{name: "takeshi"}, &testStruct{name: "takeshi"})
			},
		},
		{
			name: "Equal fails on nil: string",
			require: func(t TestingT) {
				Equal(t, "wazero", nil)
			},
			expectedLog: `expected "wazero", but was nil`,
		},
		{
			name: "Equal fails on nil: []byte",
			require: func(t TestingT) {
				Equal(t, []byte{1, 2, 3, 4}, nil)
			},
			expectedLog: `expected []byte{0x1, 0x2, 0x3, 0x4}, but was nil`,
		},
		{
			name: "Equal fails on nil: struct",
			require: func(t TestingT) {
				Equal(t, &testStruct{name: "takeshi"}, nil)
			},
			expectedLog: `expected &require.testStruct{name:"takeshi"}, but was nil`,
		},
		{
			name: "NotEqual passes on not equal",
			require: func(t TestingT) {
				NotEqual(t, uint32(1), uint32(2))
			},
		},
		{
			name: "NotEqual fails on equal: nil",
			require: func(t TestingT) {
				NotEqual(t, nil, nil)
			},
			expectedLog: `expected to not equal <nil>`,
		},
		{
			name: "NotEqual fails on equal: string",
			require: func(t TestingT) {
				NotEqual(t, "wazero", "wazero")
			},
			expectedLog: `expected to not equal "wazero"`,
		},
		{
			name: "NotEqual fails on equal: []byte",
			require: func(t TestingT) {
				NotEqual(t, []byte{1, 2, 3, 4}, []byte{1, 2, 3, 4})
			},
			expectedLog: `expected to not equal []byte{0x1, 0x2, 0x3, 0x4}`,
		},
		{
			name: "NotEqual fails on equal: struct",
			require: func(t TestingT) {
				NotEqual(t, &testStruct{name: "takeshi"}, &testStruct{name: "takeshi"})
			},
			expectedLog: `expected to not equal &require.testStruct{name:"takeshi"}`,
		},
		{
			name: "NotEqual fails on equal: struct with format",
			require: func(t TestingT) {
				NotEqual(t, &testStruct{name: "takeshi"}, &testStruct{name: "takeshi"}, "pay me %d", 5)
			},
			expectedLog: `expected to not equal &require.testStruct{name:"takeshi"}: pay me 5`,
		},
		{
			name: "NotSame passes on not same",
			require: func(t TestingT) {
				NotSame(t, struct1, struct2)
			},
		},
		{
			name: "NotSame passes on different types",
			require: func(t TestingT) {
				NotSame(t, struct1, &zero)
			},
		},
		{
			name: "NotSame fails on same pointers",
			require: func(t TestingT) {
				NotSame(t, struct1, struct1)
			},
			expectedLog: "expected &{hello} to point to a different object",
		},
		{
			name: "NotSame fails on same pointers with format",
			require: func(t TestingT) {
				NotSame(t, struct1, struct1, "pay me %d", 5)
			},
			expectedLog: "expected &{hello} to point to a different object: pay me 5",
		},
		{
			name: "Same passes on same",
			require: func(t TestingT) {
				Same(t, struct1, struct1)
			},
		},
		{
			name: "Same fails on different types",
			require: func(t TestingT) {
				Same(t, struct1, &zero)
			},
			expectedLog: fmt.Sprintf("expected %v to point to the same object as &{hello}", &zero),
		},
		{
			name: "Same fails on different pointers",
			require: func(t TestingT) {
				Same(t, struct1, struct2)
			},
			expectedLog: "expected &{hello} to point to the same object as &{hello}",
		},
		{
			name: "Same fails on different pointers with format",
			require: func(t TestingT) {
				Same(t, struct1, struct2, "pay me %d", 5)
			},
			expectedLog: "expected &{hello} to point to the same object as &{hello}: pay me 5",
		},
		{
			name: "True passes on true",
			require: func(t TestingT) {
				True(t, true)
			},
		},
		{
			name: "True fails on false",
			require: func(t TestingT) {
				True(t, false)
			},
			expectedLog: "expected true, but was false",
		},
		{
			name: "True fails on false with format",
			require: func(t TestingT) {
				True(t, false, "pay me %d", 5)
			},
			expectedLog: "expected true, but was false: pay me 5",
		},
		{
			name: "Zero passes on float32(0)",
			require: func(t TestingT) {
				Zero(t, float32(0))
			},
		},
		{
			name: "Zero passes on float64(0)",
			require: func(t TestingT) {
				Zero(t, float64(0))
			},
		},
		{
			name: "Zero passes on int(0)",
			require: func(t TestingT) {
				Zero(t, int(0))
			},
		},
		{
			name: "Zero passes on uint32(0)",
			require: func(t TestingT) {
				Zero(t, uint32(0))
			},
		},
		{
			name: "Zero passes on uint64(0)",
			require: func(t TestingT) {
				Zero(t, uint64(0))
			},
		},
		{
			name: "Zero fails on float32(1)",
			require: func(t TestingT) {
				Zero(t, float32(1))
			},
			expectedLog: "expected zero, but was 1",
		},
		{
			name: "Zero fails on float64(1)",
			require: func(t TestingT) {
				Zero(t, float64(1))
			},
			expectedLog: "expected zero, but was 1",
		},
		{
			name: "Zero fails on int(1)",
			require: func(t TestingT) {
				Zero(t, int(1))
			},
			expectedLog: "expected zero, but was 1",
		},
		{
			name: "Zero fails on uint32(1)",
			require: func(t TestingT) {
				Zero(t, uint32(1))
			},
			expectedLog: "expected zero, but was 1",
		},
		{
			name: "Zero fails on uint64(1)",
			require: func(t TestingT) {
				Zero(t, uint64(1))
			},
			expectedLog: "expected zero, but was 1",
		},
		{
			name: "Zero fails on uint64(1) with format",
			require: func(t TestingT) {
				Zero(t, uint64(1), "pay me %d", 5)
			},
			expectedLog: "expected zero, but was 1: pay me 5",
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			m := &mockT{t: t}
			tc.require(m)
			m.require(tc.expectedLog)
		})
	}
}

// compile-time check to ensure mockT implements TestingT
var _ TestingT = &mockT{}

type mockT struct {
	t   *testing.T
	log string
}

// Fatal implements TestingT.Fatal
func (t *mockT) Fatal(args ...interface{}) {
	if t.log != "" {
		t.t.Fatal("already called Fatal(")
	}
	t.log = fmt.Sprint(args...)
}

func (t *mockT) require(expectedLog string) {
	if expectedLog != t.log {
		t.t.Fatalf("expected log=%q, but found %q", expectedLog, t.log)
	}
}
