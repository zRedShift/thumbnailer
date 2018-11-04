package thumbnailer

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCreateThumbnail(t *testing.T) {
	t.Parallel()
	tests := []struct {
		filename        string
		wantErr         error
		wantThumb       bool
		wantDims        Dimensions
		wantDuration    time.Duration
		altDuration     time.Duration
		wantAlpha       bool
		wantMediaType   string
		wantArtist      string
		wantTitle       string
		wantOrientation int
	}{
		{"trollface.png", nil, true, Dimensions{5000, 4068}, 0, 0, true, "image/png", "", "", 1},
		{"EVERYBODY BETRAY ME.mkv", nil, true, Dimensions{640, 480}, 7407000000, 0, false, "video/x-matroska", "", "", 1},
		{"alpha-webm.webm", nil, true, Dimensions{720, 576}, 12040000000, 0, true, "video/webm", "", "", 1},
		{"schizo.flv", nil, true, Dimensions{480, 360}, 2560000000, 0, false, "video/x-flv", "", "", 1},
		{"2_webp_ll.webp", nil, true, Dimensions{386, 395}, 0, 0, true, "image/webp", "", "", 1},
		{"small.ogv", nil, true, Dimensions{560, 320}, 5546667000, 5538666666, false, "video/ogg", "", "", 1},
		{"spszut pszek.mp3", nil, true, Dimensions{350, 350}, 1097143000, 1071020408, false, "audio/mpeg", "lors lara", "spszut pszek", 1},
		{"Portrait_3.jpg", nil, true, Dimensions{1200, 1800}, 0, 0, false, "image/jpeg", "", "", 3},
		{"Portrait_6.jpg", nil, true, Dimensions{1200, 1800}, 0, 0, false, "image/jpeg", "", "", 6},
		{"Landscape_8.jpg", nil, true, Dimensions{1800, 1200}, 0, 0, false, "image/jpeg", "", "", 8},
		{"RAID_5.svg", nil, true, Dimensions{675, 500}, 0, 0, true, "image/svg+xml", "", "", 1},
		{"Olympic_rings_with_transparent_rims.svg", nil, true, Dimensions{1020, 495}, 0, 0, true, "image/svg+xml", "", "", 1},
		{"dürümpf.mp3", nil, false, Dimensions{0, 0}, 4675833000, 4649795918, false, "audio/mpeg", "", "", 0},
		{"perpendicular24.pdf", nil, true, Dimensions{553, 417}, 0, 0, false, "application/pdf", "", "", 1},
		{"gif_bg.gif", nil, true, Dimensions{100, 70}, 0, 0, false, "image/gif", "", "", 1},
		{"i can't believe this story your telling me.mp4", nil, true, Dimensions{492, 360}, 3925000000, 0, false, "video/mp4", "", "", 1},
		{"ancap.svgz", nil, true, Dimensions{900, 600}, 0, 0, false, "image/svg+xml-compressed", "", "", 1},
		{"sample.tif", nil, true, Dimensions{1600, 2100}, 0, 0, false, "image/tiff", "", "", 1},
		{"mqdefault_6s.webp", errors.New("vips: webp2vips: unable to read pixels"), false, Dimensions{0, 0}, 0, 0, false, "image/webp", "", "", 0},
		{"schizo_0.mp4", nil, true, Dimensions{480, 360}, 2544000000, 0, false, "video/mp4", "", "", 1},
		{"schizo_90.mp4", nil, true, Dimensions{480, 360}, 2544000000, 0, false, "video/mp4", "", "", 8},
		{"schizo_180.mp4", nil, true, Dimensions{480, 360}, 2544000000, 0, false, "video/mp4", "", "", 3},
		{"schizo_270.mp4", nil, true, Dimensions{480, 360}, 2544000000, 0, false, "video/mp4", "", "", 6},
	}
	testDir := "fixtures"
	var wg sync.WaitGroup
	InitVIPS()
	vipsCheckLeaks()
	VIPSCacheSetMaxFiles(10)
	VIPSCacheSetMax(10)
	VIPSCacheSetMaxMem(100 * 1 << 20)
	for _, test := range tests {
		test := test
		files := make([]*File, 5)
		var err error
		path := filepath.Join(testDir, test.filename)
		files[0], err = FileFromPath(path)
		if err != nil {
			t.Fatalf("FileFromPath() error = %v", err)
		}
		files[0].ToPath(fmt.Sprintf("tmp/tn_%s.jpg", strings.TrimSuffix(test.filename, filepath.Ext(test.filename))), 256, 75)
		files[1], err = FileFromPath(path)
		if err != nil {
			t.Fatalf("FileFromPath() error = %v", err)
		}
		files[1].ToWriter(ioutil.Discard, 256, 75)
		f, err := os.Open(path)
		if err != nil {
			t.Errorf("os.Open() error = %v", err)
		} else {
			files[2], err = FileFromReader(f, test.filename)
			if err != nil {
				t.Errorf("FileFromReader() error = %v", err)
				f.Close()
			} else {
				files[2].ToWriter(ioutil.Discard, 256, 75)
			}
		}
		f, err = os.Open(path)
		if err != nil {
			t.Errorf("os.Open() error = %v", err)
		} else {
			files[3], err = FileFromReadSeeker(f, true, test.filename)
			if err != nil {
				t.Errorf("FileFromReadSeeker() error = %v", err)
				f.Close()
			} else {
				files[3].ToWriter(ioutil.Discard, 256, 75)
			}
		}
		f, err = os.Open(path)
		if err != nil {
			t.Errorf("os.Open() error = %v", err)
		} else {
			files[4], err = FileFromReadSeeker(f, false, test.filename)
			if err != nil {
				t.Errorf("FileFromReadSeeker() error = %v", err)
				f.Close()
			} else {
				files[4].ToWriter(ioutil.Discard, 256, 75)
			}
		}
		for _, f := range files {
			if f == nil {
				continue
			}
			f := f
			err := err
			closer, ok := f.Reader.(io.Closer)
			wg.Add(1)
			t.Run(test.filename, func(t *testing.T) {
				defer wg.Done()
				t.Parallel()
				if err = CreateThumbnail(f); !reflect.DeepEqual(err, test.wantErr) && (f.Seeker != nil || err != avErrInvalidData) {
					t.Errorf("CreateThumbnail() error = %v, want = %v", err, test.wantErr)
				} else if err != nil {
					t.Logf("CreateThumbnail() error = %v", err)
				}
				if f.ThumbCreated != test.wantThumb && err == nil {
					t.Errorf("ThumbCreated want = %v, got = %v", test.wantThumb, f.ThumbCreated)
				}
				if f.Dimensions != test.wantDims && err == nil {
					t.Errorf("Dimensions want = %v, got = %v", test.wantDims, f.Dimensions)
				}
				if f.Duration != test.wantDuration && f.Duration != test.altDuration {
					t.Errorf("Duration want = %v or %v, got = %v", test.wantDuration, test.altDuration, f.Duration)
				}
				if f.HasAlpha != test.wantAlpha {
					t.Errorf("Alpha want = %v, got = %v", test.wantAlpha, f.HasAlpha)
				}
				if f.MediaType.MediaType() != test.wantMediaType {
					t.Errorf("MediaType want = %v, got = %v", test.wantMediaType, f.MediaType.MediaType())
				}
				if f.Artist != test.wantArtist {
					t.Errorf("Artist want = %v, got = %v", test.wantArtist, f.Artist)
				}
				if f.Title != test.wantTitle {
					t.Errorf("Title want = %v, got = %v", test.wantTitle, f.Title)
				}
				if f.Orientation != test.wantOrientation {
					t.Errorf("Orientation want = %v, got = %v", test.wantOrientation, f.Orientation)
				}
				if f.Thumbnail.Path != "" && f.ThumbCreated && f.HasAlpha {
					err = os.Rename(f.Thumbnail.Path, strings.TrimSuffix(f.Thumbnail.Path, ".jpg")+".png")
					if err != nil {
						t.Errorf("os.Rename() error = %v", err)
					}
				}
				if ok {
					closer.Close()
				}
			})
		}
	}
	t.Run("waiter", func(t *testing.T) {
		t.Helper()
		t.Parallel()
		wg.Wait()
		vipsPrintAll()
		DropAllVIPSCache()
		ShutdownVIPS()
		InitVIPS()
	})
}
