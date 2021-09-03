
# docker build -t waggle/sage-object-store:latest .
# docker run -ti --rm -v `pwd`:/app --entrypoint="/bin/bash" waggle/sage-object-store:latest

FROM golang:1.16.5

WORKDIR /app

RUN go get -u gotest.tools/gotestsum


COPY *.go go.* /app/

RUN go build -o server .
EXPOSE 8080



ENTRYPOINT [ "./server" ]
CMD []
