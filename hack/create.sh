#!/bin/bash

if [ "$#" -ne 6 ]; then
    echo "Illegal number of parameters, need 6, actual: $#"
    echo "Usage: create.sh <hookname> <provider> <endpoint> <database> <username> <password>"
    exit 1
fi

hookname=$1
provider=$2
endpoint=$3
dbname=$4
username=$5
password=$6

echo "create hook"
./apphook create -n $hookname -a $provider -e $endpoint -u $username -p $password --databases $dbname
