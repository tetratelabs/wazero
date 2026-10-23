package experimental

import (
	"context"

	"github.com/tetratelabs/wazero/internal/expctxkeys"
)

// TableLimiter controls the number of elements allocated for WebAssembly tables.
// A limiter may be shared across module instances to enforce an aggregate budget.
// This does not bound Go heap usage: backing slice capacity, allocation overhead,
// compiled code and objects retained by table references are not accounted for.
// Linear memory is controlled separately by MemoryAllocator.
//
// Methods must be safe for concurrent use and must not panic. Release can run on
// a runtime cleanup goroutine concurrently with Grow.
type TableLimiter interface {
	// Grow is called before allocating a table or increasing its length. current
	// and desired are element counts, not bytes. maximum is the module's declared
	// maximum, or math.MaxUint32 if none was declared.
	//
	// Returning true reserves desired-current elements. Returning false must
	// leave the budget unchanged: instantiation fails, or table.grow returns -1.
	// Initial allocation calls Grow even when desired is zero. Subsequent growth
	// calls it only for a nonzero delta within the table's WebAssembly limits.
	Grow(current, desired, maximum uint32) bool

	// Release returns a table's entire reservation, including successful growth,
	// after the table becomes unreachable. It is called once per admitted table.
	// Imported aliases share the original table's reservation and limiter.
	//
	// Release is asynchronous and depends on garbage collection. Closing a
	// module or failing instantiation does not immediately release its tables:
	// other instances or retained module handles may still keep them reachable.
	// Release is not guaranteed before process exit. Implementations must not
	// retain references to the modules whose tables they account for, as that
	// could prevent collection and hence release.
	Release(elements uint32)
}

// WithTableLimiter registers limiter for tables defined by a module instantiated
// with the returned context. The limiter remains attached to those tables for
// their lifetime, including growth through imports or with a different context.
// It does not attach a limiter to imported tables or previously created tables.
func WithTableLimiter(ctx context.Context, limiter TableLimiter) context.Context {
	if limiter != nil {
		return context.WithValue(ctx, expctxkeys.TableLimiterKey{}, limiter)
	}
	return ctx
}
