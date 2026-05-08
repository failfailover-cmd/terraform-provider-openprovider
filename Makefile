default: build

NAME=openprovider
BINARY=terraform-provider-openprovider

build:
	go build -o $(BINARY)

test:
	go test -v ./...

clean:
	rm -f $(BINARY)

release-snapshot:
	goreleaser release --snapshot --clean
