#!/bin/bash

if [ "$#" -ne 1 ]; then
    echo "Illegal number of parameters"
    echo "Usage: unquiesce.sh <hookname>"
    exit 1
fi

hookname=$1

./apphook unquiesce -n $hookname
