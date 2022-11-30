const std = @import("std");
const allocator = std.heap.page_allocator;
const PreopenList = std.fs.wasi.PreopenList;
const stdout = std.io.getStdOut().writer();

pub fn main() !void {
    var preopens = PreopenList.init(allocator);
    defer preopens.deinit();
    try std.os.initPreopensWasi(allocator, "/");

    var dir = try std.fs.cwd().openIterableDir(".", .{});
    var iter = dir.iterate();
    while (try iter.next()) |entry| {
        try stdout.print("{s}\n", .{entry.name});
    }
}
