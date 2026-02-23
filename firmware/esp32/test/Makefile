# firmware/esp32/test/Makefile
CC = gcc
CFLAGS = -Wall -Wextra -I../main -std=c17

test_proto: test_proto.c ../main/proto.c
	$(CC) $(CFLAGS) -o $@ $^
	./$@

clean:
	rm -f test_proto

.PHONY: clean
