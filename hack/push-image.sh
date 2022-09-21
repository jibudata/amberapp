#!/bin/bash

rootDir=$(pwd)

echo "root dir: ${rootDir}"

BRANCH=$(git rev-parse --abbrev-ref HEAD)
COMMITID=$(git rev-parse --short HEAD)
TAG=${TAG:-"${BRANCH}.${COMMITID}"}

REGISTRY=${REGISTRY:-"registry.cn-shanghai.aliyuncs.com/jibutech"}

IMAGENAME=${IMAGENAME:-"amberapp"}

FULLTAG="$REGISTRY/$IMAGENAME:$TAG"
DEVTAG="$REGISTRY/$IMAGENAME:${BRANCH}-latest"

# pushd $rootDir
docker push $FULLTAG
if [ $? -ne 0 ];then
    echo "failed to push $FULLTAG"
    exit 1
fi

function docker_push () {
    old_tag=$1
    new_tag=$2
    docker tag $1 $2
    docker push $2
    if [ $? -ne 0 ];then
        echo "failed to push $new_tag "
        exit 1
    fi

    echo "completes to push image $new_tag "
}

docker_push $FULLTAG $DEVTAG




