// Package experimental_test includes examples for experimental features. When these complete, they'll end up as real
// examples in the /examples directory.
package experimental_test

import "context"

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")
