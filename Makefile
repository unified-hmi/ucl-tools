# SPDX-License-Identifier: Apache-2.0
#
# Copyright (c) 2024  Panasonic Automotive Systems, Co., Ltd.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

#CURDIR := $(dir $(lastword $(MAKEFILE_LIST)))

GO?=go
GOBUILDFLAGS?=-v

THIS_DIR=.
MODULES=cmd \
	internal

INSTALL_MODULES=$(patsubst %,install-%, $(MODULES))
CLEAN_MODULES=$(patsubst %,clean-%, $(MODULES))
FMT_MODULES=$(patsubst %,fmt-%, $(MODULES))
LINT_MODULES=$(patsubst %,lint-%, $(MODULES))
DOC_MODULES=$(patsubst %,doc-%, $(MODULES))

.PHONY: all install $(INSTALL_MODULES)
all : install
install : $(INSTALL_MODULES) uclclientlib

$(INSTALL_MODULES):
	set -e;\
	$(MAKE) mod ;\
	target=`echo $@ | sed -e 's/install-//'`;\
	make -C $${target} install

.PHONY: fmt $(FMT_MODULES)
fmt: $(FMT_MODULES)

$(FMT_MODULES) :
	set -e;\
	target=`echo $@ | sed -e 's/fmt-//'`;\
	make -C $${target} fmt

.PHONY: lint $(LINT_MODULES)
lint: $(LINT_MODULES)

$(LINT_MODULES):
	set -e;\
	target=`echo $@ | sed -e 's/lint-//'`;\
	make -C $${target} lint

.PHONY: doc $(DOC_MODULES)
doc: $(DOC_MODULES)

$(DOC_MODULES):
	set -e;\
	target=`echo $@ | sed -e 's/doc-//'`;\
	make -C  $${target} doc

.PHONY: clean $(CLEAN_MODULES)
clean: $(CLEAN_MODULES)

$(CLEAN_MODULES):
	set -e;\
	target=`echo $@ | sed -e 's/clean-//'`;\
	make -C $${target} clean


.PHONY:uclclientlib
uclclientlib:
	set -e;\
	make GOOS=${GOOS} GOARCH=${GOARCH} CC=${CC} -C pkg/ucl-client-lib $@

.PHONY: dep
dep:

GO_VERSION := $(shell go version | awk '{print $$3}')
GO_MAJOR_MINOR := $(shell echo "$(GO_VERSION)" | sed -E 's/go([0-9]+)\.([0-9]+).*/\1.\2/')

ifeq ($(GO_MAJOR_MINOR),1.13)
  GRPC_VERSION := v1.38.1
  PROTOC_GO_VERSION := v1.31.0
  PROTOC_GO_GRPC_VERSION := v1.1.0
else ifeq ($(GO_MAJOR_MINOR),1.14)
  GRPC_VERSION := v1.38.1
  PROTOC_GO_VERSION := v1.31.0
  PROTOC_GO_GRPC_VERSION := v1.2.0
else ifeq ($(GO_MAJOR_MINOR),1.15)
  GRPC_VERSION := v1.38.1
  PROTOC_GO_VERSION := v1.31.0
  PROTOC_GO_GRPC_VERSION := v1.2.0
else ifeq ($(GO_MAJOR_MINOR),1.16)
  GRPC_VERSION := v1.38.1
  PROTOC_GO_VERSION := v1.31.0
  PROTOC_GO_GRPC_VERSION := v1.2.0
else ifeq ($(GO_MAJOR_MINOR),1.17)
  GRPC_VERSION := v1.57.2
  PROTOC_GO_VERSION := v1.34.0
  PROTOC_GO_GRPC_VERSION := v1.3.0
else ifeq ($(GO_MAJOR_MINOR),1.18)
  GRPC_VERSION := v1.57.2
  PROTOC_GO_VERSION := v1.34.0
  PROTOC_GO_GRPC_VERSION := v1.3.0
else ifeq ($(GO_MAJOR_MINOR),1.19)
  GRPC_VERSION := v1.64.1
  PROTOC_GO_VERSION := v1.34.0
  PROTOC_GO_GRPC_VERSION := v1.3.0
else ifeq ($(GO_MAJOR_MINOR),1.20)
  GRPC_VERSION := v1.64.1
  PROTOC_GO_VERSION := v1.34.0
  PROTOC_GO_GRPC_VERSION := v1.3.0
else ifeq ($(GO_MAJOR_MINOR),1.21)
  GRPC_VERSION := v1.67.3
  PROTOC_GO_VERSION := v1.36.0
  PROTOC_GO_GRPC_VERSION := v1.5.0
else ifeq ($(GO_MAJOR_MINOR),1.22)
  GRPC_VERSION := v1.71.3
  PROTOC_GO_VERSION := v1.36.0
  PROTOC_GO_GRPC_VERSION := v1.5.0
else ifeq ($(GO_MAJOR_MINOR),1.23)
  GRPC_VERSION := v1.73.0
  PROTOC_GO_VERSION := v1.36.0
  PROTOC_GO_GRPC_VERSION := v1.5.0
else
  GRPC_VERSION := latest
  PROTOC_GO_VERSION := latest
  PROTOC_GO_GRPC_VERSION := latest
endif

ifeq ($(shell echo "$(GO_MAJOR_MINOR) >= 1.16" | bc) ,1)
  GO_INSTALL_CMD := go install
else
  GO_INSTALL_CMD := go get
  export GO111MODULE=on
endif

.PHONY: mod
mod:
	set -e ;\
	if [ ! -f go.mod ]; then go mod init ucl-tools; fi ;\
	go get google.golang.org/grpc@${GRPC_VERSION}; \
	go mod tidy

.PHONY: proto
proto:
	set -e ;\
	protoc --go_out=proto --go-grpc_out=proto proto/dcm.proto ;\

.PHONY: install-protoc-tools
install-protoc-tools:
	set -e ;\
	$(GO_INSTALL_CMD) google.golang.org/grpc/cmd/protoc-gen-go-grpc@${PROTOC_GO_GRPC_VERSION} ;\
	$(GO_INSTALL_CMD) google.golang.org/protobuf/cmd/protoc-gen-go@${PROTOC_GO_VERSION}

export GO_UCL=/tmp/go-ucl
.PHONY: install-protoc-tools-go1.20
install-protoc-tools-go1.20:
	set -e ;\
	mkdir -p ${GO_UCL} ;\
	wget -P ${GO_UCL} https://go.dev/dl/go1.20.14.linux-amd64.tar.gz ;\
	tar -zxvf ${GO_UCL}/go1.20.14.linux-amd64.tar.gz -C ${GO_UCL} ;\
	${GO_UCL}/go/bin/go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.3.0 ;\
	${GO_UCL}/go/bin/go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.34.0 ;\
	rm -rf ${GO_UCL}

###################
### custom build

###linux
CUSTOM_GOOS?=linux

###x86_64 64-bit
CUSTOM_GOARCH?=amd64

###arm64 64-bit
#CUSTOM_GOARCH?=arm64

ifeq ($(CUSTOM_GOOS), linux)
    ifeq ($(CUSTOM_GOARCH), amd64)
        CUSTOM_CC=gcc
    else ifeq ($(CUSTOM_GOARCH), arm64)
        CUSTOM_CC=aarch64-linux-gnu-gcc
    endif
endif

.PHONY: custom_all
custom_all :
	set -e;\
	make GOOS=${CUSTOM_GOOS} GOARCH=${CUSTOM_GOARCH} CC=${CUSTOM_CC} all
  
