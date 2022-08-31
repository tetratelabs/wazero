#include <fcntl.h> // constants
#include <unistd.h> // posix
#include <stdio.h> // fprintf

const int BUF_LEN = 512;

// main is the same as wasi _start: "concatenate and print files."
int main(int argc, char** argv)
{
  unsigned char buf[BUF_LEN];
  int fd = 0;
  int len = 0;

  // Start at arg[1] because args[0] is the program name.
  for (int i = 1; i < argc; i++) {
    int fd = open(argv[i], O_RDONLY);
    if (fd < 0) {
      fprintf(stderr, "error opening %s: %d\n", argv[i], fd);
      return 1;
    }

    for (;;) {
      len = read(fd, &buf[0], BUF_LEN);
      if (len > 0) {
        write(STDOUT_FILENO, buf, len);
      } else if (len == 0) {
        break;
      } else {
        fprintf(stderr, "error reading %s\n", argv[i]);
        return 1;
      }
    }
    close(fd);
  }

  return 0;
}
