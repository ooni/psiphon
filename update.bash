#!/bin/bash
set -ex
basedir=$(cd $(dirname $0) && pwd -P)

oope=github.com/ooni/psiphon
psirootdir=oopsi
psidir=$psirootdir/github.com/Psiphon-Labs/psiphon-tunnel-core
psitempdir=psi.temp

rm -rf $psirootdir $psitempdir
mkdir -p $psirootdir
git clone -b staging-client https://github.com/Psiphon-Labs/psiphon-tunnel-core $psitempdir

cd $psitempdir

# Remove all go modules because we're vendoring sources
for file in go.mod go.sum; do
  find . -type f -name $file -exec rm -f {} \;
done

# Move all vendored dependencies at toplevel
for dir in $(find vendor -type d -maxdepth 1 -mindepth 1); do
  mv $dir $basedir/$psirootdir/$(basename $dir)
done

# Record the version of psiphon-tunnel-core we're using
git describe --tags > $basedir/$psirootdir/ooversion.txt

# We are using another git repository
rm -rf vendor .git

# Also move psiphon-tunnel-core sources in the right place
cd ..
mv $psitempdir $basedir/$psidir

cd $basedir/$psirootdir

# Rewrite all the imports so the dependencies used by Psiphon have a
# specific import path that does not conflict with OONI's deps.
for file in $(find . -type f -name \*.go); do
  cat $file | sed -e "s@github.com/@$oope/$psirootdir/github.com/@g"   \
                  -e "s@go.uber.org/@$oope/$psirootdir/go.uber.org/@g" \
                  -e "s@golang.org/@$oope/$psirootdir/golang.org/@g"   > $file.new
  mv $file.new $file
done

git add .

# Quirk: we need to disable QUIC when using Go 1.14 because there is conflict
# between the qtls version vendored by Psiphon and Go 1.15 stdlib.
for file in $(git grep PSIPHON_DISABLE_QUIC|awk -F: '{print $1}'|sort -u); do
  cat $file | sed -e "s@PSIPHON_DISABLE_QUIC@go1.15@g" > $file.new
  mv $file.new $file
done

git add .
