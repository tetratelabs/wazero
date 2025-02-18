const std = @import("std");
const os = std.os;
const fs = std.fs;
const posix = std.posix;
const allocator = std.heap.page_allocator;
const preopensAlloc = std.fs.wasi.preopensAlloc;
const stdout = std.io.getStdOut().writer();

pub fn main() !void {
    // Allocate arguments from the the operating system.
    const args = try std.process.argsAlloc(allocator);
    defer std.process.argsFree(allocator, args);

    if (std.mem.eql(u8, args[1], "ls")) {
        // TODO: This only looks at fd 3. See #14678
        const dir = std.fs.cwd().openDir(args[2], .{}) catch |err| switch (err) {
            error.NotDir => {
                try stdout.print("ENOTDIR\n", .{});
                return;
            },
            else => {
                try stdout.print("./{}\n", .{err});
                return;
            },
        };

        try ls(dir);
        if (args.len > 3 and std.mem.eql(u8, args[3], "repeat")) {
            try ls(dir);
        }
    } else if (std.mem.eql(u8, args[1], "stat")) {
        try stdout.print("stdin isatty: {}\n", .{posix.isatty(0)});
        try stdout.print("stdout isatty: {}\n", .{posix.isatty(1)});
        try stdout.print("stderr isatty: {}\n", .{posix.isatty(2)});
        try stdout.print("/ isatty: {}\n", .{posix.isatty(3)});
    } else if (std.mem.eql(u8, args[1], "preopen")) {
        const wasi_preopens = try preopensAlloc(allocator);
        // fs.wasi.Preopens does not have a free function

        for (wasi_preopens.names, 0..) |preopen, i| {
            try stdout.print("{}: {s}\n", .{ i, preopen });
        }
    }
}

fn ls(dir: std.fs.Dir) !void {
    var iter = dir.iterate();
    while (try iter.next()) |entry| {
        try stdout.print("./{s}\n", .{entry.name});
    }
}
