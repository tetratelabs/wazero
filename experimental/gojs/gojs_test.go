package gojs

import (
	"context"
	"strings"
	"testing"

	"github.com/tetratelabs/wazero/internal/gojs"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestWithOSWorkdir(t *testing.T) {
	t.Parallel()

	ctx, err := WithOSWorkDir(context.Background())
	require.NoError(t, err)
	actual := ctx.Value(gojs.WorkdirKey{}).(string)

	// Check c:\ or d:\ aren't retained.
	require.Equal(t, -1, strings.IndexByte(actual, '\\'))
	require.Equal(t, -1, strings.IndexByte(actual, ':'))
}
