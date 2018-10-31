package thumbnailer

import (
	"github.com/zRedShift/mimemagic"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func Test_ffmpegThumbnail(t *testing.T) {
	t.Parallel()
	testDir := "fixtures"
	files, err := ioutil.ReadDir(testDir)
	if err != nil {
		t.Fatalf("couldn't read dir: %v\n", err)
	}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		filename := file.Name()
		t.Run(filename, func(t *testing.T) {
			t.Parallel()
			f, err := os.Open(filepath.Join(testDir, filename))
			if err != nil {
				t.Fatalf("couldn't open file: %v\n", err)
			}
			defer f.Close()
			content := &Content{Reader: f, Seeker: f}
			content.MediaType, err = mimemagic.MatchReader(content, filename, -1, mimemagic.Magic)
			if err != nil {
				t.Fatalf("mimemagic error: %v\n", err)
			}
			if content.Media != "audio" && content.Media != "video" {
				return
			}
			content.Writer = ioutil.Discard
			_, err = content.Seek(0, 0)
			if err != nil {
				t.Fatalf("couldn't seek to start: %v\n", err)
			}
			thumb, err := ffmpegThumbnail(nil, content)
			if thumb {
				t.Logf("created thumbnail for %s\n", filename)
			} else {
				t.Logf("couldn't create thumbnail for %s\n", filename)
			}
			if err != nil {
				t.Fatalf("thumbnail creation error: %v\n", err)
			}
		})
	}
}
