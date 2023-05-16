#include <pthread.h>

pthread_mutex_t mutex;
int count = 0;

void run() {
  pthread_mutex_lock(&mutex);
  count++;
  pthread_mutex_unlock(&mutex);
}

int get() {
  int res;
  pthread_mutex_lock(&mutex);
  res = count;
  pthread_mutex_unlock(&mutex);
  return res;
}
