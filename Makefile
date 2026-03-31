.PHONY: build test lint run install clean

build:
	go build -o bin/verum-extract ./cmd/verum-extract

test:
	go test ./... -v

lint:
	go vet ./...

run:
	go run ./cmd/verum-extract run --input=$(INPUT) --output=$(OUTPUT)

install:
	go install ./cmd/verum-extract

clean:
	rm -rf bin/ output/
