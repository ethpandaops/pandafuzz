# Makefile for building vulnerable test app with LibFuzzer
CC = clang++
CFLAGS = -g -O1 -fsanitize=fuzzer,address
LDFLAGS = -fsanitize=fuzzer,address

all: vulnerable_app

vulnerable_app: vulnerable_app.c
	$(CC) $(CFLAGS) -o vulnerable_app vulnerable_app.c $(LDFLAGS)

clean:
	rm -f vulnerable_app

.PHONY: all clean