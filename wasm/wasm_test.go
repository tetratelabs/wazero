package wasm

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncodeDecodeF32(t *testing.T) {
	for _, v := range []float32{
		0, 100, -100, 1, -1,
		100.01234124, -100.01234124, 200.12315,
		math.MaxFloat32,
		math.SmallestNonzeroFloat32,
		float32(math.Inf(1)), float32(math.Inf(-1)), float32(math.NaN()),
	} {
		t.Run(fmt.Sprintf("%f", v), func(t *testing.T) {
			encoded := EncodeF32(v)
			decoded := DecodeF32(encoded)
			if math.IsNaN(float64(decoded)) { // NaN cannot be compared with themselves, so we have to use IsNaN
				require.True(t, math.IsNaN(float64(decoded)))
			} else {
				require.Equal(t, v, decoded)
			}
		})
	}
}

func TestEncodeDecodeF64(t *testing.T) {
	for _, v := range []float64{
		0, 100, -100, 1, -1,
		100.01234124, -100.01234124, 200.12315,
		math.MaxFloat32,
		math.SmallestNonzeroFloat32,
		math.MaxFloat64,
		math.SmallestNonzeroFloat64,
		6.8719476736e+10,  /* = 1 << 36 */
		1.37438953472e+11, /* = 1 << 37 */
		math.Inf(1), math.Inf(-1), math.NaN(),
	} {
		t.Run(fmt.Sprintf("%f", v), func(t *testing.T) {
			encoded := EncodeF64(v)
			decoded := DecodeF64(encoded)
			if math.IsNaN(decoded) { // cannot use require.Equal as NaN by definition doesn't equal itself
				require.True(t, math.IsNaN(decoded))
			} else {
				require.Equal(t, v, decoded)
			}
		})
	}
}
