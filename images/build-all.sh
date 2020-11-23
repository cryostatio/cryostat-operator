#!/bin/sh

set -x
set -e

for i in $(find . -name build.sh); do
	echo "Building image with $i..."
	sh $i
done
