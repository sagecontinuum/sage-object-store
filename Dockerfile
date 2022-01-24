
# build.sh
# docker run -ti --rm -v `pwd`:/app --entrypoint="/bin/bash" waggle/sage-object-store:latest
# or docker run -ti --rm -p 80:80 --env-file=.env waggle/sage-object-store:latest


FROM golang:1.16.5
ARG VERSION

WORKDIR /app

RUN go get -u gotest.tools/gotestsum


COPY *.go go.* /app/
COPY /vendor /app/vendor/

RUN sed -i -e 's/\[\[VERSION\]\]/'${VERSION}'/' server.go
RUN cat server.go | grep version

RUN go build -o server .

EXPOSE 8080



ENTRYPOINT [ "/app/server" ]
CMD []
