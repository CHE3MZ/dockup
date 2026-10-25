#!/usr/bin/env sh
set -eu

############################################## START
printf '\033[1mSTARTING CONTAINER...\033[0m\n'
container system start 2>/dev/null || true
printf '\033[32mDONE.\033[0m\n'
############################################## START

#CONFIG#
. .ci/platform/lifecycle-config.sh
#CONFIG#

############################################## JOB INT
printf '\033[34mRUNNING JOBS...\033[0m\n'
#======================================# JOB

# PULL CONTAINER IMAGE #
printf '\033[1mPULLING CONTAINER IMAGE...\033[0m\n'
container image pull "$CONTAINER_FULL"
printf '\033[32mDONE.\033[0m\n'

# ========================================= #
printf '\033[1mRUNNING CONTAINER...\033[0m\n'
# RUN CONTAINER STARTED #
