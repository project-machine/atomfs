all: gotest atomfs

MAIN_VERSION ?= $(shell git describe --always --dirty || echo no-git)
ifeq ($(MAIN_VERSION),$(filter $(MAIN_VERSION), "", no-git))
$(error "Bad value for MAIN_VERSION: '$(MAIN_VERSION)'")
endif

ROOT := $(shell git rev-parse --show-toplevel)
GO_SRC_DIRS := $(shell find . -name "*.go" | xargs -n1 dirname | sort -u)
GO_SRC := $(shell find . -name "*.go")
VERSION_LDFLAGS=-X main.Version=$(MAIN_VERSION)
BATS = $(TOOLS_D)/bin/bats
BATS_VERSION := v1.10.0
STACKER = $(TOOLS_D)/bin/stacker
STACKER_VERSION := v1.0.0
TOOLS_D := $(ROOT)/tools

export PATH := $(TOOLS_D)/bin:$(PATH)


.PHONY: gofmt
gofmt: .made-gofmt
.made-gofmt: $(GO_SRC)
	o=$$(gofmt -l -w $(GO_SRC_DIRS) 2>&1) && [ -z "$$o" ] || \
		{ echo "gofmt made changes: $$o" 1>&2; exit 1; }
	@touch $@

atomfs: .made-gofmt $(GO_SRC)
	cd $(ROOT)/cmd/atomfs && go build -buildvcs=false -ldflags "$(VERSION_LDFLAGS)" -o $(ROOT)/bin/atomfs ./...

gotest: $(GO_SRC)
	go test -coverprofile=coverage.txt -ldflags "$(VERSION_LDFLAGS)"  ./...

$(STACKER):
	mkdir -p $(TOOLS_D)/bin
	wget --progress=dot:giga https://github.com/project-stacker/stacker/releases/download/$(STACKER_VERSION)/stacker
	chmod +x stacker
	cp stacker $(TOOLS_D)/bin/

$(BATS):
	mkdir -p $(TOOLS_D)/bin
	git clone -b $(BATS_VERSION) https://github.com/bats-core/bats-core.git
	cd bats-core; ./install.sh $(TOOLS_D)
	mkdir -p $(ROOT)/test/test_helper
	git clone --depth 1 https://github.com/bats-core/bats-support $(ROOT)/test/test_helper/bats-support
	git clone --depth 1 https://github.com/bats-core/bats-assert $(ROOT)/test/test_helper/bats-assert
	git clone --depth 1 https://github.com/bats-core/bats-file $(ROOT)/test/test_helper/bats-file

batstest: $(BATS) $(STACKER) atomfs test/random.txt
	cd $(ROOT)/test; sudo $(BATS) --tap --timing priv-*.bats
	cd $(ROOT)/test; $(BATS) --tap --timing unpriv-*.bats

test/random.txt:
	dd if=/dev/random of=/dev/stdout count=2048 | base64 > test/random.txt

.PHONY: test toolsclean
test: gotest batstest

toolsclean:
	rm -rf $(TOOLS_D)
	rm -rf $(ROOT)/test/test_helper
	rm -rf $(ROOT)/bats-core

clean:  toolsclean
	rm -rf $(ROOT)/bin
	rm -f .made-*
