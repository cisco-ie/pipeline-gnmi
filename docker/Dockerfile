FROM alpine
ARG GITHUBORG=cisco-ie
ARG REV=master
ENV GOPATH=/go
ENV GO111MODULE=on
RUN apk add --no-cache openssl && apk add --no-cache --virtual build-deps build-base go git openssh && \
	mkdir -p /data/ && cd /data/ && git clone -b "$REV" --single-branch --depth 1 https://github.com/${GITHUBORG}/pipeline-gnmi && \
	cd pipeline-gnmi && go build --ldflags '-extldflags "-static"' -o /bin/pipeline && strip /bin/pipeline && \
	cd / && rm -rf /data && apk del build-deps
VOLUME /etc/pipeline
CMD ["/etc/pipeline/entrypoint.sh"]
COPY entrypoint.sh metrics.json /etc/pipeline/
