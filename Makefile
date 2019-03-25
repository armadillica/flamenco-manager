OUT := flamenco-manager
PKG := github.com/armadillica/flamenco-manager
VERSION := $(shell git describe --tags --dirty)
PKG_LIST := $(shell go list ${PKG}/... | grep -v /vendor/)
STATIC_OUT := ${OUT}-static-${VERSION}
PACKAGE_PATH := dist/${OUT}-${VERSION}
MONGO_BUNDLES := dist

DEPLOYHOST := biflamanager
DEPLOYPATH := /home/flamanager
SERVICENAME := flamenco-manager.service
SSH := ssh -o ClearAllForwardings=yes


server:
	go build -i -v -o ${OUT} -ldflags="-X main.applicationVersion=${VERSION}" ${PKG}

version:
	@echo "Package: ${PKG}"
	@echo "Version: ${VERSION}"
	@echo "Packaging to: ${PACKAGE_PATH}"

test:
	go test ${PKG_LIST}

vet:
	@go vet ${PKG_LIST}

lint:
	@for file in ${GO_FILES} ;  do \
		golint $$file ; \
	done

run: server
	./${OUT}

clean:
	@go clean -i -x
	git checkout -- static/latest-image.jpg
	rm -f ${OUT}-static-*

static: vet lint
	go build -i -v -o ${STATIC_OUT} -tags netgo -ldflags="-extldflags \"-static\" -w -s -X main.applicationVersion=${VERSION}" ${PKG}

package:
	@$(MAKE) _prepare_package
	@$(MAKE) _package_linux
	@$(MAKE) _package_windows
	@$(MAKE) _package_darwin
	@$(MAKE) _finish_package

package_linux:
	@$(MAKE) _prepare_package
	@$(MAKE) _package_linux
	@$(MAKE) _finish_package

package_windows:
	@$(MAKE) _prepare_package
	@$(MAKE) _package_windows
	@$(MAKE) _finish_package

package_darwin:
	@$(MAKE) _prepare_package
	@$(MAKE) _package_darwin
	@$(MAKE) _finish_package

_package_linux:
	@$(MAKE) --no-print-directory GOOS=linux MONGOOS=linux GOARCH=amd64 STATIC_OUT=${PACKAGE_PATH}/flamenco-manager _package_tar

_package_windows:
	@$(MAKE) --no-print-directory GOOS=windows MONGOOS=windows GOARCH=amd64 STATIC_OUT=${PACKAGE_PATH}/flamenco-manager.exe _package_zip

_package_darwin:
	@$(MAKE) --no-print-directory GOOS=darwin MONGOOS=osx GOARCH=amd64 STATIC_OUT=${PACKAGE_PATH}/flamenco-manager _package_zip

_prepare_package:
	git checkout -- static/latest-image.jpg
	rm -rf ${PACKAGE_PATH}
	mkdir -p ${PACKAGE_PATH}
	rsync static templates ${PACKAGE_PATH} -a --delete-after
	cp -ua flamenco-manager-example.yaml ${PACKAGE_PATH}/
	cp -ua README.md LICENSE.txt CHANGELOG.md ${PACKAGE_PATH}/

_finish_package:
	rm -rf ${PACKAGE_PATH}
	rm -f ${PACKAGE_PATH}.sha256
	sha256sum ${PACKAGE_PATH}* | tee ${PACKAGE_PATH}.sha256

_package_tar: static
ifeq (${GOOS},linux)
	cp flamenco-manager.service ${PACKAGE_PATH}/
endif
	cp -ua --link ${MONGO_BUNDLES}/mongodb-${MONGOOS}-* ${PACKAGE_PATH}/mongodb-${GOOS}
	tar -C $(dir ${PACKAGE_PATH}) -zcf $(PWD)/${PACKAGE_PATH}-${GOOS}.tar.gz $(notdir ${PACKAGE_PATH})
	rm -rf ${STATIC_OUT} ${PACKAGE_PATH}/flamenco-manager.service ${PACKAGE_PATH}/mongodb-${GOOS}

_package_zip: static
	cp -ua --link ${MONGO_BUNDLES}/mongodb-${MONGOOS}-* ${PACKAGE_PATH}/mongodb-${GOOS}
	cd $(dir ${PACKAGE_PATH}) && zip -9 -r -q $(notdir ${PACKAGE_PATH})-${GOOS}.zip $(notdir ${PACKAGE_PATH})
	rm -rf ${STATIC_OUT} ${PACKAGE_PATH}/mongodb-${GOOS}

place: static
	rsync -e "${SSH}" -va ${STATIC_OUT} ${DEPLOYHOST}:${DEPLOYPATH}/${OUT} --delete-after

deploy: static
	@echo "======== Deploying onto ${DEPLOYHOST}"
	${SSH} ${DEPLOYHOST} -t "sudo systemctl stop ${SERVICENAME}"
	rsync -e "${SSH}" -va ${STATIC_OUT} ${DEPLOYHOST}:${DEPLOYPATH}/${OUT} --delete-after
	rsync -e "${SSH}" -va *.md templates static --exclude static/latest-image.jpg ${DEPLOYHOST}:${DEPLOYPATH}/ --delete-after
	rsync -e "${SSH}" -va static/latest-image.jpg ${DEPLOYHOST}:${DEPLOYPATH}/static --ignore-existing --delete-after
	${SSH} ${DEPLOYHOST} -t "sudo systemctl start ${SERVICENAME}"

publish_online: package
	rsync ${PACKAGE_PATH}.sha256 ${PACKAGE_PATH}-linux.tar.gz ${PACKAGE_PATH}-windows.zip ${PACKAGE_PATH}-darwin.zip \
		armadillica@flamenco.io:flamenco.io/download/ -va

.PHONY: run server version static vet lint deploy package _prepare_package _package _package_linux _package_windows _package_darwin _finish_package publish_online
