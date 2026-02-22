.PHONY: build test gui-test clean

build:
	go build -o gc .

test:
	go test ./...

gui-test:
	SDL_VIDEODRIVER=dummy go test -tags gui ./...

clean:
	rm -f gc sdl-alt-test sdl-alt-test.test
	rm -f leap*.txt testdata/leap*.txt
