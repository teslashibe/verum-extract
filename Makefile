.PHONY: build test lint run install clean

build:
	go build -o bin/peptidebase ./cmd/peptidebase

test:
	go test ./... -v

lint:
	go vet ./...

run:
	go run ./cmd/peptidebase run --input=$(INPUT) --output=$(OUTPUT)

install:
	go install ./cmd/peptidebase

clean:
	rm -rf bin/ output/
