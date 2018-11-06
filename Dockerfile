FROM zredshift/thumbnailer
WORKDIR $GOPATH/src/github.com/zRedShift/thumbnailer
COPY . .
RUN go get -v ./...