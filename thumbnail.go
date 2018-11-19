// Package thumbnailer provides a lightning fast and memory usage efficient thumbnailer via libvips and ffmpeg
// C bindings, with (external) MIME sniffing, and streaming I/O support. The formats available depend on the way libvips
// and ffmpeg are compiled.
package thumbnailer

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zRedShift/mimemagic"
)

const (
	defaultQuality = 75
	probeSize      = 1 << 12
)

// File stores the io.Reader and the io.Seeker or the path to the input file (preference to the path), its dimensions
// after analysis (if applicable), its Media Type, resultant thumbnail, size (completely optional for FFmpeg seeking),
// duration and the title and artist metadata if the file is a video or audio and those exist. Setting SeekEnd to false
// disables FFmpeg from seeking the end, which enables partial file reading in "semi-streaming" files (incomplete files
// that block until more data is available but have seeking capabilities) without blocking until the file is complete.
// HasVideo and HasAudio indicates that the file has video and/or audio streams, but having a video stream does not
// guarantee a thumbnail. Orientation corresponds to the EXIF orientation of the input file.
type File struct {
	io.Reader
	io.Seeker
	Thumbnail
	mimemagic.MediaType
	Dimensions
	Orientation                 int
	Size                        int64
	Duration                    time.Duration
	Title, Artist, Path         string
	HasVideo, HasAudio, SeekEnd bool
}

// Thumbnail stores the io.Writer to which to write the thumbnail, or creates it at the given path (preference to the
// path), its resultant dimensions, target Quality (both for JPEG and lossy PNG output), and the size of the bounding
// box to which the thumbnail is shrunk (TargetDimensions). HasAlpha indicates the image is a transparent PNG, JPEG if
// false. ThumbCreated indicates the thumbnail was created successfully.
type Thumbnail struct {
	io.Writer
	Dimensions
	Quality, TargetDimensions int
	Path                      string
	HasAlpha, ThumbCreated    bool
}

// Dimensions stores the dimensions of the file and its thumbnail (if applicable).
type Dimensions struct {
	Width, Height int
}

// ThumbError stores the errors returned by the C Libraries used in this package.
type ThumbError struct {
	Library, Domain, Err string
	Code                 int
}

// Common ThumbErrors.
var (
	ErrInvalidData            = avErrorToThumbError(avErrInvalidData)
	ErrFileFormatNotSupported = vipsErrorToThumbError(vipsError{
		domain: "VipsForeignLoad",
		error:  "the current build does not support this file format",
	})
	ErrAnimatedWEBPNotSupported = vipsErrorToThumbError(vipsError{
		domain: "webp2vips",
		error:  "the current build does not support animated webp files",
	})
)

func (t *ThumbError) Error() string {
	if t.Domain == "" {
		return t.Library + ": " + t.Err
	}
	return t.Library + ": " + t.Domain + ": " + t.Err
}

func avErrorToThumbError(e avError) *ThumbError {
	return &ThumbError{
		Library: "ffmpeg",
		Err:     e.errorString(),
		Code:    int(e),
	}
}

func vipsErrorToThumbError(e vipsError) *ThumbError {
	return &ThumbError{
		Library: "vips",
		Domain:  e.domain,
		Err:     e.error,
	}
}

// FileFromReader takes an io.Reader and an optional filename (for better MIME sniffing), and returns a File ready for
// supplying a thumbnail output via ToFile or ToPath. Without seeking, thumbnailing videos with non-sequential codecs
// (H.264 in some cases), will fail more than the alternatives.
func FileFromReader(r io.Reader, filename ...string) (*File, error) {
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

// FileFromReadSeeker takes an io.ReadSeeker, a boolean seekEnd, and an optional filename (for better MIME sniffing),
// and returns a File ready for supplying a thumbnail output via ToFile or ToPath. Setting seekEnd to false disables
// FFmpeg from seeking the end, which enables partial file reading in "semi-streaming" files (incomplete files that
// block until more data is available but have seeking capabilities) without blocking until the file is complete.
// Setting it to true treats the ReadSeeker like a regular file.
func FileFromReadSeeker(r io.ReadSeeker, seekEnd bool, filename ...string) (*File, error) {
	fn := ""
	if len(filename) > 0 {
		fn = filename[0]
	}
	mediaType, err := mimemagic.MatchReader(r, fn, probeSize, mimemagic.Magic)
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
		MediaType: mediaType,
	}, nil
}

// FileFromPath takes a filepath and returns a File ready for supplying a thumbnail output via ToFile or ToPath.
func FileFromPath(path string) (file *File, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		cErr := f.Close()
		if err == nil {
			err = cErr
		}
	}()
	fs, err := f.Stat()
	if err != nil {
		return nil, err
	}
	mediaType, err := mimemagic.MatchReader(f, filepath.Base(f.Name()), probeSize, mimemagic.Magic)
	if err != nil {
		return nil, err
	}
	return &File{
		Path:      path,
		SeekEnd:   true,
		Size:      fs.Size(),
		MediaType: mediaType,
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

// ToWriter directs the thumbnailer to write the resultant thumbnail to the supplied io.Write at the target bounding box
// size and quality (quality corresponds to libjpeg quality for JPEG thumbnails and to libimagequant quality for PNG
// thumbnails).
func (f *File) ToWriter(w io.Writer, size int, quality ...int) *File {
	f.Writer = w
	return f.to(size, quality...)
}

// ToPath directs the thumbnailer to write the resultant thumbnail to the supplied path at the target bounding box size
// and quality (quality corresponds to libjpeg quality for JPEG thumbnails and to libimagequant quality for PNG
// thumbnails).
func (f *File) ToPath(path string, size int, quality ...int) *File {
	f.Thumbnail.Path = path
	return f.to(size, quality...)
}

// CreateThumbnailWithContext creates a thumbnail from the supplied file (should go through FileFromReader,
// FromReadSeeker or FileFromPath and then ToWriter or ToPath, or equivalent for defined behaviour) and a context for
// interruption. Currently it's only checked if Done() in FFmpeg before blocking operations via an interrupt callback.
func CreateThumbnailWithContext(ctx context.Context, file *File) (err error) {
	defer func() {
		switch tErr := err.(type) {
		case avError:
			if tErr == avErrInvalidData {
				err = ErrInvalidData
			} else {
				err = avErrorToThumbError(tErr)
			}
		case vipsError:
			switch {
			case tErr.domain == "VipsForeignLoad" && strings.HasSuffix(tErr.error, "not a known file format"):
				err = ErrFileFormatNotSupported
			case tErr.domain == "webp2vips" && tErr.error == "unable to read pixels":
				err = ErrAnimatedWEBPNotSupported
			default:
				err = vipsErrorToThumbError(tErr)
			}
		}
	}()
	if file.Media == "video" || file.Media == "audio" {
		if file.Path != "" {
			f, err := os.Open(file.Path)
			if err != nil {
				return err
			}
			defer func() {
				cErr := f.Close()
				if err == nil {
					err = cErr
				}
			}()
			file.Reader, file.Seeker = f, f
		}
		return ffmpegThumbnail(ctx, file)
	}
	return thumbnailFromFile(file)
}

// CreateThumbnail calls CreateThumbnailWithContext with a background context.
func CreateThumbnail(file *File) error {
	return CreateThumbnailWithContext(context.Background(), file)
}
