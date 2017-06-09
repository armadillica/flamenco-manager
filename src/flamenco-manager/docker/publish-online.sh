#!/bin/bash -e

source _version.sh
echo "Uploading Flamenco Manager $FLAMENCO_VERSION to flamenco.io"

PREFIX="flamenco-manager-$FLAMENCO_VERSION"
rsync $PREFIX-linux.tar.gz $PREFIX-windows.zip $PREFIX-darwin.zip armadillica@flamenco.io:flamenco.io/download/ -va

echo "Done!"
