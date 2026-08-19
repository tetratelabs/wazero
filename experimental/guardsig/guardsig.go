//go:build cgo && (darwin || linux) && (amd64 || arm64)

package guardsig

/*
#include <signal.h>
#include <stdatomic.h>
#include <stdint.h>
#include <string.h>

// Registries shared with the signal handler. Writers (Go, via the exported
// functions below) serialize on a mutex; the handler reads lock-free. The
// key field (base / lo) of a slot is written last with release semantics and
// read first with acquire semantics, so a handler that observes a non-zero
// key observes the other fields of that registration.

#define WZ_GS_MAX_REGIONS 512
#define WZ_GS_MAX_STACKS 8192

typedef struct {
	_Atomic uintptr_t base;
	uintptr_t end;
} wz_gs_region;

typedef struct {
	_Atomic uintptr_t lo;
	uintptr_t hi;
	uintptr_t execctx;
	uintptr_t exitseq;
} wz_gs_stack;

static wz_gs_region wz_gs_regions[WZ_GS_MAX_REGIONS];
static wz_gs_stack wz_gs_stacks[WZ_GS_MAX_STACKS];
static struct sigaction wz_gs_prev_segv, wz_gs_prev_bus;
static int wz_gs_ok = 0;

// ---- ucontext accessors ---------------------------------------------------
//
// The handler needs the faulting thread's SP (to identify the wasm call
// stack, and with it the execution context), and rewrites PC to the engine's
// guard-fault exit sequence with the execution context pointer and the
// faulting PC placed in the registers that sequence expects
// (arm64: x0/x1, amd64: rax/rcx).

#if defined(__APPLE__) && defined(__aarch64__)
#include <sys/ucontext.h>
static uintptr_t wz_gs_uc_sp(void *ucv) {
	return (uintptr_t)__darwin_arm_thread_state64_get_sp(((ucontext_t *)ucv)->uc_mcontext->__ss);
}
static uintptr_t wz_gs_uc_pc(void *ucv) {
	return (uintptr_t)__darwin_arm_thread_state64_get_pc(((ucontext_t *)ucv)->uc_mcontext->__ss);
}
static void wz_gs_uc_redirect(void *ucv, uintptr_t exitseq, uintptr_t execctx, uintptr_t faultpc) {
	ucontext_t *uc = (ucontext_t *)ucv;
	uc->uc_mcontext->__ss.__x[0] = execctx;
	uc->uc_mcontext->__ss.__x[1] = faultpc;
	__darwin_arm_thread_state64_set_pc_fptr(uc->uc_mcontext->__ss, (void *)exitseq);
}
#elif defined(__APPLE__) && defined(__x86_64__)
#include <sys/ucontext.h>
static uintptr_t wz_gs_uc_sp(void *ucv) {
	return (uintptr_t)((ucontext_t *)ucv)->uc_mcontext->__ss.__rsp;
}
static uintptr_t wz_gs_uc_pc(void *ucv) {
	return (uintptr_t)((ucontext_t *)ucv)->uc_mcontext->__ss.__rip;
}
static void wz_gs_uc_redirect(void *ucv, uintptr_t exitseq, uintptr_t execctx, uintptr_t faultpc) {
	ucontext_t *uc = (ucontext_t *)ucv;
	uc->uc_mcontext->__ss.__rax = execctx;
	uc->uc_mcontext->__ss.__rcx = faultpc;
	uc->uc_mcontext->__ss.__rip = exitseq;
}
#elif defined(__linux__) && defined(__aarch64__)
#include <ucontext.h>
static uintptr_t wz_gs_uc_sp(void *ucv) {
	return (uintptr_t)((ucontext_t *)ucv)->uc_mcontext.sp;
}
static uintptr_t wz_gs_uc_pc(void *ucv) {
	return (uintptr_t)((ucontext_t *)ucv)->uc_mcontext.pc;
}
static void wz_gs_uc_redirect(void *ucv, uintptr_t exitseq, uintptr_t execctx, uintptr_t faultpc) {
	ucontext_t *uc = (ucontext_t *)ucv;
	uc->uc_mcontext.regs[0] = execctx;
	uc->uc_mcontext.regs[1] = faultpc;
	uc->uc_mcontext.pc = exitseq;
}
#elif defined(__linux__) && defined(__x86_64__)
#include <ucontext.h>
static uintptr_t wz_gs_uc_sp(void *ucv) {
	return (uintptr_t)((ucontext_t *)ucv)->uc_mcontext.gregs[REG_RSP];
}
static uintptr_t wz_gs_uc_pc(void *ucv) {
	return (uintptr_t)((ucontext_t *)ucv)->uc_mcontext.gregs[REG_RIP];
}
static void wz_gs_uc_redirect(void *ucv, uintptr_t exitseq, uintptr_t execctx, uintptr_t faultpc) {
	ucontext_t *uc = (ucontext_t *)ucv;
	uc->uc_mcontext.gregs[REG_RAX] = (greg_t)execctx;
	uc->uc_mcontext.gregs[REG_RCX] = (greg_t)faultpc;
	uc->uc_mcontext.gregs[REG_RIP] = (greg_t)exitseq;
}
#endif

// ---- the handler ----------------------------------------------------------

static void wz_gs_handler(int sig, siginfo_t *info, void *uc) {
	uintptr_t addr = (uintptr_t)info->si_addr;
	int i;
	for (i = 0; i < WZ_GS_MAX_REGIONS; i++) {
		uintptr_t base = atomic_load_explicit(&wz_gs_regions[i].base, memory_order_acquire);
		if (base == 0 || addr < base || addr >= wz_gs_regions[i].end) {
			continue;
		}
		// The fault address is inside a guard region. If the thread is
		// executing wasm (SP inside a registered call stack), this is a
		// guest out-of-bounds access: redirect to the exit sequence.
		{
			uintptr_t sp = wz_gs_uc_sp(uc);
			int j;
			for (j = 0; j < WZ_GS_MAX_STACKS; j++) {
				uintptr_t lo = atomic_load_explicit(&wz_gs_stacks[j].lo, memory_order_acquire);
				if (lo != 0 && sp >= lo && sp < wz_gs_stacks[j].hi) {
					wz_gs_uc_redirect(uc, wz_gs_stacks[j].exitseq, wz_gs_stacks[j].execctx, wz_gs_uc_pc(uc));
					return;
				}
			}
		}
		break;
	}

	// Not a guest out-of-bounds access. Chain to the handler that was
	// installed before us if any; otherwise restore the default action and
	// return, so the fault re-triggers and the process dies with the
	// correct signal disposition.
	{
		struct sigaction *prev = (sig == SIGBUS) ? &wz_gs_prev_bus : &wz_gs_prev_segv;
		if ((prev->sa_flags & SA_SIGINFO) != 0 && prev->sa_sigaction != NULL) {
			prev->sa_sigaction(sig, info, uc);
			return;
		}
		if (prev->sa_handler == SIG_IGN) {
			return;
		}
		if (prev->sa_handler != SIG_DFL && prev->sa_handler != NULL) {
			prev->sa_handler(sig);
			return;
		}
		signal(sig, SIG_DFL);
	}
}

// wz_gs_install is called from the package's Go init function, i.e. after
// the Go runtime installed its own signal handlers. Our handler becomes the
// primary one and chains to the saved Go runtime handler for every fault
// that is not a guest out-of-bounds access, which is the interposition
// pattern documented in os/signal ("Non-Go programs that call Go code" /
// "If the non-Go code installs any signal handlers after the Go runtime is
// initialized, it must save the existing Go signal handler and call it").
// Going through the Go runtime's forwarding in the other direction is not
// possible here: the runtime only forwards synchronous signals raised on
// non-Go threads or inside cgo calls, and JIT-compiled wasm code runs on
// ordinary goroutines.
static int wz_gs_install(void) {
	struct sigaction sa;
	if (wz_gs_ok) {
		return 1;
	}
	memset(&sa, 0, sizeof(sa));
	sa.sa_sigaction = wz_gs_handler;
	sa.sa_flags = SA_SIGINFO | SA_ONSTACK;
	sigemptyset(&sa.sa_mask);
	if (sigaction(SIGSEGV, &sa, &wz_gs_prev_segv) != 0) {
		return 0;
	}
	if (sigaction(SIGBUS, &sa, &wz_gs_prev_bus) != 0) {
		sigaction(SIGSEGV, &wz_gs_prev_segv, NULL);
		return 0;
	}
	wz_gs_ok = 1;
	return 1;
}

// ---- accessors (called from Go) --------------------------------------------
//
// Registration is done by Go code writing directly into these C-owned tables
// (see the Go side below): a cgo call per registration would dominate
// embedders that create many short-lived call engines. The write protocol is
// unchanged: non-key fields first, then the key field with a release store,
// which pairs with the handler's acquire load.

static int wazero_guardsig_install_go(void) { return wz_gs_install(); }

static void *wazero_guardsig_region_table(void) { return wz_gs_regions; }

static void *wazero_guardsig_stack_table(void) { return wz_gs_stacks; }

*/
import "C"

import (
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/tetratelabs/wazero/internal/platform"
)

const (
	maxRegions = 512  // must match WZ_GS_MAX_REGIONS
	maxStacks  = 8192 // must match WZ_GS_MAX_STACKS
)

// regionSlot/stackSlot mirror the C structs. The key field (base/lo) is
// accessed atomically; a slot with key 0 is free.
type (
	regionSlot struct{ base, end uintptr }
	stackSlot  struct{ lo, hi, execctx, exitseq uintptr }
)

// The registries live in C static memory (async-signal-safe for the handler,
// exempt from checkptr/GC concerns for Go). Go writes them directly — no cgo
// call on the registration path.
var (
	mu          sync.Mutex
	regions     []regionSlot
	stacks      []stackSlot
	regionFree  []int32
	stackFree   []int32
	regionIndex map[uintptr]int32
	stackIndex  map[uintptr]int32
)

func init() {
	if C.wazero_guardsig_install_go() == 0 {
		return
	}
	regions = unsafe.Slice((*regionSlot)(C.wazero_guardsig_region_table()), maxRegions)
	stacks = unsafe.Slice((*stackSlot)(C.wazero_guardsig_stack_table()), maxStacks)
	regionFree = make([]int32, 0, maxRegions)
	for i := int32(maxRegions - 1); i >= 0; i-- {
		regionFree = append(regionFree, i)
	}
	stackFree = make([]int32, 0, maxStacks)
	for i := int32(maxStacks - 1); i >= 0; i-- {
		stackFree = append(stackFree, i)
	}
	regionIndex = make(map[uintptr]int32)
	stackIndex = make(map[uintptr]int32)

	platform.SetGuardFaultHandlerHooks(
		func() bool { return true },
		regionAdd, regionDel, stackAdd, stackDel,
	)
}

func regionAdd(base, end uintptr) bool {
	mu.Lock()
	defer mu.Unlock()
	n := len(regionFree)
	if n == 0 {
		return false
	}
	i := regionFree[n-1]
	regionFree = regionFree[:n-1]
	regionIndex[base] = i
	s := &regions[i]
	s.end = end
	atomic.StoreUintptr(&s.base, base) // release: publish after fields
	return true
}

func regionDel(base uintptr) {
	mu.Lock()
	defer mu.Unlock()
	if i, ok := regionIndex[base]; ok {
		atomic.StoreUintptr(&regions[i].base, 0)
		delete(regionIndex, base)
		regionFree = append(regionFree, i)
	}
}

func stackAdd(lo, hi, execCtx, exitSeq uintptr) bool {
	mu.Lock()
	defer mu.Unlock()
	n := len(stackFree)
	if n == 0 {
		return false
	}
	i := stackFree[n-1]
	stackFree = stackFree[:n-1]
	stackIndex[lo] = i
	s := &stacks[i]
	s.hi = hi
	s.execctx = execCtx
	s.exitseq = exitSeq
	atomic.StoreUintptr(&s.lo, lo) // release: publish after fields
	return true
}

func stackDel(lo uintptr) {
	mu.Lock()
	defer mu.Unlock()
	if i, ok := stackIndex[lo]; ok {
		atomic.StoreUintptr(&stacks[i].lo, 0)
		delete(stackIndex, lo)
		stackFree = append(stackFree, i)
	}
}

// Supported returns true when the fault handler is installed and guard-page
// memory will therefore run without per-access bounds checks.
func Supported() bool { return platform.GuardFaultHandlerInstalled() }
