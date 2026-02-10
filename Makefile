GO ?= /usr/local/go/bin/go
BINARY := jira

.PHONY: build clean

build:
	CGO_ENABLED=0 $(GO) build -o $(BINARY) .

clean:
	rm -f $(BINARY)
