FROM golang:1.12-stretch as build
ARG GITHUBORG=cisco-ie
ARG REV=master
ARG GO111MODULE=on
ARG GOPROXY=https://proxy.golang.org
ARG CGO_ENABLED=0
RUN mkdir -p /data/ && cd /data/ && git clone -b "$REV" --single-branch --depth 1 https://github.com/${GITHUBORG}/pipeline-gnmi && \
	cd pipeline-gnmi && make linux/amd64 && strip bin/pipeline_linux_amd64

FROM alpine
RUN apk add --no-cache openssl
VOLUME /etc/pipeline
COPY --from=build /data/pipeline-gnmi/bin/pipeline_linux_amd64 /bin/pipeline
COPY entrypoint.sh metrics.json /etc/pipeline/
RUN chmod +x /etc/pipeline/entrypoint.sh
ENTRYPOINT ["/etc/pipeline/entrypoint.sh"]
