.PHONY: all
all: bin/shark

.PHONY: clean
clean:
	rm -rf bin/*

bin/shark: cmd/shark/main.go cli/*.go command/*.go
	go build -o bin/shark cmd/shark/main.go
