CC = gcc
CFLAGS = -g -O0 -fno-omit-frame-pointer
AFL_CC = afl-gcc

all: simple-crash simple-crash-afl

simple-crash: simple-crash.c
	$(CC) $(CFLAGS) -o simple-crash simple-crash.c

simple-crash-afl: simple-crash.c
	$(AFL_CC) $(CFLAGS) -o simple-crash-afl simple-crash.c

clean:
	rm -f simple-crash simple-crash-afl

.PHONY: all clean