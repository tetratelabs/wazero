use std::fs;

fn main() {
    for path in fs::read_dir(".").unwrap() {
        println!("{}", path.unwrap().path().display())
    }
}
