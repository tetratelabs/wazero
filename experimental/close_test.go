package experimental_test

import (
	"context"
	"testing"

	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/expctxkeys"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

type arbitrary struct{}

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), arbitrary{}, "arbitrary")

func TestWithCloseNotifier(t *testing.T) {
	tests := []struct {
		name         string
		notification experimental.CloseNotifier
		expected     bool
	}{
		{
			name:     "returns input when notification nil",
			expected: false,
		},
		{
			name:         "decorates with notification",
			notification: experimental.CloseNotifyFunc(func(context.Context, uint32) {}),
			expected:     true,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			if decorated := experimental.WithCloseNotifier(testCtx, tc.notification); tc.expected {
				require.NotNil(t, decorated.Value(expctxkeys.CloseNotifierKey{}))
			} else {
				require.Same(t, testCtx, decorated)
			}
		})
	}
}
