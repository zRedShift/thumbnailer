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
	"unsafe"
)

func init() {
	C.init_vips()
}

func getVipsError() error {
	defer C.shutdown_vips_thread_on_error()
	return errors.New(C.GoString(C.vips_error_buffer()))
}

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

func thumbnailFromFile(file *File) error {
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
			os.Remove(filename)
		}(f.Name())
		_, err = io.Copy(f, file)
		if err != nil {
			f.Close()
			return err
		}
		if err = f.Close(); err != nil {
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
		return getVipsError()
	}
	if file.Thumbnail.Path != "" {
		if thumb.has_alpha != 0 {
			file.HasAlpha = true
		}
		file.ThumbCreated = true
		return nil
	}
	defer C.g_free(C.gpointer(thumb.output))
	file.Thumbnail.Width, file.Thumbnail.Height = int(thumb.width), int(thumb.height)
	p := (*[1 << 30]byte)(unsafe.Pointer(thumb.output))[:thumb.output_size:thumb.output_size]
	_, err := file.Write(p)
	if err == nil {
		if thumb.has_alpha != 0 {
			file.HasAlpha = true
		}
		file.ThumbCreated = true
	}
	return err
}
