use std::env;
use std::fs;
use std::io;
use std::io::{Read,Write};
use std::net::{TcpListener};
use std::os::wasi::io::FromRawFd;
use std::process::exit;
use std::str::from_utf8;
use std::error::Error;

use std::collections::HashMap;
use std::time::Duration;

// Until NotADirectory is implemented, read the underlying error raised by
// wasi-libc. See https://github.com/rust-lang/rust/issues/86442
use libc::ENOTDIR;

fn main() {
    let args: Vec<String> = env::args().collect();

    match args[1].as_str() {
        "ls" => {
            main_ls(&args[2]);
            if args.len() == 4 && args[3].as_str() == "repeat" {
                main_ls(&args[2]);
            }
        }
        "stat" => main_stat(),
        "sock" => main_sock(),
        "mixed" => main_mixed().unwrap(),
        _ => {
            writeln!(io::stderr(), "unknown command: {}", args[1]).unwrap();
            exit(1);
        }
    }
}

fn main_ls(dir_name: &String) {
    match fs::read_dir(dir_name) {
        Ok(paths) => {
            for ent in paths.into_iter() {
                println!("{}", ent.unwrap().path().display());
            }
        }
        Err(e) => {
            if let Some(error_code) = e.raw_os_error() {
                if error_code == ENOTDIR {
                    println!("ENOTDIR");
                } else {
                    println!("errno=={}", error_code);
                }
            } else {
                writeln!(io::stderr(), "failed to read directory: {}", e).unwrap();
            }
        }
    }
}

extern crate libc;

fn main_stat() {
    unsafe {
        println!("stdin isatty: {}", libc::isatty(0) != 0);
        println!("stdout isatty: {}", libc::isatty(1) != 0);
        println!("stderr isatty: {}", libc::isatty(2) != 0);
        println!("/ isatty: {}", libc::isatty(3) != 0);
    }
}

fn main_sock() {
    // Get a listener from the pre-opened file descriptor.
    // The listener is the first pre-open, with a file-descriptor of 3.
    let listener = unsafe { TcpListener::from_raw_fd(3) };
    for conn in listener.incoming() {
        match conn {
            Ok(mut conn) => {
                // Do a blocking read of up to 32 bytes.
                // Note: the test should write: "wazero", so that's all we should read.
                let mut data = [0 as u8; 32];
                match conn.read(&mut data) {
                    Ok(size) => {
                        let text = from_utf8(&data[0..size]).unwrap();
                        println!("{}", text);

                        // Exit instead of accepting another connection.
                        exit(0);
                    },
                    Err(e) => writeln!(io::stderr(), "failed to read data: {}", e).unwrap(),
                } {}
            }
            Err(e) => writeln!(io::stderr(), "failed to read connection: {}", e).unwrap(),
        }
    }
}

fn main_mixed() -> Result<(), Box<dyn Error>> {

    use mio::net::{TcpListener, TcpStream};
    use mio::{Events, Interest, Poll, Token};

    // Some tokens to allow us to identify which event is for which socket.
    const SERVER: Token = Token(0);
    const STDIN: Token = Token(1);

    // Create a poll instance.
    let mut poll = Poll::new()?;
    // Create storage for events.
    let mut events = Events::with_capacity(128);

    let mut server = unsafe { TcpListener::from_raw_fd(3) };
    let mut stdin = unsafe { TcpStream::from_raw_fd(0) };


    // Start listening for incoming connections.
    poll.registry()
        .register(&mut server, SERVER, Interest::READABLE)?;

    // Keep track of incoming connections.
    let mut m: HashMap<Token, TcpStream> = HashMap::new();

    let mut count = 2;

    // Start an event loop.
    loop {
        // Poll Mio for events, blocking until we get an event.
        if let Err(e) = poll.poll(&mut events, Some(Duration::from_nanos(0))) {
            // Ignore EINTR.
            if e.kind() == std::io::ErrorKind::Interrupted {
                continue;
            }
            return Err(Box::from(e))
        }

        // Process each event.
        for event in events.iter() {
            // We can use the token we previously provided to `register` to
            // determine for which socket the event is.
            match event.token() {
                    SERVER => {
                    // If this is an event for the server, it means a connection
                    // is ready to be accepted.
                    //
                    // Accept the connection and add it to the map.
                    match server.accept() {
                        Ok((mut connection, _addr)) => {
                            let tok = Token(count);
                            _ = poll.registry()
                                .register(&mut connection, tok, Interest::READABLE);
                            m.insert(tok, connection);
                            // drop(connection);
                            count+=1;
                        },
                        Err(e) if e.kind() == std::io::ErrorKind::WouldBlock => {
                            // ignore
                        },
                        Err(err) => panic!("ERROR! {}", err),
                    }
                },

                STDIN => {
                    // There is for reading on one of our connections, read it and echo.
                    let mut buf = [0u8; 32];
                    match stdin.read(&mut buf) {
                        Ok(n) if n>0 =>
                            println!("{}", String::from_utf8_lossy(&buf[0..n])),
                        _ => {} // ignore error.
                    }
                },


                conn_id => {
                    // There is for reading on one of our connections, read it and echo.
                    let mut buf = [0u8; 32];
                    let mut el = m.get(&conn_id).unwrap();
                    match el.read(&mut buf) {
                        Ok(n) if n>0 => {
                            let s = String::from_utf8_lossy(&buf[0..n]);
                            println!("{}", s);
                            // Quit when the socket contains the string wazero.
                            if s.contains("wazero") {
                                return Ok(());
                            }
                        },

                        _ => {} // ignore error.
                    }
                }
            }
        }

    }
}


