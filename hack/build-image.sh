#!/bin/bash

rootDir=$(pwd)

echo "root dir: ${rootDir}"

GOPROXY=${GOPROXY:-"https://proxy.golang.org,direct"}

BRANCH=$(git rev-parse --abbrev-ref HEAD)
COMMITID=$(git rev-parse --short HEAD)
TAG=${TAG:-"${BRANCH}.${COMMITID}"}

REGISTRY=${REGISTRY:-"registry.cn-shanghai.aliyuncs.com/jibudata"}
IMAGENAME=${IMAGENAME:-"amberapp"}

DOCKERFILE=${DOCKERFILE:-"$rootDir/Dockerfile"}
FULLTAG="$REGISTRY/$IMAGENAME:$TAG"

git show --oneline -s > VERSION
echo "compiled time: `date`" >> VERSION

echo "$DOCKERFILE $FULLTAG"
docker build -f $DOCKERFILE -t $FULLTAG --build-arg GOPROXY=${GOPROXY} $rootDir
if [ $? -ne 0 ];then
    echo "failed to build $FULLTAG"
    exit 1
fi


