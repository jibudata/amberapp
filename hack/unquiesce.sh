#!/bin/bash

if [ "$#" -ne 1 ]; then
    if [[ ! -z "${NAMESPACE}"  ]] && [[ ! -z "${APP_NAME}" ]]; then
        hookname="${NAMESPACE}-${APP_NAME}"
    else
        echo "Illegal number of parameters"
        echo "Usage: unquiesce.sh <hookname>"
        exit 1
    fi
else
    hookname=$1
fi

echo "hook name: ${hookname}"

./apphook unquiesce -n $hookname
