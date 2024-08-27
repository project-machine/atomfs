all: atomfs

MAIN_VERSION ?= $(shell git describe --always --dirty || echo no-git)
ifeq ($(MAIN_VERSION),$(filter $(MAIN_VERSION), "", no-git))
$(error "Bad value for MAIN_VERSION: '$(MAIN_VERSION)'")
endif

ROOT := $(shell git rev-parse --show-toplevel)
GO_SRC_DIRS := . $(ROOT)/cmd/atomfs
GO_SRC := $(shell find $(GO_SRC_DIRS) -name "*.go")
VERSION_LDFLAGS=-X main.Version=$(MAIN_VERSION)

.PHONY: gofmt
gofmt: .made-gofmt
.made-gofmt: $(GO_SRC)
	o=$$(gofmt -l -w $(GO_SRC_DIRS) 2>&1) && [ -z "$$o" ] || \
		{ echo "gofmt made changes: $$o" 1>&2; exit 1; }
	@touch $@

atomfs: .made-gofmt $(GO_SRC)
	cd $(ROOT)/cmd/atomfs && go build -buildvcs=false -ldflags "$(VERSION_LDFLAGS)" -o $(ROOT)/bin/atomfs ./...

clean:
	rm -f $(ROOT)/cmd/atomfs/atomfs
	rm .made-*
