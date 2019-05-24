FROM alpine
ARG GITHUBORG=cisco-ie
ARG REV=master
ENV GOPATH=/go
ENV GO111MODULE=off
RUN apk add --no-cache openssl && apk add --no-cache --virtual build-deps build-base go git openssh && \
	mkdir -p /go/src/github.com/cisco-ie && cd /go/src/github.com/cisco-ie && git clone -b "$REV" --single-branch --depth 1 https://github.com/${GITHUBORG}/pipeline-gnmi && \
	cd pipeline-gnmi && go build --ldflags '-extldflags "-static"' -o /bin/pipeline && strip /bin/pipeline && \
	cd / && rm -rf /pipeline && apk del build-deps
VOLUME /etc/pipeline
CMD ["/etc/pipeline/entrypoint.sh"]
COPY entrypoint.sh metrics.json /etc/pipeline/
