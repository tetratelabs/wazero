const std = @import("std");
const io = std.io;
const os = std.os;
const allocator = std.heap.page_allocator;
const PreopenList = std.fs.wasi.PreopenList;
const stdout = std.io.getStdOut();
const warn = std.log.warn;

pub fn main() !void {
    // Allocate arguments from the the operating system.
    const args = try std.process.argsAlloc(allocator);
    defer std.process.argsFree(allocator, args);

    // Pre-opened files are not available by default. When using WASI, you must
    // manually initialize pre-opens to prevent openFile crashes.
    //
    // See https://ziglang.org/documentation/0.10.0/#WASI
    var preopens = PreopenList.init(allocator);
    defer preopens.deinit();
    try std.os.initPreopensWasi(allocator, "/");

    // loop on the args, skipping the filename (args[0])
    for (args[1..args.len]) |arg| {

        // open the file from its absolute path, as "/" is pre-opened.
        const file = std.fs.openFileAbsolute(arg, .{ .mode = .read_only }) catch |err| {
            warn("Unable to open file {s}: {s}\n", .{ arg, @errorName(err) });
            return err;
        };
        defer file.close();

        // Write the contents to stdout
        stdout.writeFileAll(file, .{}) catch |err| {
            warn("Unable to write contents to stdout: {s}\n", .{@errorName(err)});
            return err;
        };
    }
}
