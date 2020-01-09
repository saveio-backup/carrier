.DEFAULT_GOAL := build

GOFMT=gofmt
GC=GO111MODULE="off" go build
VERSION := $(shell git describe --abbrev=4 --dirty --always --tags)
Minversion := $(shell date)
IDENTIFIER= $(GOOS)-$(GOARCH)

help:
	@grep -F -h "##" $(MAKEFILE_LIST) | grep -v grep | awk 'BEGIN { FS = ":.*?##" }; { printf "%-15s  %s\n", $$1,$$2 }'

.PHONY: mac
mac: clean
	GOOS=darwin $(GC) -o carrier main.go

.PHONY: windows
windows: clean
	GOOS=windows $(GC) -o carrier main.go

.PHONY: linux
linux: clean
	GOOS=linux $(GC) -o carrier main.go

.PHONY: clean
clean:
	rm -rf carrier
	rm -rf build/

.PHONY: glide
glide:   ## Installs glide for go package management
	@ mkdir -p $$(go env GOPATH)/bin
	@ curl https://glide.sh/get | sh;

vendor: glide.yaml glide.lock
	@ glide install

.PHONY: pb
pb:
	protoc -I=. -I=$(GOPATH)/src -I=$(GOPATH)/src/github.com/gogo/protobuf/protobuf --gogoslick_out=. pb/*.proto

.PHONY: deploy
deploy:
	deploy_test carrier
