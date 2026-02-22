.PHONY: build test gui-test clean

build:
	go build -o gc .

test:
	go test ./...

gui-test:
	SDL_VIDEODRIVER=dummy go test -tags gui ./...

clean:
	rm -f gc gc.test
	rm -f leap*.txt testdata/leap*.txt
	rm - f qk
