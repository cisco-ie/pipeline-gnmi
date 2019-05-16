FROM alpine:latest
ARG REV=master 
RUN apk add --no-cache openssl
RUN apk add --no-cache --virtual .build-deps build-base go git openssh
RUN git clone -b "$REV" --single-branch --depth 1 https://github.com/cisco-ie/pipeline-gnmi /pipeline && \
	cd /pipeline && export GOPATH=/pipeline && ln -s vendor src && \
	go build --ldflags "-X main.appVersion=v${REV}(cisco-ie/pipeline-gnmi) -extldflags \"-static\"" -o /bin/pipeline && strip /bin/pipeline && \
	cd / && rm -rf /pipeline
RUN apk del .build-deps
VOLUME /etc/pipeline
CMD ["/etc/pipeline/entrypoint.sh"]
COPY entrypoint.sh metrics.json /etc/pipeline/
