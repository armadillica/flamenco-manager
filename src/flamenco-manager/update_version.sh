#!/bin/bash

if [ -z "$1" ]; then
    echo "Usage: $0 new-version" >&2
    exit 1
fi

sed "s/FLAMENCO_VERSION = \"[^\"]*\"/FLAMENCO_VERSION = \"$1\"/" -i main.go

git diff
echo
echo "Don't forget to commit and tag:"
echo git commit -m \'Bumped version to $1\' main.go
echo git tag -a v$1 -m \'Tagged version $1\'
