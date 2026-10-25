############################################## ENV VARIABLES
export CONTAINER_IMAGE="alpine"
export CONTAINER_TAG="latest"
export CONTAINER_FULL="$CONTAINER_IMAGE:$CONTAINER_TAG"
export OS=$(fastfetch -s OS --format json | jq -r '.[0].result.name' | tr '[:upper:]' '[:lower:]')
############################################## ENV VARIABLES