use std::env;
use std::fs;
use std::io;
use std::io::Write;
use std::process::exit;

fn main() {
    let args: Vec<String> = env::args().collect();

    match args[1].as_str() {
        "ls" => {
            main_ls();
         },
        "stat" => {
            main_stat();
         },
         _ => {
             writeln!(io::stderr(), "unknown command: {}", args[1]).unwrap();
             exit(1);
         }
    }
}

fn main_ls() {
    for path in fs::read_dir(".").unwrap() {
        println!("{}", path.unwrap().path().display())
    }
}

extern crate libc;

fn main_stat() {
  unsafe{
    println!("stdin isatty: {}", libc::isatty(0) != 0);
    println!("stdout isatty: {}", libc::isatty(1) != 0);
    println!("stderr isatty: {}", libc::isatty(2) != 0);
    println!("/ isatty: {}", libc::isatty(3) != 0);
  }
}
