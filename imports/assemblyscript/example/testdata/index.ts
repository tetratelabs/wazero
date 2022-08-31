export function hello_world(arg: u32): u32 {
  trace("hello world", 2, arg + 0.1, arg + 0.2);
  return arg + 3;
}

export function goodbye_world(): void {
  throw new Error("sad sad world");
}
