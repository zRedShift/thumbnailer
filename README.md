Thumbnailer
=========
[![GoDoc](https://godoc.org/github.com/zRedShift/thumbnailer?status.svg)](https://godoc.org/github.com/zRedShift/thumbnailer)
[![Build Status](https://travis-ci.org/zRedShift/thumbnailer.svg?branch=master)](https://travis-ci.org/zRedShift/thumbnailer)
[![Codecov](https://codecov.io/gh/zRedShift/thumbnailer/branch/master/graph/badge.svg)](https://codecov.io/gh/zRedShift/thumbnailer/)
[![Go Report Card](https://goreportcard.com/badge/github.com/zRedShift/thumbnailer)](https://goreportcard.com/report/github.com/zRedShift/thumbnailer)

Thumbnailer provides a lightning fast and memory usage efficient image, video and audio (cover art) thumbnailer via
libvips and ffmpeg C bindings, with MIME sniffing (via mimemagic), and streaming I/O support.

## License
[MIT License.](https://github.com/zRedShift/thumbnailer/blob/master/LICENSE)

## API
See the [Godoc](https://godoc.org/github.com/zRedShift/thumbnailer) reference.

## Dependencies
- pkg-config
- libvips 8.7.0+ compiled with libimagequant and all the formats required
- ffmpeg 4.0.2+ compiled with all the formats required
- pthread

All these prerequisites are available in [this](https://hub.docker.com/r/zredshift/thumbnailer/) Docker image. The
Dockerfile is available [here](https://github.com/zRedShift/thumbnailer-docker).