#include <dirent.h>
#include <errno.h>
#include <stdio.h>
#include <string.h>
#include <unistd.h>
#include <stdbool.h>
#include <sys/select.h>
#include <stdlib.h>
#include <time.h>

#define formatBool(b) ((b) ? "true" : "false")

void main_ls(char *dir_name, bool repeat) {
  DIR *d;
  struct dirent *dir;
  d = opendir(dir_name);
  if (d) {
    while ((dir = readdir(d)) != NULL) {
      printf("./%s\n", dir->d_name);
    }
    if (repeat) {
      rewinddir(d);
      while ((dir = readdir(d)) != NULL) {
        printf("./%s\n", dir->d_name);
      }
    }
    closedir(d);
  } else if (errno == ENOTDIR) {
    printf("ENOTDIR\n");
  } else {
    printf("%s\n", strerror(errno));
  }
}

void main_stat() {
  printf("stdin isatty: %s\n", formatBool(isatty(0)));
  printf("stdout isatty: %s\n", formatBool(isatty(1)));
  printf("stderr isatty: %s\n", formatBool(isatty(2)));
  printf("/ isatty: %s\n", formatBool(isatty(3)));
}

void main_poll(int timeout, int millis) {
  int ret = 0;
  fd_set rfds;
  struct timeval tv;

  FD_ZERO(&rfds);
  FD_SET(0, &rfds);

  tv.tv_sec = timeout;
  tv.tv_usec = millis*1000;
  ret = select(1, &rfds, NULL, NULL, &tv);
  if ((ret > 0) && FD_ISSET(0, &rfds)) {
    printf("STDIN\n");
  } else {
    printf("NOINPUT\n");
  }
}

void main_sleepmillis(int millis) {
   struct timespec tim, tim2;
   tim.tv_sec = 0;
   tim.tv_nsec = millis * 1000000;

   if(nanosleep(&tim , &tim2) < 0 ) {
      printf("ERR\n");
      return;
   }

   printf("OK\n");
}

int main(int argc, char** argv) {
  if (strcmp(argv[1],"ls")==0) {
    bool repeat = false;
    if (argc > 3) {
      repeat = strcmp(argv[3],"repeat")==0;
    }
    main_ls(argv[2], repeat);
  } else if (strcmp(argv[1],"stat")==0) {
    main_stat();
  } else if (strcmp(argv[1],"poll")==0) {
    int timeout = 0;
    int usec = 0;
    if (argc > 2) {
        timeout = atoi(argv[2]);
    }
    if (argc > 3) {
        usec = atoi(argv[3]);
    }
    main_poll(timeout, usec);
  } else if (strcmp(argv[1],"sleepmillis")==0) {
    int timeout = 0;
    if (argc > 2) {
        timeout = atoi(argv[2]);
    }
    main_sleepmillis(timeout);

  } else {
    fprintf(stderr, "unknown command: %s\n", argv[1]);
    return 1;
  }
  return 0;
}
