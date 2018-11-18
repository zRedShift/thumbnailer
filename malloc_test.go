package thumbnailer

import (
	"runtime"
	"testing"
	"unsafe"
)

func TestMallocTrim(t *testing.T) {
	free(malloc(1 << 12))
	if !MallocTrim(0) {
		t.Errorf("MallocTrim() want = %v, got = %v", true, false)
	}
}

func TestMallopt(t *testing.T) {
	tests := []struct {
		name  string
		param MalloptParam
		value int
		want  bool
	}{
		{"ArenaMax", ArenaMax, 8 * runtime.GOMAXPROCS(0), true},
		{"ArenaTest", ArenaTest, int(unsafe.Sizeof(0)), true},
		{"CheckAction", CheckAction, 0, true},
		{"MMapMax", MMapMax, 1 << 17, true},
		{"MMapThreshold", MMapThreshold, 1 << 16, true},
		{"MXFast", MXFast, 16 * int(unsafe.Sizeof(0)), true},
		{"Perturb", Perturb, 0, true},
		{"TopPad", TopPad, 1 << 17, true},
		{"TrimThreshold", TrimThreshold, 1 << 17, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Mallopt(test.param, test.value); got != test.want {
				t.Errorf("AVLogLevel want = %v, got = %v", test.want, got)
			}
		})
	}
}
