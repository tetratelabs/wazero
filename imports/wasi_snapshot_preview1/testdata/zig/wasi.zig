const std = @import("std");
const allocator = std.heap.page_allocator;
const preopensAlloc = std.fs.wasi.preopensAlloc;
const stdout = std.io.getStdOut().writer();

pub fn main() !void {
    // Allocate arguments from the the operating system.
    const args = try std.process.argsAlloc(allocator);
    defer std.process.argsFree(allocator, args);

    if (std.mem.eql(u8, args[1], "ls")) {
        _ = try preopensAlloc(allocator);

        var dir = try std.fs.cwd().openIterableDir(".", .{});
        var iter = dir.iterate();
        while (try iter.next()) |entry| {
            try stdout.print("./{s}\n", .{entry.name});
        }
    } else if (std.mem.eql(u8, args[1], "stat")) {
        try stdout.print("stdin isatty: {}\n", .{std.c.isatty(0) != 0});
        try stdout.print("stdout isatty: {}\n", .{std.c.isatty(1) != 0});
        try stdout.print("stderr isatty: {}\n", .{std.c.isatty(2) != 0});
        // TODO: use std.os.isatty and remove the dependency on libc after it's fixed to work on WASI target.
        try stdout.print("/ isatty: {}\n", .{std.c.isatty(3) != 0});
    }
}
