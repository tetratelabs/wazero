const std = @import("std");
const CrossTarget = std.zig.CrossTarget;

pub fn build(b: *std.build.Builder) void {
    // Standard release options allow the person running `zig build` to select
    // between Debug, ReleaseSafe, ReleaseFast, and ReleaseSmall.
    const mode = b.standardReleaseOptions();

    const exe = b.addExecutable("main", "main.zig");
    // Don't use wasi because this calls os_exit on panic. An ExitError isn't
    // wrapped due to logic in FromRecovered.
    // TODO: Find another way to avoid re-wrapping!
    exe.setTarget(CrossTarget{ .cpu_arch = .wasm32, .os_tag = .freestanding });
    exe.setBuildMode(mode);
    exe.install();
}
