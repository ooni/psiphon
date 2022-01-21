#!/bin/bash
set -ex
basedir=$(cd $(dirname $0) && pwd -P)

rm -rf tunnel-core
mkdir -p tunnel-core
git clone -b staging-client https://github.com/Psiphon-Labs/psiphon-tunnel-core tunnel-core

cd tunnel-core

# As of 2021-01-21, it seems doable to import most Psiphon dependencies directly,
# except for the ones in github.com/Psiphon-Labs. Those seem to have a circular
# dependencies between them and some data structures in the core repository.
mkdir oovendor
for dir in $(cd vendor/github.com/Psiphon-Labs && ls); do
	mv vendor/github.com/Psiphon-Labs/$dir oovendor/$dir
	for file in $(find . -type f -name \*.go); do
		cat $file | sed "s|Psiphon-Labs/$dir|ooni/psiphon/tunnel-core/oovendor/$dir|g" >$file.tmp
		mv $file.tmp $file
	done
done
find oovendor -type f -name go.mod -exec rm {} \;
find oovendor -type f -name go.sum -exec rm {} \;

for file in $(find . -type f -name \*.go); do
	cat $file | sed 's|Psiphon-Labs/psiphon-tunnel-core|ooni/psiphon/tunnel-core|g' >$file.tmp
	mv $file.tmp $file
done

go mod init github.com/ooni/psiphon/tunnel-core
go mod tidy

git describe --tags > VERSION

rm -rf vendor .git
