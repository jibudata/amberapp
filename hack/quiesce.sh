#!/bin/bash

if [ "$#" -ne 1 ]; then
    echo "Illegal number of parameters"
    echo "Usage: quiesce.sh <hookname>"
    exit 1
fi

hookname=$1

./apphook quiesce -n $hookname -w
