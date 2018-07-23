FROM alpine:3.8
ARG REV=master 
RUN apk add --no-cache openssl && apk add --no-cache --virtual build-deps build-base go git openssh && \
	git clone -b "$REV" --single-branch --depth 1 https://github.com/cisco-ie/pipeline-gnmi /pipeline && \
	cd /pipeline && export GOPATH=/pipeline && ln -s vendor src && \
	go build --ldflags '-extldflags "-static"' -o /bin/pipeline && strip /bin/pipeline && \
	cd / && rm -rf /pipeline && apk del build-deps
VOLUME /etc/pipeline
CMD ["/etc/pipeline/entrypoint.sh"]
COPY entrypoint.sh metrics.json /etc/pipeline/
