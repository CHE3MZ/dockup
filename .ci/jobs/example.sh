#!/usr/bin/env sh
set -eu

#CONFIG#
. .ci/platform/lifecycle-config.sh
#CONFIG#

#START#
sh .ci/platform/lifecycle-start.sh
#START#

container run --rm "$CONTAINER_FULL" echo "hello world"

#STOP#
sh .ci/platform/lifecycle-stop.sh
#STOP#