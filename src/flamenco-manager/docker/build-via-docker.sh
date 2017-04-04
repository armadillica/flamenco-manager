#!/bin/bash -e

GID=$(id --group)
FLAMENCO_VERSION="2.0.3"

cd "$(dirname "$0")"
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
     golang /bin/bash -e << EOT
echo -n "Using "
go version
cd \${GOPATH}/src/flamenco-manager

function build {
    export GOOS=\$1
    export GOARCH=\$2
    export SUFFIX=\$3

    # GOARCH is always the same, so don't include in filename.
    TARGET=/docker/flamenco-manager-\$GOOS\$SUFFIX

    echo "Building \$TARGET"
    go get -a -ldflags '-s'
    go build -o \$TARGET

    if [ \$GOOS == linux -o \$GOOS == windows ]; then
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
rsync ../static ../templates $PREFIX -a --delete-after --exclude static/latest-image.jpg
cp ../flamenco-manager-example.yaml $PREFIX/
cp ../../../{README.md,LICENSE.txt} $PREFIX/

echo "Creating archive for Linux"
cp flamenco-manager-linux $PREFIX/flamenco-manager
cp ../flamenco-manager.service $PREFIX/
tar zcf $PREFIX-linux.tar.gz $PREFIX/
rm -f $PREFIX/flamenco-manager{,.service}

echo "Creating archive for Windows"
cp flamenco-manager-windows.exe $PREFIX/flamenco-manager.exe
zip -9 -r -q $PREFIX-windows.zip $PREFIX/
rm $PREFIX/flamenco-manager.exe

echo "Creating archive for Darwin"
cp flamenco-manager-darwin $PREFIX/flamenco-manager
zip -9 -r -q $PREFIX-darwin.zip $PREFIX/
rm -f $PREFIX/flamenco-manager

# Clean up after ourselves
rm -rf $PREFIX/

echo "Done building & packaging Flamenco Manager."
