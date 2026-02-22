.PHONY: build test gui-test

build:
	go build -o gc .

test:
	go test ./...

gui-test:
	SDL_VIDEODRIVER=dummy go test -tags gui ./...
