GOTOOLS = \
	github.com/golang/lint/golint \
	github.com/GeertJohan/fgt \
	github.com/mattn/goveralls

all: tools build validate

build:
	go build ./...

vet:
	fgt go vet ./zk

lint:
	fgt golint ./zk

fmt:
	fgt gofmt -l .

test: build validate
	go test -v -race -coverprofile=profile.cov ./zk
	go tool cover -html=profile.cov -o coverage.html

tools:
	go get -u -v $(GOTOOLS)

validate: vet fmt

.PHONY: all vet lint fmt test tools build validate
