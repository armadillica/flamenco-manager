FROM golang:1.11
MAINTAINER Dr. Sybren <sybren@blender.studio>

ENV GOARCH=amd64
ENV CGO_ENABLED=0

RUN GOOS=linux GOPATH=/go-$GOOS \
    set -e; \
    echo "$GOOS: Making directory $GOPATH"; \
    mkdir ${GOPATH}; \
    echo "$GOOS: Getting dep"; \
    go get -v github.com/golang/dep/cmd/dep; \
    echo "$GOOS: Building everything statically"; \
    cd $GOPATH/src/github.com/golang/dep/cmd/dep; \
    go build -a -tags netgo -ldflags '-w -extldflags "-static"'

RUN GOOS=windows GOPATH=/go-$GOOS \
    set -e; \
    echo "$GOOS: Making directory $GOPATH"; \
    mkdir ${GOPATH}; \
    echo "$GOOS: Getting dep"; \
    go get -v github.com/golang/dep/cmd/dep; \
    echo "$GOOS: Building everything statically"; \
    cd $GOPATH/src/github.com/golang/dep/cmd/dep; \
    go build -a -tags netgo -ldflags '-w -extldflags "-static"'

RUN GOOS=darwin GOPATH=/go-$GOOS \
    set -e; \
    echo "$GOOS: Making directory $GOPATH"; \
    mkdir ${GOPATH}; \
    echo "$GOOS: Getting dep"; \
    go get -v github.com/golang/dep/cmd/dep; \
    echo "$GOOS: Building everything statically"; \
    cd $GOPATH/src/github.com/golang/dep/cmd/dep; \
    go build -a -tags netgo -ldflags '-w -extldflags "-static"'
