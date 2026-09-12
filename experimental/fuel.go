package experimental

import (
	"context"
	"errors"

	"github.com/tetratelabs/wazero/internal/expctxkeys"
	"github.com/tetratelabs/wazero/internal/fuel"
	"github.com/tetratelabs/wazero/internal/wasmruntime"
)

// ErrOutOfFuel is returned when fuel set via SetFuel is exhausted.
var ErrOutOfFuel = wasmruntime.ErrRuntimeOutOfFuel

// ErrFuelNotSupported is returned when fuel is set but the engine does not
// support metering.
var ErrFuelNotSupported = errors.New("fuel metering requires interpreter runtime")

// SetFuel installs or replaces a fuel budget on ctx. Each dispatched
// operation consumes one unit; execution traps with ErrOutOfFuel when the
// budget is exhausted.
//
// Subsequent SetFuel calls replace the budget in place, so refueling from a
// host function is just another SetFuel call.
//
// Only the interpreter engine honors fuel; the compiler engine returns
// ErrFuelNotSupported.
func SetFuel(ctx context.Context, units uint64) context.Context {
	if m, ok := ctx.Value(expctxkeys.FuelKey{}).(*fuel.Meter); ok {
		m.Set(units)
		return ctx
	}
	return context.WithValue(ctx, expctxkeys.FuelKey{}, fuel.New(units))
}

// GetFuel returns the remaining fuel, or 0 if no fuel is installed or the
// budget is exhausted.
func GetFuel(ctx context.Context) uint64 {
	if m, ok := ctx.Value(expctxkeys.FuelKey{}).(*fuel.Meter); ok {
		return m.Remaining()
	}
	return 0
}

// AddFuel adds units to the fuel budget. No-op if no fuel is installed.
func AddFuel(ctx context.Context, units uint64) {
	if m, ok := ctx.Value(expctxkeys.FuelKey{}).(*fuel.Meter); ok {
		m.Add(units)
	}
}
