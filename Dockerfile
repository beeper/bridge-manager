# syntax=docker/dockerfile:1

FROM golang

RUN apt-get update && apt-get install -y jq

RUN go install golang.org/x/tools/cmd/goimports@latest
RUN go install honnef.co/go/tools/cmd/staticcheck@latest

COPY . /build
WORKDIR /build

RUN ./build.sh

RUN mkdir /data
RUN mkdir /litefs

CMD export BBCTL_CONFIG=/bbctl.json && jq -n '{environments: {prod: {access_token: env.MATRIX_ACCESS_TOKEN, database_dir: "/litefs", bridge_data_dir: "/data"}}}' > $BBCTL_CONFIG && ./bbctl run $BRIDGE_NAME
