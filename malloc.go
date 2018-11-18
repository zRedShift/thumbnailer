package thumbnailer

// #cgo CFLAGS: -std=c11
// #include "malloc.h"
import "C"
import "unsafe"

// MalloptParam holds possible mallopt parameters.
type MalloptParam int

// All possible values of MalloptParam.
const (
	ArenaMax      MalloptParam = C.M_ARENA_MAX
	ArenaTest     MalloptParam = C.M_ARENA_TEST
	CheckAction   MalloptParam = C.M_CHECK_ACTION
	MMapMax       MalloptParam = C.M_MMAP_MAX
	MMapThreshold MalloptParam = C.M_MMAP_THRESHOLD
	MXFast        MalloptParam = C.M_MXFAST
	Perturb       MalloptParam = C.M_PERTURB
	TopPad        MalloptParam = C.M_TOP_PAD
	TrimThreshold MalloptParam = C.M_TRIM_THRESHOLD
)

// MallocTrim calls glibc's malloc_trim. Go refuses to return freed C memory to the OS otherwise for some reason.
func MallocTrim(pad int) bool {
	return C.malloc_trim(C.size_t(pad)) > 0
}

// Mallopt calls glibc's mallopt. Reducing M_TOP_PAD or M_TRIM_THRESHOLD, increasing M_MMAP_THRESHOLD, and setting
// M_ARENA_MAX to GOMAXPROCS (it defaults to 8 * #threads), has shown good results with regards to freed memory "leaks".
func Mallopt(param MalloptParam, value int) bool {
	return C.mallopt(C.int(param), C.int(value)) > 0
}

func malloc(size int) unsafe.Pointer {
	return C.malloc(C.size_t(size))
}

func free(ptr unsafe.Pointer) {
	C.free(ptr)
}
