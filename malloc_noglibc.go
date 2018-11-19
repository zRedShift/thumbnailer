// +build noglibc

package thumbnailer

// #cgo CFLAGS: -std=c11
// #include "stdlib.h"
import "C"
import "unsafe"

// MalloptParam holds possible mallopt parameters.
type MalloptParam int

// All possible values of MalloptParam.
const (
	ArenaMax      MalloptParam = 0
	ArenaTest     MalloptParam = 0
	CheckAction   MalloptParam = 0
	MMapMax       MalloptParam = 0
	MMapThreshold MalloptParam = 0
	MXFast        MalloptParam = 0
	Perturb       MalloptParam = 0
	TopPad        MalloptParam = 0
	TrimThreshold MalloptParam = 0
)

// MallocTrim calls glibc's malloc_trim. Go refuses to return freed C memory to the OS otherwise for some reason.
func MallocTrim(pad int) bool {
	return true
}

// Mallopt calls glibc's mallopt. Reducing M_TOP_PAD or M_TRIM_THRESHOLD, increasing M_MMAP_THRESHOLD, and setting
// M_ARENA_MAX to GOMAXPROCS (it defaults to 8 * #threads), has shown good results with regards to freed memory "leaks".
func Mallopt(param MalloptParam, value int) bool {
	return true
}

func malloc(size int) unsafe.Pointer {
	return C.malloc(C.size_t(size))
}

func free(ptr unsafe.Pointer) {
	C.free(ptr)
}
