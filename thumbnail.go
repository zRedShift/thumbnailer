package thumbnailer

import (
	"bytes"
	"context"
	"github.com/zRedShift/mimemagic"
	"io"
	"os"
	"path/filepath"
	"time"
)

const (
	defaultQuality = 75
	probeSize      = 1 << 12
)

type File struct {
	io.Reader
	io.Seeker
	SeekEnd bool
	Path    string
	Thumbnail
	mimemagic.MediaType
	Dimensions
	Size               int64
	Duration           time.Duration
	Title, Artist      string
	HasVideo, HasAudio bool
}

type Thumbnail struct {
	io.Writer
	Path string
	Dimensions
	Quality, TargetDimensions int
	HasAlpha, ThumbCreated    bool
}

type Dimensions struct {
	Width, Height int
}

func FromReader(r io.Reader, filename ...string) (*File, error) {
	data := make([]byte, probeSize)
	if n, err := io.ReadAtLeast(r, data, probeSize); err == io.ErrUnexpectedEOF || err == io.EOF {
		data = data[:n]
	} else if err != nil {
		return nil, err
	}
	fn := ""
	if len(filename) > 0 {
		fn = filename[0]
	}
	return &File{
		Reader:    io.MultiReader(bytes.NewReader(data), r),
		MediaType: mimemagic.Match(data, fn, mimemagic.Magic),
	}, nil
}

func FromReadSeeker(r io.ReadSeeker, seekEnd bool, filename ...string) (*File, error) {
	fn := ""
	if len(filename) > 0 {
		fn = filename[0]
	}
	mediatype, err := mimemagic.MatchReader(r, fn, probeSize)
	if err != nil {
		return nil, err
	}
	if _, err = r.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	return &File{
		Reader:    r,
		Seeker:    r,
		SeekEnd:   seekEnd,
		MediaType: mediatype,
	}, nil
}

func FromPath(path string) (*File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fs, err := f.Stat()
	if err != nil {
		return nil, err
	}
	mediatype, err := mimemagic.MatchReader(f, filepath.Base(f.Name()), probeSize, mimemagic.Magic)
	if err != nil {
		return nil, err
	}
	return &File{
		Path:      path,
		SeekEnd:   true,
		Size:      fs.Size(),
		MediaType: mediatype,
	}, nil
}

func (f *File) to(size int, quality ...int) *File {
	f.TargetDimensions = size
	f.Quality = defaultQuality
	if len(quality) > 0 {
		f.Quality = quality[0]
	}
	return f
}

func (f *File) ToWriter(w io.Writer, size int, quality ...int) *File {
	f.Writer = w
	return f.to(size, quality...)
}

func (f *File) ToPath(path string, size int, quality ...int) *File {
	f.Thumbnail.Path = path
	return f.to(size, quality...)
}

func CreateThumbnailWithContext(ctx context.Context, file *File) error {
	if file.Media == "video" || file.Media == "audio" {
		if file.Path != "" {
			f, err := os.Open(file.Path)
			if err != nil {
				return err
			}
			defer f.Close()
			file.Reader, file.Seeker = f, f
		}
		return ffmpegThumbnail(ctx, file)
	}
	if file.Media == "image" || file.Subtype == "pdf" {
		return thumbnailFromFile(file)
	}
	return nil
}

func CreateThumbnail(file *File) error {
	return CreateThumbnailWithContext(context.Background(), file)
}
