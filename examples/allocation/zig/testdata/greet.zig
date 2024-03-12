const std = @import("std");
const allocator = std.heap.page_allocator;

extern "env" fn log(ptr: [*]const u8, size: u32) void;

// _log prints a message to the console using log.
pub fn _log(message: []const u8) void {
    log(message.ptr, message.len);
}

pub export fn malloc(length: usize) ?[*]u8 {
    const buff = allocator.alloc(u8, length) catch return null;
    return buff.ptr;
}

pub export fn free(buf: [*]u8, length: usize) void {
    allocator.free(buf[0..length]);
}

pub fn _greeting(name: []const u8) ![]u8 {
    return try std.fmt.allocPrint(
        allocator,
        "Hello, {s}!",
        .{name},
    );
}

// _greet prints a greeting to the console.
pub fn _greet(name: []const u8) !void {
    const s = try std.fmt.allocPrint(
        allocator,
        "wasm >> {s}",
        .{name},
    );
    _log(s);
}

// greet is a WebAssembly export that accepts a string pointer (linear memory offset) and calls greet.
pub export fn greet(message: [*]const u8, size: u32) void {
    const name = _greeting(message[0..size]) catch |err| @panic(switch (err) {
        error.OutOfMemory => "out of memory",
    });
    _greet(name) catch |err| @panic(switch (err) {
        error.OutOfMemory => "out of memory",
    });
}

// greeting is a WebAssembly export that accepts a string pointer (linear memory
// offset) and returns a pointer/size pair packed into a uint64.
//
// Note: This uses a uint64 instead of two result values for compatibility with
// WebAssembly 1.0.
pub export fn greeting(message: [*]const u8, size: u32) u64 {
    const g = _greeting(message[0..size]) catch return 0;
    return stringToPtr(g);
}

// stringToPtr returns a pointer and size pair for the given string in a way
// compatible with WebAssembly numeric types.
pub fn stringToPtr(s: []const u8) u64 {
    const p: u64 = @intFromPtr(s.ptr);
    return p << 32 | s.len;
}
