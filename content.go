package thumbnailer

import (
	"github.com/zRedShift/mimemagic"
	"io"
	"time"
)

type Content struct {
	io.Reader
	Thumbnail
	mimemagic.MediaType
	Dimensions
	Size               int64
	Duration           time.Duration
	Title, Artist      string
	HasVideo, HasAudio bool
	io.Seeker
	CanSeekToEnd bool
}

type Thumbnail struct {
	io.Writer
	TargetDimensions Dimensions
	AlphaChannel     bool
}

type Dimensions struct {
	Width, Height int
}
