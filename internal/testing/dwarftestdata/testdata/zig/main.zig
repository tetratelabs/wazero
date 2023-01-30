pub fn main() !void {
    @call(.always_inline, inlined_a, .{});
}

fn inlined_a() void {
    @call(.always_inline, inlined_b, .{});
}

fn inlined_b() void {
    unreachable;
}
