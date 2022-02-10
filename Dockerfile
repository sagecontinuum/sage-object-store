FROM golang:1.17 AS builder
ARG RELEASE_VERSION
WORKDIR /build
COPY . .
RUN sed -i -e "s/{RELEASE_VERSION}/${RELEASE_VERSION}/" main.go && \
    CGO_ENABLED=0 go build -ldflags="-s -w" -o sage-object-store

# using alpine instead of scratch for x509 root certificates
FROM alpine:3.15
COPY --from=builder /build/sage-object-store /sage-object-store
ENTRYPOINT [ "/sage-object-store" ]
