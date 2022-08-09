const std = @import("std");
const allocator = std.heap.page_allocator;

// main is required for Zig to compile to enable WASI.
pub fn main() anyerror!void {
    return;
}

extern "env" fn log(ptr: u32, size: u32) void;

// _log prints a message to the console using log.
pub fn _log(message: []u8) void {
    const ptrSize = stringToPtr(message);
    var p = ptrSize >> 32;
    log(@truncate(u32, p), @truncate(u32, ptrSize));
}

pub export fn malloc(size: usize) usize {
    var memory = allocator.alloc(u8, size) catch unreachable;
    return @ptrToInt(memory.ptr);
}

pub export fn free(ptr: usize) void {
    var s: []u8 = undefined;
    s.ptr = @intToPtr([*]u8, ptr);
    allocator.free(s);
}

pub fn _greeting(name: []u8) ![]u8 {
    return try std.fmt.allocPrint(
        allocator,
        "Hello, {s}!",
        .{name},
    );
}

// _greet prints a greeting to the console.
pub fn _greet(name: []u8) void {
    const g = _greeting(name);
    const s = std.fmt.allocPrint(
        allocator,
        "wasm >> {s}",
        .{g},
    ) catch unreachable;
    _log(s);
}

// greet is a WebAssembly export that accepts a string pointer (linear memory offset) and calls greet.
pub export fn greet(ptr: u32, size: u32) void {
    const name = ptrToString(ptr, size);
    _greet(name);
}

// greeting is a WebAssembly export that accepts a string pointer (linear memory
// offset) and returns a pointer/size pair packed into a uint64.
//
// Note: This uses a uint64 instead of two result values for compatibility with
// WebAssembly 1.0.
pub export fn greeting(ptr: u32, size: u32) u64 {
    const name = ptrToString(ptr, size);
    const g = _greeting(name) catch unreachable;
    return stringToPtr(g);
}

// ptrToString returns a string from WebAssembly compatible numeric types
// representing its pointer and length.
pub fn ptrToString(ptr: u32, size: u32) []u8 {
    var s: []u8 = undefined;
    s.ptr = @intToPtr([*]u8, ptr);
    s.len = size;
    return s;
}

// stringToPtr returns a pointer and size pair for the given string in a way
// compatible with WebAssembly numeric types.
pub fn stringToPtr(s: []const u8) u64 {
    const p: u64 = @ptrToInt(s.ptr);
    return p << 32 | s.len;
}
