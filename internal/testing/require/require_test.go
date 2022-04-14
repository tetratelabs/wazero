package require

import (
	"errors"
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
