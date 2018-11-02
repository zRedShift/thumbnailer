package thumbnailer

//#cgo pkg-config: vips
// #cgo CFLAGS: -std=c11 -O3 -DNDEBUG
// #include "vips.h"
import "C"
import (
	"errors"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"sync"
	"unsafe"
)

func init() {
	C.init_vips()
}

type vipsError struct {
	sync.Mutex
	errSlice []string
}

func (v *vipsError) error() error {
	v.Lock()
	defer v.Unlock()
	errSlice := strings.Split(C.GoString(C.vips_error_buffer()), "\n")
	C.shutdown_vips_thread_on_error()
	v.errSlice = append(errSlice[:len(errSlice)-1], v.errSlice...)
	if len(v.errSlice) == 0 {
		return errors.New("vips: failed to fetch error")
	}
	err := errors.New("vips: " + v.errSlice[0])
	v.errSlice = v.errSlice[1:]
	return err
}

var vErr = &vipsError{errSlice: make([]string, 0, 10)}

func thumbnailFromFFmpeg(file *File, data *C.uchar) error {
	thumb := C.RawThumbnail{
		width:       C.int(file.Width),
		height:      C.int(file.Height),
		target_size: C.int(file.TargetDimensions),
		input:       data,
		quality:     C.int(file.Quality),
		bands:       3,
	}
	if file.HasAlpha {
		thumb.bands++
	}
	thumb.input_size = C.size_t(thumb.bands * thumb.height * thumb.width)
	return handleThumbnailOutput(file, &thumb)
}

func thumbnailFromFile(file *File) (err error) {
	thumb := C.RawThumbnail{
		target_size: C.int(file.TargetDimensions),
		quality:     C.int(file.Quality),
	}
	if file.Path != "" {
		thumb.input_path = C.CString(file.Path)
		defer C.free(unsafe.Pointer(thumb.input_path))
	} else {
		f, err := ioutil.TempFile(os.TempDir(), "")
		if err != nil {
			return err
		}
		thumb.input_path = C.CString(f.Name())
		defer func(filename string) {
			C.free(unsafe.Pointer(thumb.input_path))
			rErr := os.Remove(filename)
			if err == nil {
				err = rErr
			}
		}(f.Name())
		_, err = io.Copy(f, file)
		if cErr := f.Close(); err != nil || cErr != nil {
			if err == nil {
				return cErr
			}
			return err
		}
	}
	return handleThumbnailOutput(file, &thumb)
}

func handleThumbnailOutput(file *File, thumb *C.RawThumbnail) error {
	if file.Thumbnail.Path != "" {
		thumb.output_path = C.CString(file.Thumbnail.Path)
		defer C.free(unsafe.Pointer(thumb.output_path))
	}
	if C.thumbnail(thumb) != 0 {
		return vErr.error()
	}
	file.Thumbnail.Width, file.Thumbnail.Height = int(thumb.thumb_width), int(thumb.thumb_height)
	if thumb.has_alpha != 0 {
		file.HasAlpha = true
	}
	if file.Width == 0 || file.Height == 0 {
		file.Width, file.Height = int(thumb.width), int(thumb.height)
	}
	if file.Thumbnail.Path != "" {
		file.ThumbCreated = true
		return nil
	}
	defer C.g_free(C.gpointer(thumb.output))
	p := (*[1 << 30]byte)(unsafe.Pointer(thumb.output))[:thumb.output_size:thumb.output_size]
	_, err := file.Write(p)
	if err == nil {
		file.ThumbCreated = true
	}
	return err
}
