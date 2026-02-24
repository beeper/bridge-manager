FROM golang:1.26-alpine3.23 AS builder

COPY . /build/
RUN cd /build && ./build.sh

FROM alpine:3.23

RUN apk add --no-cache bash curl jq git ffmpeg \
	# Python for Google Chat
	python3 py3-pip py3-setuptools py3-wheel \
	# Common dependencies that need native extensions for Python bridges
	py3-magic py3-ruamel.yaml py3-aiohttp py3-pillow py3-olm py3-pycryptodome \
	# Lottieconverter for animated sticker conversion
	lottieconverter

COPY --from=builder /build/bbctl /usr/local/bin/bbctl
COPY ./docker/run-bridge.sh /usr/local/bin/run-bridge.sh
ENV SYSTEM_SITE_PACKAGES=true
VOLUME /data

ENTRYPOINT ["/usr/local/bin/run-bridge.sh"]
