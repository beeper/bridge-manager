FROM dock.mau.dev/tulir/lottieconverter:alpine-3.21 AS lottie

FROM golang:1.24-alpine3.21 AS builder

COPY . /build/
RUN cd /build && ./build.sh

FROM alpine:3.21

RUN apk add --no-cache bash curl jq git ffmpeg \
	# Python for python bridges
	python3 py3-pip py3-setuptools py3-wheel \
	# Common dependencies that need native extensions for Python bridges
	py3-magic py3-ruamel.yaml py3-aiohttp py3-pillow py3-olm py3-pycryptodome

COPY --from=lottie /cryptg-*.whl /tmp/
RUN pip3 install --break-system-packages /tmp/cryptg-*.whl && rm -f /tmp/cryptg-*.whl

COPY --from=builder /build/bbctl /usr/local/bin/bbctl
COPY --from=lottie /usr/lib/librlottie.so* /usr/lib/
COPY --from=lottie /usr/local/bin/lottieconverter /usr/local/bin/lottieconverter
COPY ./docker/run-bridge.sh /usr/local/bin/run-bridge.sh
ENV SYSTEM_SITE_PACKAGES=true
VOLUME /data

ENTRYPOINT ["/usr/local/bin/run-bridge.sh"]
