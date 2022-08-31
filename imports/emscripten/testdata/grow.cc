#include <stdlib.h>
#include <cstdint> // for uint8_t

uint8_t *mem;

int main() {
    mem = (uint8_t *) malloc(64 * 1024 + 1);
    if (mem == NULL) {
        return 1;
    } else {
        return 0;
    }
}
