package thumbnailer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateThumbnail(t *testing.T) {
	t.Parallel()
	tests := []struct {
		filename  string
		wantThumb bool
	}{
		{"trollface.png", true},
		{"EVERYBODY BETRAY ME.mkv", true},
		{"alpha-webm.webm", true},
		{"schizo.flv", true},
		{"2_webp_ll.webp", true},
		{"small.ogv", true},
		{"spszut pszek.mp3", true},
		{"Portrait_3.jpg", true},
		{"RAID_5.svg", true},
		{"Olympic_rings_with_transparent_rims.svg", true},
		{"dürümpf.mp3", false},
		{"perpendicular24.pdf", true},
		{"gif_bg.gif", true},
		{"i can't believe this story your telling me.mp4", true},
	}
	testDir := "fixtures"
	for _, test := range tests {
		test := test
		t.Run(test.filename, func(t *testing.T) {
			t.Parallel()
			f, err := FromPath(filepath.Join(testDir, test.filename))
			if err != nil {
				t.Fatalf("FromPath() error = %v", err)
			}
			thumbPath := fmt.Sprintf("tmp/tn_%s.jpg", strings.TrimSuffix(test.filename, filepath.Ext(test.filename)))
			f.ToPath(thumbPath, 256)
			if err := CreateThumbnail(f); err != nil {
				t.Errorf("CreateThumbnail() error = %v", err)
			}
			if f.ThumbCreated != test.wantThumb {
				t.Errorf("ThumbCreated want = %v, got = %v", test.wantThumb, f.ThumbCreated)
			}
			if f.ThumbCreated && f.HasAlpha {
				os.Rename(thumbPath, strings.Replace(thumbPath, ".jpg", ".png", -1))
			}
		})
	}
}
