all: lzhuf.c lzhuf.h test

jnos2/src/lzhuf.c jnos2/src/lzhuf.h:
	git clone https://github.com/mlangelaar/jnos2.git
	cd jnos2;git reset --hard b2229b973b4094a1c697f480926bf61c21ef4def
	
lzhuf.c lzhuf.h: jnos2/src/lzhuf.c jnos2/src/lzhuf.h
	cp jnos2/src/lzhuf.c jnos2/src/lzhuf.h ./
	patch -p1 < 00_remove_unused_code.patch

test: lzhuf.c lzhuf.h lzhuf.go lzhuf_test.go
	go test .

clean:
	rm -rf jnos2 lzhuf.h lzhuf.c
