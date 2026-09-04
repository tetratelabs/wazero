// Source for hello_world_kt.wasm — a Kotlin/Wasm WASI binary.
// From https://github.com/Kotlin/kotlin-wasm-wasi-template
//
// To rebuild: clone the template repo and run:
//   ./gradlew wasmWasiNodeProductionRun
// The .wasm output lands under build/compileSync/wasmWasi/main/productionExecutable/

import kotlin.wasm.WasmImport
import kotlin.wasm.unsafe.Pointer
import kotlin.wasm.unsafe.UnsafeWasmMemoryApi
import kotlin.wasm.unsafe.withScopedMemoryAllocator

fun main() {
    println("Hello from Kotlin via WASI")
    println("Current 'realtime' timestamp is: ${wasiRealTime()}")
    println("Current 'monotonic' timestamp is: ${wasiMonotonicTime()}")
}

@WasmImport("wasi_snapshot_preview1", "clock_time_get")
private external fun wasiRawClockTimeGet(clockId: Int, precision: Long, resultPtr: Int): Int

private const val REALTIME = 0
private const val MONOTONIC = 1

@OptIn(UnsafeWasmMemoryApi::class)
fun wasiGetTime(clockId: Int): Long = withScopedMemoryAllocator { allocator ->
    val rp0 = allocator.allocate(8)
    val ret = wasiRawClockTimeGet(
        clockId = clockId,
        precision = 1,
        resultPtr = rp0.address.toInt()
    )
    check(ret == 0) {
        "Invalid WASI return code $ret"
    }
    (Pointer(rp0.address.toInt().toUInt())).loadLong()
}

fun wasiRealTime(): Long = wasiGetTime(REALTIME)

fun wasiMonotonicTime(): Long = wasiGetTime(MONOTONIC)

// We need it to run WasmEdge with the _initialize function
@WasmExport
fun dummy() {}
