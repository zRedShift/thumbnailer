package thumbnailer

//#cgo pkg-config: libavformat libavutil libavcodec libswscale
// #cgo CFLAGS: -std=c11 -O3 -DNDEBUG
// #cgo LDFLAGS: -lm
// #include "ffmpeg.h"
import "C"
import (
	"context"
	"fmt"
	"io"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

//export readCallback
func readCallback(opaque unsafe.Pointer, buf *C.uint8_t, bufSize C.int) C.int {
	ctx, ok := ctxMap.file(opaque)
	if !ok {
		return C.int(avErrUnknown)
	}
	p := (*[1 << 30]byte)(unsafe.Pointer(buf))[:bufSize:bufSize]
	n, err := ctx.Read(p)
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

//export writeCallback
func writeCallback(opaque unsafe.Pointer, buf *C.uint8_t, bufSize C.int) C.int {
	ctx, ok := ctxMap.file(opaque)
	if !ok {
		return C.int(avErrUnknown)
	}
	p := (*[1 << 30]byte)(unsafe.Pointer(buf))[:bufSize:bufSize]
	n, err := ctx.Write(p)
	if err != nil {
		return C.int(avErrUnknown)
	}
	return C.int(n)
}

//export seekCallback
func seekCallback(opaque unsafe.Pointer, offset C.int64_t, whence C.int) C.int64_t {
	ctx, ok := ctxMap.file(opaque)
	if !ok {
		return C.int64_t(avErrUnknown)
	}
	if !ctx.SeekEnd && whence >= C.SEEK_END {
		return C.int64_t(avErrUnknown)
	}
	if whence == C.AVSEEK_SIZE && ctx.Size > 0 {
		return C.int64_t(ctx.Size)
	}
	n, err := ctx.Seek(int64(offset), int(whence))
	if err != nil {
		return C.int64_t(avErrUnknown)
	}
	return C.int64_t(n)
}

//export interruptCallback
func interruptCallback(opaque unsafe.Pointer) C.int {
	if ctx, ok := ctxMap.context(opaque); ok {
		select {
		case <-ctx.Done():
			return 1
		default:
			return 0
		}
	}
	return 0
}

// AVLogLevel defines the ffmpeg threshold for dumping information to stderr.
type AVLogLevel int

// Possible values for AVLogLevel
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
	writeCallbackflag
	seekCallbackFlag
	interruptCallbackFlag
)

const (
	hasVideo = 1 << iota
	hasAudio
)

type contextKey int

const (
	fileKey contextKey = iota
	fmtCtxKey
	streamKey
	decCtxKey
	thumbCtxKey
	frameKey
	durationKey
)

type avError C.int

var (
	avErrNoMem           = -avError(syscall.ENOMEM)
	avErrEOF             = avError(C.AVERROR_EOF)
	avErrUnknown         = avError(C.AVERROR_UNKNOWN)
	avErrDecoderNotFound = avError(C.AVERROR_DECODER_NOT_FOUND)
	avErrInvalidData     = avError(C.AVERROR_INVALIDDATA)
	errTooBig            = avError(C.ERR_TOO_BIG)
)

func (e avError) Error() string {
	if e == avErrNoMem {
		return "ffmpeg: Cannot allocate memory"
	}
	if e == errTooBig {
		return "ffmpeg: video or cover art size exceeds maximum allowed dimensions"
	}
	errString := (*C.char)(C.av_malloc(C.AV_ERROR_MAX_STRING_SIZE))
	if errString == nil {
		return fmt.Sprintf("ffmpeg: cannot allocate memory for error string, error code: %d", int(e))
	}
	C.av_make_error_string(errString, C.AV_ERROR_MAX_STRING_SIZE, C.int(e))
	goString := "ffmpeg: " + C.GoString(errString)
	C.av_free(unsafe.Pointer(errString))
	return goString
}

type contextMap struct {
	sync.RWMutex
	m map[*C.AVFormatContext]context.Context
}

func (m *contextMap) file(opaque unsafe.Pointer) (file *File, ok bool) {
	m.RLock()
	ctx, ok := m.m[(*C.AVFormatContext)(opaque)]
	m.RUnlock()
	if ok {
		file, ok = ctx.Value(fileKey).(*File)
	}
	return
}

func (m *contextMap) context(opaque unsafe.Pointer) (ctx context.Context, ok bool) {
	m.RLock()
	ctx, ok = m.m[(*C.AVFormatContext)(opaque)]
	m.RUnlock()
	return
}

func (m *contextMap) set(ctx context.Context) {
	fmtCtx := ctx.Value(fmtCtxKey).(*C.AVFormatContext)
	m.Lock()
	m.m[fmtCtx] = ctx
	m.Unlock()
	return
}

func (m *contextMap) delete(ctx context.Context) (*C.AVFormatContext, bool) {
	if fmtCtx, ok := ctx.Value(fmtCtxKey).(*C.AVFormatContext); ok {
		m.Lock()
		delete(m.m, fmtCtx)
		m.Unlock()
		return fmtCtx, true
	}
	return nil, false
}

var ctxMap = contextMap{m: make(map[*C.AVFormatContext]context.Context)}

func freeFormatContext(ctx context.Context) {
	if fmtCtx, ok := ctxMap.delete(ctx); ok && fmtCtx != nil {
		C.free_format_context(fmtCtx)
	}
}

func createFormatContext(ctx context.Context, callbackFlags C.int) (context.Context, error) {
	var fmtCtx *C.AVFormatContext
	intErr := C.allocate_format_context(&fmtCtx)
	if intErr < 0 {
		return nil, avError(intErr)
	}
	ctx = context.WithValue(ctx, fmtCtxKey, fmtCtx)
	ctxMap.set(ctx)
	intErr = C.create_format_context(fmtCtx, callbackFlags)
	if intErr < 0 {
		ctxMap.delete(ctx)
		return nil, avError(intErr)
	}
	metaData(ctx)
	ctx = duration(ctx)
	ctx, err := findStreams(ctx)
	if err != nil {
		freeFormatContext(ctx)
	}
	return ctx, err
}

func metaData(ctx context.Context) {
	var artist, title *C.char
	fmtCtx := ctx.Value(fmtCtxKey).(*C.AVFormatContext)
	C.get_metadata(fmtCtx, &artist, &title)
	file := ctx.Value(fileKey).(*File)
	file.Artist = C.GoString(artist)
	file.Title = C.GoString(title)
}

func duration(ctx context.Context) context.Context {
	durationInFormat := false
	if fmtCtx := ctx.Value(fmtCtxKey).(*C.AVFormatContext); fmtCtx.duration > 0 {
		durationInFormat = true
		ctx.Value(fileKey).(*File).Duration = time.Duration(1000 * fmtCtx.duration)
	}
	return context.WithValue(ctx, durationKey, durationInFormat)

}

func fullDuration(ctx context.Context) error {
	defer freeFormatContext(ctx)
	if ctx.Value(durationKey).(bool) {
		return nil
	}
	newDuration := time.Duration(C.find_duration(ctx.Value(fmtCtxKey).(*C.AVFormatContext)))
	if newDuration < 0 {
		return avError(newDuration)
	}
	file := ctx.Value(fileKey).(*File)
	if newDuration > file.Duration {
		file.Duration = newDuration
	}
	return nil
}

func findStreams(ctx context.Context) (context.Context, error) {
	var vStream *C.AVStream
	var orientation C.int
	err := C.find_streams(ctx.Value(fmtCtxKey).(*C.AVFormatContext), &vStream, &orientation)
	if err < 0 {
		return ctx, avError(err)
	}
	ctx = context.WithValue(ctx, streamKey, vStream)
	file := ctx.Value(fileKey).(*File)
	file.HasVideo = err&hasVideo != 0
	file.HasAudio = err&hasAudio != 0
	if !file.HasVideo {
		file.Media = "audio"
	} else {
		file.Width = int(vStream.codecpar.width)
		file.Height = int(vStream.codecpar.height)
		file.Orientation = int(orientation)
	}
	return ctx, nil
}

func createDecoder(ctx context.Context) (context.Context, error) {
	var decCtx *C.AVCodecContext
	err := C.create_codec_context(ctx.Value(streamKey).(*C.AVStream), &decCtx)
	if err < 0 {
		return ctx, avError(err)
	}
	ctx = context.WithValue(ctx, decCtxKey, decCtx)
	defer C.avcodec_free_context(&decCtx)
	return createThumbContext(ctx)
}

func incrementDuration(ctx context.Context, frame *C.AVFrame) {
	if !ctx.Value(durationKey).(bool) && frame.pts != C.AV_NOPTS_VALUE {
		vStream := ctx.Value(streamKey).(*C.AVStream)
		ptsToNano := C.int64_t(1000000000 * vStream.time_base.num / vStream.time_base.den)
		newDuration := time.Duration(frame.pts * ptsToNano)
		file := ctx.Value(fileKey).(*File)
		if newDuration > file.Duration {
			file.Duration = newDuration
		}
	}
}

func populateHistogram(ctx context.Context, frames <-chan *C.AVFrame) <-chan struct{} {
	done := make(chan struct{})
	thumbCtx := ctx.Value(thumbCtxKey).(*C.ThumbContext)
	go func() {
		var n C.int
		for frame := range frames {
			C.populate_histogram(thumbCtx, n, frame)
			n++
		}
		thumbCtx.n = n
		done <- struct{}{}
		close(done)
	}()
	return done
}

func createThumbContext(ctx context.Context) (context.Context, error) {
	pkt := C.create_packet()
	fmtCtx := ctx.Value(fmtCtxKey).(*C.AVFormatContext)
	vStream := ctx.Value(streamKey).(*C.AVStream)
	decCtx := ctx.Value(decCtxKey).(*C.AVCodecContext)
	var frame *C.AVFrame
	var thumbCtx *C.ThumbContext
	err := C.obtain_next_frame(fmtCtx, decCtx, vStream.index, &pkt, &frame)
	if err >= 0 {
		incrementDuration(ctx, frame)
		thumbCtx = C.create_thumb_context(vStream, frame)
		if thumbCtx == nil {
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
		return ctx, avError(err)
	}
	ctx = context.WithValue(ctx, thumbCtxKey, thumbCtx)
	defer C.free_thumb_context(thumbCtx)
	frames := make(chan *C.AVFrame, thumbCtx.max_frames)
	done := populateHistogram(ctx, frames)
	frames <- frame
	if pkt.buf != nil {
		C.av_packet_unref(&pkt)
	}
	return populateThumbContext(ctx, frames, done)
}

func populateThumbContext(ctx context.Context, frames chan *C.AVFrame, done <-chan struct{}) (context.Context, error) {
	pkt := C.create_packet()
	var frame *C.AVFrame
	fmtCtx := ctx.Value(fmtCtxKey).(*C.AVFormatContext)
	vStream := ctx.Value(streamKey).(*C.AVStream)
	decCtx := ctx.Value(decCtxKey).(*C.AVCodecContext)
	thumbCtx := ctx.Value(thumbCtxKey).(*C.ThumbContext)
	var err C.int
	for i := C.int(1); i < thumbCtx.max_frames; i++ {
		err = C.obtain_next_frame(fmtCtx, decCtx, vStream.index, &pkt, &frame)
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
		return ctx, avError(err)
	}
	return convertFrameToRGB(ctx)
}

func convertFrameToRGB(ctx context.Context) (context.Context, error) {
	thumbCtx := ctx.Value(thumbCtxKey).(*C.ThumbContext)
	outputFrame := C.convert_frame_to_rgb(C.process_frames(thumbCtx), thumbCtx.alpha)
	if outputFrame == nil {
		return ctx, avErrNoMem
	}
	ctx = context.WithValue(ctx, frameKey, outputFrame)
	ctx.Value(fileKey).(*File).HasAlpha = thumbCtx.alpha != 0
	return ctx, nil
}

func thumbnail(ctx context.Context) <-chan error {
	errCh := make(chan error)
	outputFrame := ctx.Value(frameKey).(*C.AVFrame)
	go func() {
		err := thumbnailFromFFmpeg(ctx.Value(fileKey).(*File), outputFrame.data[0])
		C.av_frame_free(&outputFrame)
		errCh <- err
		close(errCh)
	}()
	return errCh
}

func ffmpegThumbnail(ctx context.Context, file *File) error {
	ctx = context.WithValue(ctx, fileKey, file)
	callbackFlags := C.int(readCallbackFlag | interruptCallbackFlag)
	if file.Seeker != nil {
		callbackFlags |= seekCallbackFlag
	}
	ctx, err := createFormatContext(ctx, callbackFlags)
	if err != nil {
		return err
	}
	if !file.HasVideo {
		return fullDuration(ctx)
	}
	if ctx, err = createDecoder(ctx); err == errTooBig || err == avErrDecoderNotFound {
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
