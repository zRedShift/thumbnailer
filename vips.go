package thumbnailer

// #cgo pkg-config: vips
// #cgo CFLAGS: -std=c11
// #include "vips.h"
import "C"
import (
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
	once sync.Once
)

// VIPSOptions stores various options for vips.
type VIPSOptions struct {
	Leak                                 bool
	CacheMax, CacheMaxMem, CacheMaxFiles *int
}

// VIPSMemoryProfile contains the vips memory profile.
type VIPSMemoryProfile struct {
	Memory, MemoryHighWater int64
	Allocations, Files      int
}

// InitVIPS initializes vips explicitly.
func InitVIPS() {
	initVIPS()
}

// SetVIPSOptions initializes vips with options.
func SetVIPSOptions(options VIPSOptions) {
	initVIPS()
	if options.Leak {
		C.vips_leak_set(1)
	}
	if options.CacheMax != nil {
		C.vips_cache_set_max(C.int(*options.CacheMax))
	}
	if options.CacheMaxMem != nil {
		C.vips_cache_set_max_mem(C.ulong(*options.CacheMaxMem))
	}
	if options.CacheMaxFiles != nil {
		C.vips_cache_set_max_files(C.int(*options.CacheMaxFiles))
	}
}

func initVIPS() {
	once.Do(func() {
		name := C.CString(os.Args[0])
		defer free(unsafe.Pointer(name))
		if C.vips_init(name) != 0 {
			vErr := errBuf.lastError()
			panic("couldn't start vips: " + vErr.Error())
		}
		C.vips_concurrency_set(1)
	})
}

// ShutdownVIPS shuts vips down. It can't be used again after this.
func ShutdownVIPS() {
	C.vips_shutdown()
}

// VIPSMemory returns vips's internal tracked memory profile.
func VIPSMemory() VIPSMemoryProfile {
	initVIPS()
	return VIPSMemoryProfile{
		Memory:          int64(C.vips_tracked_get_mem()),
		MemoryHighWater: int64(C.vips_tracked_get_mem_highwater()),
		Allocations:     int(C.vips_tracked_get_allocs()),
		Files:           int(C.vips_tracked_get_files()),
	}
}

// PrintAllVIPSObjects prints all objects in vips's operation cache.
func PrintAllVIPSObjects() {
	initVIPS()
	C.vips_object_print_all()
}

// DropAllVIPSCache drops the whole operation cache. Called automatically on ShutdownVips(). Some VIPS versions can't
// run after this operation is performed.
func DropAllVIPSCache() {
	initVIPS()
	C.vips_cache_drop_all()
}

type errorBuf struct {
	sync.Mutex
	errSlice []string
	lastErr  vipsError
}

type vipsError struct {
	domain, error string
}

func (v vipsError) Error() string { return "vips: " + v.domain + ": " + v.error }

func (b *errorBuf) lastError() vipsError {
	b.Lock()
	defer b.Unlock()
	errSlice := strings.Split(C.GoString(C.vips_error_buffer()), "\n")
	if len(errSlice) > 1 {
		C.vips_error_clear()
	}
	b.errSlice = append(errSlice[:len(errSlice)-1], b.errSlice...)
	if len(b.errSlice) != 0 {
		err := strings.SplitN(b.errSlice[0], ": ", 2)
		b.errSlice = b.errSlice[1:]
		if len(err) == 2 {
			b.lastErr.domain = err[0]
			err = err[1:]
		}
		b.lastErr.error = err[0]
	}
	return b.lastErr
}

var errBuf = &errorBuf{errSlice: make([]string, 0, 10)}

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
	defer free(unsafe.Pointer(thumb.input_path))
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
		defer free(unsafe.Pointer(thumb.output_path))
	}
	initVIPS()
	if C.thumbnail(thumb) != 0 {
		return errBuf.lastError()
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
