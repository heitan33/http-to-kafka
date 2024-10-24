#!/bin/bash
timestamps=`date +"%Y%m%d%H%M"`
tag=$timestamps
podman build -t localhost/httptokafka:1.0 -f ./Dockerfile .
podman tag localhost/httptokafka:1.0 eu-frankfurt-1.ocir.io/xxxxxx/httptokafka:$tag
podman login -u xxxxxx/xxxxxx --password "xxxxxx" eu-frankfurt-1.ocir.io
podman push eu-frankfurt-1.ocir.io/xxxxxx/httptokafka:$tag
