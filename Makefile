all: atomfs

MAIN_VERSION ?= $(shell git describe --always --dirty || echo no-git)
ifeq ($(MAIN_VERSION),$(filter $(MAIN_VERSION), "", no-git))
$(error "Bad value for MAIN_VERSION: '$(MAIN_VERSION)'")
endif

GO_SRC_DIRS := .

GO_SRC := $(shell find $(GO_SRC_DIRS) -name "*.go")

VERSION_LDFLAGS=-X main.Version=$(MAIN_VERSION)

.PHONY: gofmt
gofmt: .made-gofmt
.made-gofmt: $(GO_SRC)
	o=$$(gofmt -l -w $(GO_SRC_DIRS) 2>&1) && [ -z "$$o" ] || \
		{ echo "gofmt made changes: $$o" 1>&2; exit 1; }
	@touch $@

atomfs: .made-gofmt $(GO_SRC)
	go build -buildvcs=false -ldflags "$(VERSION_LDFLAGS)" -o atomfs ./...

clean:
	rm -f atomfs
