#!/bin/bash
# This is only for velero hook integration, which needs to add annotations to 
# application pod, to specify hook container and commands.
# As the apphook binary is executing inside application pod, we need a way to
# apply required cluster role to the command, which need the hack below.

if [ "$#" -ne 4 ]; then
    echo "Illegal number of parameters"
    echo "Usage: ./hack/prepare.sh <namespace> <app> <hookname> <database>"
    exit 1
fi

namespace=$1
appname=$2
hookname=$3
dbname=$4

echo "remove hook"
apphook delete -n $hookname

echo "remove annotation of pod"
kubectl annotate pod -n $namespace -l app=$appname pre.hook.backup.velero.io/command-
kubectl annotate pod -n $namespace -l app=$appname pre.hook.backup.velero.io/container-
kubectl annotate pod -n $namespace -l app=$appname post.hook.backup.velero.io/command-
kubectl annotate pod -n $namespace -l app=$appname post.hook.backup.velero.io/container-

podname=`kubectl get pod -n $namespace -l app=$appname | grep -v NAME | awk '{print $1}'`
echo "pod name: $podname"

servicename=`kubectl get svc -n $namespace | grep -v NAME | awk '{print $1}'`

endpoint=$servicename"."$namespace
echo "endpoint name: $endpoint"

echo "create hook"
apphook create -n $hookname -a mysql -e $endpoint -u root -p passw0rd --databases $dbname

# Hack: copy the root config to the container
kubectl cp ~/.kube/config -n $namespace -c app-hook $podname:/root/

echo "annotate pod"
kubectl annotate pod -n $namespace -l app=$appname \
    pre.hook.backup.velero.io/command='["/bin/bash", "-c", "./quiesce.sh"]' \
    pre.hook.backup.velero.io/container=app-hook \
    post.hook.backup.velero.io/command='["/bin/bash", "-c", "./unquiesce.sh"]' \
    post.hook.backup.velero.io/container=app-hook

