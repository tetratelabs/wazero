const std = @import("std");
const CrossTarget = std.zig.CrossTarget;

pub fn build(b: *std.build.Builder) void {
    // Don't use wasi because this calls os_exit on panic. An ExitError isn't
    // wrapped due to logic in FromRecovered.
    // TODO: Find another way to avoid re-wrapping!
    const target = .{.cpu_arch = .wasm32, .os_tag = .freestanding};
    const optimize = b.standardOptimizeOption(.{});

    const exe = b.addExecutable(.{
        .name = "main",
        .root_source_file = .{ .path = "main.zig" },
        .target = target,
        .optimize = optimize,
    });

    b.installArtifact(exe);
}
