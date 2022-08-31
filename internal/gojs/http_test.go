package gojs_test

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/tetratelabs/wazero"
	gojs "github.com/tetratelabs/wazero/imports/go"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

type roundTripperFunc func(r *http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func Test_http(t *testing.T) {
	t.Parallel()

	ctx := gojs.WithRoundTripper(testCtx, roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/error" {
			return nil, errors.New("error")
		}
		if req.Body != nil {
			require.Equal(t, http.MethodPost, req.Method)
			bytes, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			require.Equal(t, "ice cream", string(bytes))
		}
		return &http.Response{
			StatusCode:    http.StatusOK,
			Status:        http.StatusText(http.StatusOK),
			Header:        http.Header{"Custom": {"1"}},
			Body:          io.NopCloser(strings.NewReader("abcdef")),
			ContentLength: 6,
		}, nil
	}))

	stdout, stderr, err := compileAndRun(ctx, "http", wazero.NewModuleConfig().
		WithEnv("BASE_URL", "http://host"))

	require.EqualError(t, err, `module "" closed with exit_code(0)`)
	require.Zero(t, stderr)
	require.Equal(t, `Get "http://host/error": net/http: fetch() failed: error
1
abcdef
`, stdout)
}
