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
UCLCLIENTSO?=libuclclient.so
UCLCLIENTH?=libuclclient.h
INSTALL_LIBDIR?=$(GOPATH)/pkg/libuclclient

.PHONY: uclclientlib
uclclientlib:
	set -e ; \
	GOOS=${GOOS} GOARCH=${GOARCH} CGO_ENABLED=1 CC=${CC} $(GO) build -buildmode=c-shared -o $(UCLCLIENTSO) $(GOBUILDFLAGS) -ldflags '-extldflags "-Wl,-soname=$(UCLCLIENTSO)"' . ; \
	install -o $(USER) -g $(USER) -m 755 -d $(INSTALL_LIBDIR) ; \
	install -o $(USER) -g $(USER) -m 664 $(UCLCLIENTSO) $(INSTALL_LIBDIR)/$(UCLCLIENTSO) ; \
	install -o $(USER) -g $(USER) -m 664 $(UCLCLIENTH) $(INSTALL_LIBDIR)/$(UCLCLIENTH) ; \
	rm $(UCLCLIENTSO) $(UCLCLIENTH)

.PHONY: test
test: $(TEST_MODULES)
	set -e;\
	$(GO) test .

.PHONY: fmt
fmt: $(FMT_MODULES)
	set -e;\
	$(GO) fmt .

.PHONY: lint
lint: $(LINT_MODULES)
	set -e;\
	$(GO) vet .

.PHONY: doc
doc: $(DOC_MODULES)
	set -e;\
	$(GO) doc .

.PHONY: clean
clean: $(CLEAN_MODULES)
	set -e;\
	$(GO) clean -v .
