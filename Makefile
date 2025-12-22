.PHONY: build test clean simulate

BINARY_NAME=fast

build:
	go build -o $(BINARY_NAME) .

test:
	go test ./...

clean:
	rm -f $(BINARY_NAME)
	rm -rf tmp_simulation

simulate: build
	./simulate_download.sh
