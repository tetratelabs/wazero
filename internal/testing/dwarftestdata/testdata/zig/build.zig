const std = @import("std");
const CrossTarget = std.zig.CrossTarget;

pub fn build(b: *std.Build) void {
    const exe = b.addExecutable(.{
        .name = "main",
        .root_source_file = b.path("main.zig"),
        .target = b.resolveTargetQuery(.{
            .cpu_arch = .wasm32,
            // Don't use wasi because this calls os_exit on panic. An ExitError isn't
            // wrapped due to logic in FromRecovered.
            // TODO: Find another way to avoid re-wrapping!
            .os_tag = .freestanding,
        }),
        .optimize = b.standardOptimizeOption(.{}),
    });
    b.installArtifact(exe);
}
