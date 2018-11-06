FROM zredshift/thumbnailer

ENV GO111MODULE=on

WORKDIR $GOPATH/src/github.com/zRedShift/thumbnailer

COPY . .

RUN go mod download