#!/bin/bash -e

GID=$(id -g)

cd "$(dirname "$0")"
source _version.sh
echo "Building into $(pwd)"

# Empty -> build & package for all operating systems.
# Non-empty -> build only for this OS, don't package.
TARGET="$1"
if [ ! -z "$TARGET" ]; then
    echo "Only building for $TARGET, not packaging."
fi

if [ -z "$GOPATH" ]; then
    echo "You have to define \$GOPATH." >&2
    exit 2
fi

# Use Docker to get Go in a way that allows overwriting the
# standard library with statically linked versions.
docker build -t flamenco-manager-build .
docker run -i --rm \
    -v $(pwd):/docker \
    -v "${GOPATH}:/go-local" \
    -v $(pwd)/vendor:/go-local/src/github.com/armadillica/flamenco-manager/vendor \
    flamenco-manager-build /bin/bash -e << EOT
echo -n "Using "
go version

function build {
    export GOOS=\$1
    export GOARCH=\$2
    export SUFFIX=\$3

    export GOPATH=/go-\$GOOS

    # Get a copy of the sources, so that building dependencies doesn't
    # swap out the host's vendor directory with a root-owned one.
    mkdir -p \${GOPATH}/src/github.com/armadillica/
    cd \${GOPATH}/src/github.com/armadillica
    cp -a /go-local/src/github.com/armadillica/flamenco-manager .
    cd flamenco-manager

    # GOARCH is always the same, so don't include in filename.
    OUTFILE=/docker/flamenco-manager-\$GOOS\$SUFFIX

    echo "Ensuring vendor directory is accurate"
    \$GOPATH/bin/dep ensure -v -vendor-only

    echo "Building \$OUTFILE"
    go build -tags netgo -ldflags '-w -extldflags "-static"' -o \$OUTFILE

    if [ \$GOOS == linux ]; then
        strip \$OUTFILE
    fi
    chown $UID:$GID \$OUTFILE
}

export CGO_ENABLED=0
if [ -z "$TARGET" -o "$TARGET" = "linux"   ]; then build linux  amd64       ; fi
if [ -z "$TARGET" -o "$TARGET" = "windows" ]; then build windows amd64 .exe ; fi
if [ -z "$TARGET" -o "$TARGET" = "darwin"  ]; then build darwin  amd64      ; fi
EOT

if [ ! -z "$TARGET" ]; then
    echo "Done building Flamenco Manager for $TARGET"
    exit 0
fi

# Package together with the static files
PREFIX="flamenco-manager-$FLAMENCO_VERSION"
if [ -d $PREFIX ]; then
    rm -rf $PREFIX
fi
mkdir $PREFIX

echo "Assembling files into $PREFIX/"
git checkout -- ../static/latest-image.jpg
rsync ../static ../templates $PREFIX -a --delete-after
cp ../flamenco-manager-example.yaml $PREFIX/
cp ../{README.md,LICENSE.txt,CHANGELOG.md} $PREFIX/

if [ -z "$TARGET" -o "$TARGET" = "linux"   ]; then
    echo "Creating archive for Linux"
    cp flamenco-manager-linux $PREFIX/flamenco-manager
    cp ../flamenco-manager.service $PREFIX/
    cp -ua --link mongodb-linux-x86_64-* $PREFIX/mongodb-linux
    tar zcf $PREFIX-linux.tar.gz $PREFIX/
    rm -rf $PREFIX/flamenco-manager{,.service} $PREFIX/mongodb-linux
fi

if [ -z "$TARGET" -o "$TARGET" = "windows" ]; then
    echo "Creating archive for Windows"
    cp flamenco-manager-windows.exe $PREFIX/flamenco-manager.exe
    cp -ua --link mongodb-windows-* $PREFIX/mongodb-windows
    rm -f $PREFIX-windows.zip
    cd $PREFIX
    zip -9 -r -q ../$PREFIX-windows.zip *
    cd -
    rm -rf $PREFIX/flamenco-manager.exe $PREFIX/mongodb-windows
fi

if [ -z "$TARGET" -o "$TARGET" = "darwin"  ]; then
    echo "Creating archive for Darwin"
    cp flamenco-manager-darwin $PREFIX/flamenco-manager
    cp -ua --link mongodb-osx-x86_64-* $PREFIX/mongodb-darwin
    rm -f $PREFIX-darwin.zip
    zip -9 -r -q $PREFIX-darwin.zip $PREFIX/
    rm -rf $PREFIX/flamenco-manager $PREFIX/mongodb-darwin
fi

# Clean up after ourselves
echo "Cleaning up"
rm -rf $PREFIX/

# Create the SHA256 sum file.
echo "Creating sha256 file"
sha256sum flamenco-manager-$FLAMENCO_VERSION-* | tee flamenco-manager-$FLAMENCO_VERSION.sha256

echo "Done building & packaging Flamenco Manager $FLAMENCO_VERSION."
