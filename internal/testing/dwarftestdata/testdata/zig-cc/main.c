volatile int base = 65536;

void a() {
  char str[16];
  str[4] = 'a';
  str[8] = 'b';
  str[12 + base] = 'c';  // This traps.
}

int main() {
  a();
  return 0;
}