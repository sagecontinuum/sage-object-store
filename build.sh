#!/bin/bash
set -e



echo -n "Checking if there are uncommited changes... "
trap 'echo -e "\033[0;31mFAILED\033[0m"' ERR
git diff-index --quiet HEAD --
trap - ERR
echo -e "\033[0;32mAll set!\033[0m"

set -x


VERSION=`git describe --tags --long`

echo "VERSION: ${VERSION}"
sleep 2

VER_SHORT=$(echo ${VERSION} | cut -d '-' -f 1)
echo "VER_SHORT: ${VER_SHORT}"
# example: 2.1.1

REL_COMMIT_COUNT=$(echo ${VERSION} | cut -d '-' -f 2)
echo "REL_COMMIT_COUNT: ${REL_COMMIT_COUNT}"
# if this is not 0, do not do a release
# example: 2
set +x

if [[ ( "${REL_COMMIT_COUNT}_" != "0_" ) && "$1_" != "--force_" ]] ; then
    echo ""
    echo "Error:"
    echo "  The current git commit has not been tagged. Please create a new tag first to ensure a proper unique version number."
    echo "  Use --force to ignore error (for debugging only)"
    echo ""
    exit 1
fi



docker build --build-arg VERSION=${VERSION} -t waggle/sage-object-store:latest .