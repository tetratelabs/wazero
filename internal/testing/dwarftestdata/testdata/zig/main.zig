pub fn main() !void {
    @call(.{ .modifier = .always_inline }, inlined_a, .{});
}

fn inlined_a() void {
    @call(.{ .modifier = .always_inline }, inlined_b, .{});
}

fn inlined_b() void {
    unreachable;
}
