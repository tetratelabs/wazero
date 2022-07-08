use std::env;
use std::fs::File;
use std::io;
use std::io::Write;
use std::process::exit;

fn main() {
    // Start at arg[1] because args[0] is the program name.
    for path in env::args().skip(1) {
        if let Ok(mut file) = File::open(&path) {
            io::copy(&mut file, &mut io::stdout()).unwrap();
        } else {
            writeln!(io::stderr(), "error opening: {}: No such file or directory", path).unwrap();
            exit(1);
        }
    }
}
