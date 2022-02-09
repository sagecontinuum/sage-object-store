FROM golang:1.17 AS builder
WORKDIR /build
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o sage-object-store

# using alpine base image for x509 certificates
FROM alpine:3.15
COPY --from=builder /build/sage-object-store /sage-object-store
ENTRYPOINT [ "/sage-object-store" ]

# # build.sh
# # docker run -ti --rm -v `pwd`:/app --entrypoint="/bin/bash" waggle/sage-object-store:latest
# # or docker run -ti --rm -p 80:80 --env-file=.env waggle/sage-object-store:latest


# FROM golang:1.16.5
# ARG VERSION

# WORKDIR /app

# RUN go get -u gotest.tools/gotestsum


# COPY *.go go.* /app/
# COPY /vendor /app/vendor/

# RUN sed -i -e 's/\[\[VERSION\]\]/'${VERSION}'/' server.go
# RUN cat server.go | grep version

# RUN go build -o server .

# EXPOSE 8080



# ENTRYPOINT [ "/app/server" ]
# CMD []
