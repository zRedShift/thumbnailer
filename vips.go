package thumbnailer

//#cgo pkg-config: vips
// #cgo CFLAGS: -std=c11
// #include "vips.h"
// #include "stdlib.h"
import "C"
import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"github.com/zRedShift/seekstream"
)

var (
	vipsMu    sync.Mutex
	initiated bool
)

func vipsCheckLeaks() {
	C.vips_leak_set(1)
}

func vipsPrintAll() {
	C.vips_object_print_all()
}

// InitVIPS initializes vips.
func InitVIPS() {
	vipsMu.Lock()
	defer vipsMu.Unlock()
	if C.init_vips() != 0 {
		panic(fmt.Sprintf("couldn't start vips: %v", vErr.error()))
	}
	initiated = true
}

// ShutdownVIPS shuts vips down.
func ShutdownVIPS() {
	vipsMu.Lock()
	C.shutdown_vips()
	initiated = false
	vipsMu.Unlock()
}

// VIPSCacheSetMaxMem Sets the maximum amount of tracked memory vips allows before it starts dropping cached operations.
func VIPSCacheSetMaxMem(maxMem int) {
	if !initiated {
		InitVIPS()
	}
	C.vips_cache_set_max_mem(C.size_t(maxMem))
}

// VIPSCacheSetMax sets the maximum number of operations vips keeps in cache.
func VIPSCacheSetMax(max int) {
	if !initiated {
		InitVIPS()
	}
	C.vips_cache_set_max(C.int(max))
}

// VIPSCacheSetMaxFiles Sets the maximum number of tracked files vips allows before it starts dropping cached
// operations.
func VIPSCacheSetMaxFiles(maxFiles int) {
	if !initiated {
		InitVIPS()
	}
	C.vips_cache_set_max_files(C.int(maxFiles))
}

// DropAllVIPSCache drops the whole operation cache. Called automatically on ShutdownVips().
func DropAllVIPSCache() {
	if !initiated {
		InitVIPS()
	}
	C.vips_cache_drop_all()
}

type vipsError struct {
	sync.Mutex
	errSlice []string
	lastErr  error
}

func (v *vipsError) error() error {
	v.Lock()
	defer v.Unlock()
	errSlice := strings.Split(C.GoString(C.vips_error_buffer()), "\n")
	if len(errSlice) > 1 {
		C.vips_error_clear()
	}
	v.errSlice = append(errSlice[:len(errSlice)-1], v.errSlice...)
	if len(v.errSlice) != 0 {
		v.lastErr = errors.New("vips: " + v.errSlice[0])
		v.errSlice = v.errSlice[1:]
	}
	return v.lastErr
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
		orientation: C.int(file.Orientation),
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
	} else if f, ok := file.Reader.(*seekstream.File); ok {
		f.Wait()
		thumb.input_path = C.CString(f.Name())
	} else {
		f, err := ioutil.TempFile(os.TempDir(), "")
		if err != nil {
			return err
		}
		thumb.input_path = C.CString(f.Name())
		defer func(filename string) {
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
	defer C.free(unsafe.Pointer(thumb.input_path))
	return handleThumbnailOutput(file, &thumb)
}

func handleThumbnailOutput(file *File, thumb *C.RawThumbnail) error {
	runtime.LockOSThread()
	defer func() {
		C.vips_thread_shutdown()
		runtime.UnlockOSThread()
	}()
	if file.Thumbnail.Path != "" {
		thumb.output_path = C.CString(file.Thumbnail.Path)
		defer C.free(unsafe.Pointer(thumb.output_path))
	}
	if !initiated {
		InitVIPS()
	}
	if C.thumbnail(thumb) != 0 {
		return vErr.error()
	}
	file.Thumbnail.Width, file.Thumbnail.Height = int(thumb.thumb_width), int(thumb.thumb_height)
	if thumb.has_alpha != 0 {
		file.HasAlpha = true
	}
	file.Width, file.Height = int(thumb.width), int(thumb.height)
	file.Orientation = int(thumb.orientation)
	if file.Orientation > 4 {
		file.Width, file.Height = file.Height, file.Width
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
