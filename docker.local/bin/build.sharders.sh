#!/bin/bash
set -e

GIT_COMMIT=$(git rev-list -1 HEAD)
echo "$GIT_COMMIT"

# generate mocks
make build-mocks

cmd="build"

docker $cmd --build-arg GIT_COMMIT="$GIT_COMMIT" -f docker.local/build.sharder/Dockerfile . -t sharder

for i in $(seq 1 3);
do
  SHARDER=$i docker-compose -p sharder$i -f docker.local/build.sharder/docker-compose.yml build --force-rm
done

docker.local/bin/sync_clock.sh
