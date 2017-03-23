#!/bin/bash -e

GID=$(id --group)

# Use Docker to get Go in a way that allows overwriting the
# standard library with statically linked versions.
docker run -i --rm \
    -v $(pwd):/docker \
    -v "${GOPATH}:/go-local" \
    --env GOPATH=/go-local \
     golang /bin/bash -e << EOT
go version
set -x
cd \${GOPATH}/src/flamenco-manager

function build {
    export GOOS=\$1
    export GOARCH=\$2
    export SUFFIX=\$3
    TARGET=flamenco-manager-\$GOOS-\$GOARCH\$SUFFIX

    echo "Building \$TARGET"
    go get -a -ldflags '-s'
    go build -o /docker/\$TARGET

    chown $UID:$GID /docker/\$TARGET
}

export CGO_ENABLED=0
build linux amd64
build windows amd64 .exe
build darwin amd64
EOT
