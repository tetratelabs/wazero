const std = @import("std");
const CrossTarget = std.zig.CrossTarget;

pub fn build(b: *std.Build) void {
    const exe = b.addExecutable(.{
        .name = "wasi",
        .root_source_file = b.path("wasi.zig"),
        .target = b.resolveTargetQuery(.{
            .cpu_arch = .wasm32,
            .os_tag = .wasi,
        }),
        .optimize = b.standardOptimizeOption(.{}),
    });
    b.installArtifact(exe);
}
