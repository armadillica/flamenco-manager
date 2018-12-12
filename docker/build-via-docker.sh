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
docker run -i --rm \
    -v $(pwd):/docker \
    -v "${GOPATH}:/go-local" \
    --env GOPATH=/go-local \
     golang:1.11 /bin/bash -e << EOT
echo -n "Using "
go version
cd \${GOPATH}/src/github.com/armadillica/flamenco-manager

function build {
    export GOOS=\$1
    export GOARCH=\$2
    export SUFFIX=\$3

    # GOARCH is always the same, so don't include in filename.
    TARGET=/docker/flamenco-manager-\$GOOS\$SUFFIX

    echo "Building \$TARGET"
    go get -ldflags '-s' github.com/golang/dep/cmd/dep
    \$GOPATH/bin/dep ensure
    go build -o \$TARGET

    if [ \$GOOS == linux ]; then
        strip \$TARGET
    fi
    chown $UID:$GID \$TARGET
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

echo "Creating archive for Linux"
cp flamenco-manager-linux $PREFIX/flamenco-manager
cp ../flamenco-manager.service $PREFIX/
cp -ua --link mongodb-linux-x86_64-* $PREFIX/mongodb-linux
tar zcf $PREFIX-linux.tar.gz $PREFIX/
rm -rf $PREFIX/flamenco-manager{,.service} $PREFIX/mongodb-linux

echo "Creating archive for Windows"
cp flamenco-manager-windows.exe $PREFIX/flamenco-manager.exe
cp -ua --link mongodb-windows-* $PREFIX/mongodb-windows
rm -f $PREFIX-windows.zip
cd $PREFIX
zip -9 -r -q ../$PREFIX-windows.zip *
cd -
rm -rf $PREFIX/flamenco-manager.exe $PREFIX/mongodb-windows

echo "Creating archive for Darwin"
cp flamenco-manager-darwin $PREFIX/flamenco-manager
cp -ua --link mongodb-osx-x86_64-* $PREFIX/mongodb-darwin
rm -f $PREFIX-darwin.zip
zip -9 -r -q $PREFIX-darwin.zip $PREFIX/
rm -rf $PREFIX/flamenco-manager $PREFIX/mongodb-darwin

# Clean up after ourselves
rm -rf $PREFIX/

# Create the SHA256 sum file.
sha256sum flamenco-manager-$FLAMENCO_VERSION-* | tee flamenco-manager-$FLAMENCO_VERSION.sha256

echo "Done building & packaging Flamenco Manager $FLAMENCO_VERSION."
