const std = @import("std");
const CrossTarget = std.zig.CrossTarget;

pub fn build(b: *std.Build) void {
    const lib = b.addExecutable(.{
        .name = "greet",
        .root_source_file = b.path("greet.zig"),
        .target = b.resolveTargetQuery(.{
            .cpu_arch = .wasm32,
            .os_tag = .freestanding,
        }),
        .optimize = b.standardOptimizeOption(.{}),
    });
    lib.entry = .disabled;
    lib.rdynamic = true;
    b.installArtifact(lib);
}
