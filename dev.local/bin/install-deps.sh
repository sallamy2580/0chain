#!/bin/bash

mkdir ./build && cd ./build

root=$(pwd)

echo ""
echo "1> Build and install rocksdb"
echo ""

wget -O - https://github.com/facebook/rocksdb/archive/v6.15.5.tar.gz | tar xz 
cd ./rocksdb* && \
PORTABLE=1 make -j $(nproc) install-shared OPT=-g0 USE_RTTI=1 
cd /usr/local/lib/
ln -fs librocksdb.6.15.5.dylib /usr/local/lib/-mmacosx-version-min=12.0librocksdb.6.15.dylib
ln -fs librocksdb.6.15.5.dylib /usr/local/lib/-mmacosx-version-min=10.12librocksdb.6.15.dylib



echo ""
echo "2> Build and install herumi/mcl"
echo ""
cd $root
wget -O - https://github.com/herumi/mcl/archive/master.tar.gz | tar xz
mv mcl* mcl
make -C mcl -j $(nproc) lib/libmclbn256.dylib install 

echo ""
echo "3> Build and install herumi/bls"
echo ""
cd $root
wget -O - https://github.com/herumi/bls/archive/master.tar.gz | tar xz 
mv bls* bls
make MCL_DIR=../mcl -C bls -j $(nproc) install 


echo ""
echo "4> install openssl@1.1"
echo ""
cd $root
brew install openssl@1.1