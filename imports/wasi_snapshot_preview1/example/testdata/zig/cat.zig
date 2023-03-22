const std = @import("std");
const io = std.io;
const os = std.os;
const allocator = std.heap.page_allocator;
const stdout = std.io.getStdOut();
const warn = std.log.warn;

pub fn main() !void {
    // Allocate arguments from the the operating system.
    const args = try std.process.argsAlloc(allocator);
    defer std.process.argsFree(allocator, args);

    // loop on the args, skipping the filename (args[0])
    for (args[1..args.len]) |arg| {

        // open the file from a relative path, as "/" is pre-opened and the CWD.
        const file = std.fs.cwd().openFile(arg, .{ .mode = .read_only }) catch |err| {
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
