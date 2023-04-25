fn main() {
    a();
}

#[inline(always)]
fn a() {
    b(42);
}

fn b<A: std::fmt::Debug>(x: A) {
    println!("{:?}", x);
    panic!("unreachable");
}
