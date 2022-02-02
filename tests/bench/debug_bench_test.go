package bench

import (
	"fmt"
	"io"
	"testing"
)

// These benchmark guard usage at various scopes.
const (
	const_debug_true  = true
	const_debug_false = false
)

var (
	var_debug_true  = true
	var_debug_false = false
	arg             = map[string]string{"foo": "bar"}
)

type obj struct {
	debug bool
}

func (o *obj) fprintf() {
	if o.debug {
		fmt.Fprintf(io.Discard, "arg: %v", arg)
	}
}

func BenchmarkFprintf(b *testing.B) {
	// should be the same as NoOp without a const false guard as the compiler should delete the block
	b.Run("NoOp const false", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if const_debug_false {
				fmt.Fprintf(io.Discard, "arg: %v", arg)
			}
		}
	})
	b.Run("NoOp var false", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if var_debug_false {
				fmt.Fprintf(io.Discard, "arg: %v", arg)
			}
		}
	})
	objFalse := obj{false}
	b.Run("Fprintf obj false", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			objFalse.fprintf()
		}
	})
	// should be the same as NoOp without a const true guard as the compiler should delete the if
	b.Run("Fprintf const true", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if const_debug_true {
				fmt.Fprintf(io.Discard, "arg: %v", arg)
			}
		}
	})
	b.Run("Fprintf var true", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if var_debug_true {
				fmt.Fprintf(io.Discard, "arg: %v", arg)
			}
		}
	})
	objTrue := obj{true}
	b.Run("Fprintf obj true", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			objTrue.fprintf()
		}
	})
}
