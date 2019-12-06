PROJECT=sysutil
GOPATH ?= $(shell go env GOPATH)

# Ensure GOPATH is set before running build process.
ifeq "$(GOPATH)" ""
  $(error Please set the environment variable GOPATH before running `make`)
endif
FAIL_ON_STDOUT := awk '{ print } END { if (NR > 0) { exit 1 } }'

CURDIR := $(shell pwd)
path_to_add := $(addsuffix /bin,$(subst :,/bin:,$(GOPATH))):$(PWD)/tools/bin
export PATH := $(path_to_add):$(PATH)

GO        := GO111MODULE=on go
GOBUILD   := CGO_ENABLED=1 $(GO) build $(BUILD_FLAG)
GOTEST    := CGO_ENABLED=1 $(GO) test -p 4
OVERALLS  := CGO_ENABLED=1 GO111MODULE=on overalls

ARCH      := "`uname -s`"
LINUX     := "Linux"
MAC       := "Darwin"
PACKAGE_LIST  := go list ./...| grep -vE "cmd"
PACKAGES  := $$($(PACKAGE_LIST))
PACKAGE_DIRECTORIES := $(PACKAGE_LIST) | sed 's|github.com/pingcap/$(PROJECT)/||'
FILES     := $$(find $$($(PACKAGE_DIRECTORIES)) -name "*.go")

FAILPOINT_ENABLE  := $$(find $$PWD/ -type d | grep -vE "(\.git|tools)" | xargs tools/bin/failpoint-ctl enable)
FAILPOINT_DISABLE := $$(find $$PWD/ -type d | grep -vE "(\.git|tools)" | xargs tools/bin/failpoint-ctl disable)

CHECK_LDFLAGS += $(LDFLAGS) ${TEST_LDFLAGS}

TARGET = ""

.PHONY: all update clean test gotest dev check checklist tidy

default: test

# Install the check tools.
check-setup:tools/bin/revive tools/bin/goword tools/bin/gometalinter tools/bin/gosec

check: fmt errcheck lint tidy check-static vet

# These need to be fixed before they can be ran regularly
check-fail: goword check-slow

fmt:
	@echo "gofmt (simplify)"
	@gofmt -s -l -w $(FILES) 2>&1 | $(FAIL_ON_STDOUT)

goword:tools/bin/goword
	tools/bin/goword $(FILES) 2>&1 | $(FAIL_ON_STDOUT)

gosec:tools/bin/gosec
	tools/bin/gosec $$($(PACKAGE_DIRECTORIES))

check-static:tools/bin/gometalinter tools/bin/misspell tools/bin/ineffassign
	@ # TODO: enable megacheck.
	@ # TODO: gometalinter has been DEPRECATED.
	@ # https://github.com/alecthomas/gometalinter/issues/590
	tools/bin/gometalinter --disable-all --deadline 120s \
	  --enable misspell \
	  --enable ineffassign \
	  $$($(PACKAGE_DIRECTORIES))

check-slow:tools/bin/gometalinter tools/bin/gosec
	tools/bin/gometalinter --disable-all \
	  --enable errcheck \
	  $$($(PACKAGE_DIRECTORIES))

errcheck:tools/bin/errcheck
	@echo "errcheck"
	@GO111MODULE=on tools/bin/errcheck -exclude ./tools/check/errcheck_excludes.txt -blank $(PACKAGES) | grep -v "_test\.go" | awk '{print} END{if(NR>0) {exit 1}}'

lint:tools/bin/revive
	@echo "linting"
	@tools/bin/revive -formatter friendly -config tools/check/revive.toml $(FILES)

vet:
	@echo "vet"
	$(GO) vet -all $(PACKAGES) 2>&1 | $(FAIL_ON_STDOUT)

tidy:
	@echo "go mod tidy"
	./tools/check/check-tidy.sh

clean:
	$(GO) clean -i ./...
	rm -rf *.out
	rm -rf parser

test: checklist gotest

gotest: failpoint-enable
ifeq ("$(TRAVIS_COVERAGE)", "1")
	@echo "Running in TRAVIS_COVERAGE mode."
	@export log_level=error; \
	$(GO) get github.com/go-playground/overalls
else
	@echo "Running in native mode."
	@export log_level=error; \
	$(GOTEST) -ldflags '$(TEST_LDFLAGS)' -cover $(PACKAGES) || { $(FAILPOINT_DISABLE); exit 1; }
endif
	@$(FAILPOINT_DISABLE)

race: failpoint-enable
	@export log_level=debug; \
	$(GOTEST) -timeout 20m -race $(PACKAGES) || { $(FAILPOINT_DISABLE); exit 1; }
	@$(FAILPOINT_DISABLE)

failpoint-enable: tools/bin/failpoint-ctl
# Converting gofail failpoints...
	@$(FAILPOINT_ENABLE)

failpoint-disable: tools/bin/failpoint-ctl
# Restoring gofail failpoints...
	@$(FAILPOINT_DISABLE)

tools/bin/megacheck: tools/check/go.mod
	cd tools/check; \
	$(GO) build -o ../bin/megacheck honnef.co/go/tools/cmd/megacheck

tools/bin/revive: tools/check/go.mod
	cd tools/check; \
	$(GO) build -o ../bin/revive github.com/mgechev/revive

tools/bin/goword: tools/check/go.mod
	cd tools/check; \
	$(GO) build -o ../bin/goword github.com/chzchzchz/goword

tools/bin/gometalinter: tools/check/go.mod
	cd tools/check; \
	$(GO) build -o ../bin/gometalinter gopkg.in/alecthomas/gometalinter.v3

tools/bin/gosec: tools/check/go.mod
	cd tools/check; \
	$(GO) build -o ../bin/gosec github.com/securego/gosec/cmd/gosec

tools/bin/errcheck: tools/check/go.mod
	cd tools/check; \
	$(GO) build -o ../bin/errcheck github.com/kisielk/errcheck

tools/bin/failpoint-ctl: go.mod
	$(GO) build -o $@ github.com/pingcap/failpoint/failpoint-ctl

tools/bin/misspell:tools/check/go.mod
	$(GO) get -u github.com/client9/misspell/cmd/misspell

tools/bin/ineffassign:tools/check/go.mod
	cd tools/check; \
	$(GO) build -o ../bin/ineffassign github.com/gordonklaus/ineffassign
