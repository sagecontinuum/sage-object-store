FROM golang:1.20-alpine AS builder
ARG RELEASE_VERSION
WORKDIR /build
COPY . .
RUN sed -i -e "s/RELEASE_VERSION/${RELEASE_VERSION}/" version.go && \
    CGO_ENABLED=0 go build

# using alpine instead of scratch for x509 root certificates
FROM alpine:3.18
COPY --from=builder /build/sage-object-store /usr/bin/sage-object-store
ENTRYPOINT [ "/usr/bin/sage-object-store" ]
