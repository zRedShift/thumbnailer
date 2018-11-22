package thumbnailer

// #cgo pkg-config: libavformat libavutil libavcodec libswscale
// #cgo CFLAGS: -std=c11
// #cgo LDFLAGS: -lm
// #include "ffmpeg.h"
import "C"
import (
	"context"
	"io"
	"strconv"
	"sync"
	"time"
	"unsafe"

	"github.com/zRedShift/seekstream"
)

//export readCallback
func readCallback(opaque unsafe.Pointer, buf *C.uint8_t, bufSize C.int) C.int {
	ctx, ok := ctxMap.context(opaque)
	if !ok {
		return C.int(avErrUnknown)
	}
	p := (*[1 << 30]byte)(unsafe.Pointer(buf))[:bufSize:bufSize]
	n, err := ctx.file.Read(p)
	if n > 0 || err == nil {
		return C.int(n)
	}
	switch err {
	case io.EOF:
		return C.int(avErrEOF)
	default:
		return C.int(avErrUnknown)
	}
}

////export writeCallback
//func writeCallback(opaque unsafe.Pointer, buf *C.uint8_t, bufSize C.int) C.int {
//	ctx, ok := ctxMap.file(opaque)
//	if !ok {
//		return C.int(avErrUnknown)
//	}
//	p := (*[1 << 30]byte)(unsafe.Pointer(buf))[:bufSize:bufSize]
//	n, err := ctx.Write(p)
//	if err != nil {
//		return C.int(avErrUnknown)
//	}
//	return C.int(n)
//}

//export seekCallback
func seekCallback(opaque unsafe.Pointer, offset C.int64_t, whence C.int) C.int64_t {
	ctx, ok := ctxMap.context(opaque)
	if !ok {
		return C.int64_t(avErrUnknown)
	}
	if !ctx.file.SeekEnd && whence >= C.SEEK_END {
		f, ok := ctx.file.Reader.(*seekstream.File)
		if !ok || !f.IsDone() {
			return C.int64_t(avErrUnknown)

		}
		ctx.file.SeekEnd = true
		ctx.file.Size = f.Size()
	}
	if whence == C.AVSEEK_SIZE && ctx.file.Size > 0 {
		return C.int64_t(ctx.file.Size)
	}
	n, err := ctx.file.Seek(int64(offset), int(whence))
	if err != nil {
		return C.int64_t(avErrUnknown)
	}
	return C.int64_t(n)
}

//export interruptCallback
func interruptCallback(opaque unsafe.Pointer) C.int {
	if ctx, ok := ctxMap.context(opaque); ok {
		select {
		case <-ctx.context.Done():
			return 1
		default:
			return 0
		}
	}
	return 0
}

// AVLogLevel defines the ffmpeg threshold for dumping information to stderr.
type AVLogLevel int

// Possible values for AVLogLevel.
const (
	AVLogQuiet AVLogLevel = (iota - 1) * 8
	AVLogPanic
	AVLogFatal
	AVLogError
	AVLogWarning
	AVLogInfo
	AVLogVerbose
	AVLogDebug
	AVLogTrace
)

func logLevel() AVLogLevel {
	return AVLogLevel(C.av_log_get_level())
}

// SetFFmpegLogLevel allows you to change the log level from the default (AVLogInfo).
func SetFFmpegLogLevel(logLevel AVLogLevel) {
	C.av_log_set_level(C.int(logLevel))
}

const (
	readCallbackFlag = 1 << iota
	//writeCallbackflag
	seekCallbackFlag
	interruptCallbackFlag
)

const (
	hasVideo = 1 << iota
	hasAudio
)

type avContext struct {
	context          context.Context
	file             *File
	formatContext    *C.AVFormatContext
	stream           *C.AVStream
	codecContext     *C.AVCodecContext
	thumbContext     *C.ThumbContext
	frame            *C.AVFrame
	durationInFormat bool
}

type avError int

const (
	avErrNoMem           = avError(-C.ENOMEM)
	avErrEOF             = avError(C.AVERROR_EOF)
	avErrUnknown         = avError(C.AVERROR_UNKNOWN)
	avErrDecoderNotFound = avError(C.AVERROR_DECODER_NOT_FOUND)
	avErrInvalidData     = avError(C.AVERROR_INVALIDDATA)
	errTooBig            = avError(C.ERR_TOO_BIG)
)

func (e avError) errorString() string {
	if e == avErrNoMem {
		return "cannot allocate memory"
	}
	if e == errTooBig {
		return "video or cover art size exceeds maximum allowed dimensions"
	}
	errString := (*C.char)(C.av_malloc(C.AV_ERROR_MAX_STRING_SIZE))
	if errString == nil {
		return "cannot allocate memory for error string, error code: " + strconv.Itoa(int(e))
	}
	defer C.av_free(unsafe.Pointer(errString))
	C.av_make_error_string(errString, C.AV_ERROR_MAX_STRING_SIZE, C.int(e))
	return C.GoString(errString)
}

func (e avError) Error() string { return "ffmpeg: " + e.errorString() }

type contextMap struct {
	sync.RWMutex
	m map[*C.AVFormatContext]*avContext
}

func (m *contextMap) context(opaque unsafe.Pointer) (*avContext, bool) {
	m.RLock()
	ctx, ok := m.m[(*C.AVFormatContext)(opaque)]
	m.RUnlock()
	return ctx, ok
}

func (m *contextMap) set(ctx *avContext) {
	m.Lock()
	m.m[ctx.formatContext] = ctx
	m.Unlock()
}

func (m *contextMap) delete(ctx *avContext) {
	m.Lock()
	delete(m.m, ctx.formatContext)
	m.Unlock()
}

var ctxMap = contextMap{m: make(map[*C.AVFormatContext]*avContext)}

func freeFormatContext(ctx *avContext) {
	C.free_format_context(ctx.formatContext)
	ctxMap.delete(ctx)
}

func createFormatContext(ctx *avContext, callbackFlags C.int) error {
	intErr := C.allocate_format_context(&ctx.formatContext)
	if intErr < 0 {
		return avError(intErr)
	}
	ctxMap.set(ctx)
	intErr = C.create_format_context(ctx.formatContext, callbackFlags)
	if intErr < 0 {
		ctxMap.delete(ctx)
		return avError(intErr)
	}
	metaData(ctx)
	duration(ctx)
	err := findStreams(ctx)
	if err != nil {
		freeFormatContext(ctx)
	}
	return err
}

func metaData(ctx *avContext) {
	var artist, title *C.char
	C.get_metadata(ctx.formatContext, &artist, &title)
	ctx.file.Artist = C.GoString(artist)
	ctx.file.Title = C.GoString(title)
}

func duration(ctx *avContext) {
	if ctx.formatContext.duration > 0 {
		ctx.durationInFormat = true
		ctx.file.Duration = time.Duration(1000 * ctx.formatContext.duration)
	}
}

func fullDuration(ctx *avContext) error {
	defer freeFormatContext(ctx)
	if ctx.durationInFormat {
		return nil
	}
	newDuration := time.Duration(C.find_duration(ctx.formatContext))
	if newDuration < 0 {
		return avError(newDuration)
	}
	if newDuration > ctx.file.Duration {
		ctx.file.Duration = newDuration
	}
	return nil
}

func findStreams(ctx *avContext) error {
	var orientation C.int
	err := C.find_streams(ctx.formatContext, &ctx.stream, &orientation)
	if err < 0 {
		return avError(err)
	}
	ctx.file.HasVideo = err&hasVideo != 0
	ctx.file.HasAudio = err&hasAudio != 0
	if !ctx.file.HasVideo {
		ctx.file.Media = "audio"
	} else {
		ctx.file.Width = int(ctx.stream.codecpar.width)
		ctx.file.Height = int(ctx.stream.codecpar.height)
		ctx.file.Orientation = int(orientation)
	}
	return nil
}

func createDecoder(ctx *avContext) error {
	err := C.create_codec_context(ctx.stream, &ctx.codecContext)
	if err < 0 {
		return avError(err)
	}
	defer C.avcodec_free_context(&ctx.codecContext)
	return createThumbContext(ctx)
}

func incrementDuration(ctx *avContext, frame *C.AVFrame) {
	if !ctx.durationInFormat && frame.pts != C.AV_NOPTS_VALUE {
		ptsToNano := C.int64_t(1000000000 * ctx.stream.time_base.num / ctx.stream.time_base.den)
		newDuration := time.Duration(frame.pts * ptsToNano)
		if newDuration > ctx.file.Duration {
			ctx.file.Duration = newDuration
		}
	}
}

func populateHistogram(ctx *avContext, frames <-chan *C.AVFrame) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		var n C.int
		for frame := range frames {
			C.populate_histogram(ctx.thumbContext, n, frame)
			n++
		}
		ctx.thumbContext.n = n
		done <- struct{}{}
		close(done)
	}()
	return done
}

func createThumbContext(ctx *avContext) error {
	pkt := C.create_packet()
	var frame *C.AVFrame
	err := C.obtain_next_frame(ctx.formatContext, ctx.codecContext, ctx.stream.index, &pkt, &frame)
	if err >= 0 {
		incrementDuration(ctx, frame)
		ctx.thumbContext = C.create_thumb_context(ctx.stream, frame)
		if ctx.thumbContext == nil {
			err = C.int(avErrNoMem)
		}
	}
	if err < 0 {
		if pkt.buf != nil {
			C.av_packet_unref(&pkt)
		}
		if frame != nil {
			C.av_frame_free(&frame)
		}
		return avError(err)
	}
	defer C.free_thumb_context(ctx.thumbContext)
	frames := make(chan *C.AVFrame, ctx.thumbContext.max_frames)
	done := populateHistogram(ctx, frames)
	frames <- frame
	if pkt.buf != nil {
		C.av_packet_unref(&pkt)
	}
	return populateThumbContext(ctx, frames, done)
}

func populateThumbContext(ctx *avContext, frames chan *C.AVFrame, done <-chan struct{}) error {
	pkt := C.create_packet()
	var frame *C.AVFrame
	var err C.int
	for i := C.int(1); i < ctx.thumbContext.max_frames; i++ {
		err = C.obtain_next_frame(ctx.formatContext, ctx.codecContext, ctx.stream.index, &pkt, &frame)
		if err < 0 {
			break
		}
		incrementDuration(ctx, frame)
		frames <- frame
		frame = nil
	}
	close(frames)
	if pkt.buf != nil {
		C.av_packet_unref(&pkt)
	}
	if frame != nil {
		C.av_frame_free(&frame)
	}
	<-done
	if err != 0 && err != C.int(avErrEOF) {
		return avError(err)
	}
	return convertFrameToRGB(ctx)
}

func convertFrameToRGB(ctx *avContext) error {
	outputFrame := C.convert_frame_to_rgb(C.process_frames(ctx.thumbContext), ctx.thumbContext.alpha)
	if outputFrame == nil {
		return avErrNoMem
	}
	ctx.frame = outputFrame
	ctx.file.HasAlpha = ctx.thumbContext.alpha != 0
	return nil
}

func thumbnail(ctx *avContext) <-chan error {
	errCh := make(chan error)
	go func() {
		err := thumbnailFromFFmpeg(ctx.file, ctx.frame.data[0])
		C.av_frame_free(&ctx.frame)
		errCh <- err
		close(errCh)
	}()
	return errCh
}

func ffmpegThumbnail(context context.Context, file *File) error {
	ctx := &avContext{context: context, file: file}
	callbackFlags := C.int(readCallbackFlag | interruptCallbackFlag)
	if file.Seeker != nil {
		callbackFlags |= seekCallbackFlag
	}
	err := createFormatContext(ctx, callbackFlags)
	if err != nil {
		return err
	}
	if !file.HasVideo {
		return fullDuration(ctx)
	}
	if err = createDecoder(ctx); err == errTooBig || err == avErrDecoderNotFound {
		return fullDuration(ctx)
	}
	if err != nil {
		freeFormatContext(ctx)
		return err
	}
	errCh := thumbnail(ctx)
	err = fullDuration(ctx)
	thumbErr := <-errCh
	if thumbErr != nil {
		return thumbErr
	}
	return err
}
