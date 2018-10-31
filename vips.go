package thumbnailer

//#cgo pkg-config: vips
// #cgo CFLAGS: -std=c11 -O3 -DNDEBUG
// #include "vips.h"
import "C"
import "errors"

func (c *Content) thumbnailFromBytes(data *C.uchar) error {
	bands := 3
	if c.AlphaChannel {
		bands++
	}
	if C.thumbnail_from_ffmpeg(data, C.size_t(c.Width * c.Height * bands), C.int(c.Width), C.int(c.Height), C.int(bands)) != 0 {
		return errors.New("vips error")
	}
	return nil
}