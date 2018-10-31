package thumbnailer

import "C"

//#cgo pkg-config: libavformat libavutil libavcodec libswscale
// #cgo CFLAGS: -std=c11 -O3 -DNDEBUG
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
	ctx, ok := contexts.Get(opaque)
	if !ok {
		return C.int(avErrUnknown)
	}
	p := (*[1 << 30]byte)(unsafe.Pointer(buf))[:bufSize:bufSize]
	n, err := ctx.Read(p)
	if err == io.EOF {
		return C.int(avErrEOF)
	}
	if err != nil {
		return C.int(avErrUnknown)
	}
	return C.int(n)
}

//export writeCallback
func writeCallback(opaque unsafe.Pointer, buf *C.uint8_t, bufSize C.int) C.int {
	ctx, ok := contexts.Get(opaque)
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
	ctx, ok := contexts.Get(opaque)
	if !ok {
		return C.int64_t(avErrUnknown)
	}
	if !ctx.CanSeekToEnd && whence >= C.SEEK_END {
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
	if ctx, ok := contexts.Get(opaque); ok && ctx.Interrupt != nil {
		select {
		case <-ctx.Interrupt.Done():
			return 1
		default:
			return 0
		}
	}
	return 0
}

func init() {
	C.av_log_set_level(C.AV_LOG_ERROR)
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

type avError C.int

var (
	avErrNoMem           = -avError(syscall.ENOMEM)
	avErrEOF             = avError(C.AVERROR_EOF)
	avErrUnknown         = avError(C.AVERROR_UNKNOWN)
	avErrDecoderNotFound = avError(C.AVERROR_DECODER_NOT_FOUND)
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

type avContextMap struct {
	sync.RWMutex
	m map[uintptr]*avContext
}

func (m *avContextMap) Get(opaque unsafe.Pointer) (ctx *avContext, ok bool) {
	m.RLock()
	ctx, ok = m.m[uintptr(opaque)]
	m.RUnlock()
	return
}

func (m *avContextMap) Set(ctx *avContext) {
	m.Lock()
	m.m[uintptr(unsafe.Pointer(ctx.formatContext))] = ctx
	m.Unlock()
	return
}

func (m *avContextMap) Delete(ctx *avContext) {
	m.Lock()
	delete(m.m, uintptr(unsafe.Pointer(ctx.formatContext)))
	m.Unlock()
	return
}

var contexts = avContextMap{m: make(map[uintptr]*avContext)}

type avContext struct {
	formatContext  *C.AVFormatContext
	decoderContext *C.AVCodecContext
	videoStream    *C.AVStream
	thumbContext   *C.ThumbContext
	outputFrame    *C.AVFrame
	*Content
	durationInFormat bool
	Interrupt        context.Context
}

func (ctx *avContext) freeFormatContext() {
	contexts.Delete(ctx)
	if ctx.formatContext != nil {
		C.free_format_context(ctx.formatContext)
	}
	ctx.videoStream = nil
	ctx.formatContext = nil
}

func createAVContext(outsideCtx context.Context, content *Content) (ctx *avContext, err error) {
	callbackFlags := C.int(readCallbackFlag)
	if content.Seeker != nil {
		callbackFlags |= seekCallbackFlag
	}
	if outsideCtx != nil {
		callbackFlags |= interruptCallbackFlag
	}
	ctx = new(avContext)
	defer func() {
		if err != nil {
			ctx.freeFormatContext()
			ctx = nil
		}
	}()
	ctx.Content = content
	ctx.Interrupt = outsideCtx
	intErr := C.allocate_format_context(&ctx.formatContext)
	if intErr < 0 {
		return ctx, avError(intErr)
	}
	contexts.Set(ctx)
	intErr = C.create_format_context(ctx.formatContext, callbackFlags)
	if intErr < 0 {
		return ctx, avError(intErr)
	}
	ctx.getMetaData()
	ctx.getDuration()
	return ctx, ctx.findStreams()
}

func (ctx *avContext) getMetaData() {
	var artist, title *C.char
	C.get_metadata(ctx.formatContext, &artist, &title)
	ctx.Artist = C.GoString(artist)
	ctx.Title = C.GoString(title)
}

func (ctx *avContext) getDuration() {
	if ctx.formatContext.duration > 0 {
		ctx.durationInFormat = true
		ctx.Duration = time.Duration(1000 * ctx.formatContext.duration)
	}
}

func (ctx *avContext) incrementDuration() {
	if !ctx.durationInFormat && ctx.outputFrame.pts != C.AV_NOPTS_VALUE {
		ptsToNanoseconds := C.int64_t(1000000000 * ctx.videoStream.time_base.num /
			ctx.videoStream.time_base.den)
		newDuration := time.Duration(ctx.outputFrame.pts * ptsToNanoseconds)
		if newDuration > ctx.Duration {
			ctx.Duration = newDuration
		}
	}
}

func (ctx *avContext) fullDuration() error {
	defer ctx.freeFormatContext()
	if ctx.durationInFormat {
		return nil
	}
	newDuration := time.Duration(C.find_duration(ctx.formatContext))
	if newDuration < 0 {
		return avError(newDuration)
	}
	if newDuration > ctx.Duration {
		ctx.Duration = newDuration
	}
	return nil
}

func (ctx *avContext) findStreams() error {
	err := C.find_streams(ctx.formatContext, &ctx.videoStream)
	if err < 0 {
		return avError(err)
	}
	ctx.HasVideo = err&hasVideo != 0
	ctx.HasAudio = err&hasAudio != 0
	if !ctx.HasVideo {
		ctx.Media = "audio"
		return nil
	}
	ctx.Width = int(ctx.videoStream.codecpar.width)
	ctx.Height = int(ctx.videoStream.codecpar.height)
	return nil
}

func (ctx *avContext) createDecoder() error {
	err := C.create_codec_context(ctx.videoStream, &ctx.decoderContext)
	if err < 0 {
		return avError(err)
	}
	defer C.avcodec_free_context(&ctx.decoderContext)
	return ctx.createThumbContext()
}

func (ctx *avContext) populateHistogram(frames <- chan *C.AVFrame) <- chan struct{} {
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

func (ctx *avContext) createThumbContext() error {
	pkt := C.create_packet()
	err := C.obtain_next_frame(ctx.formatContext, ctx.decoderContext, ctx.videoStream.index, &pkt, &ctx.outputFrame)
	if err >= 0 {
		ctx.incrementDuration()
		ctx.thumbContext = C.create_thumb_context(ctx.videoStream, ctx.outputFrame)
		if ctx.thumbContext == nil {
			err = C.int(avErrNoMem)
		}
	}
	if err < 0 {
		if pkt.buf != nil {
			C.av_packet_unref(&pkt)
		}
		if ctx.outputFrame != nil {
			C.av_frame_free(&ctx.outputFrame)
		}
		return avError(err)
	}
	defer func() {
		C.free_thumb_context(ctx.thumbContext)
		ctx.thumbContext = nil
	}()
	frames := make(chan *C.AVFrame, ctx.thumbContext.max_frames)
	done := ctx.populateHistogram(frames)
	frames <- ctx.outputFrame
	ctx.outputFrame = nil
	if pkt.buf != nil {
		C.av_packet_unref(&pkt)
	}
	return ctx.populateThumbContext(frames, done)
}

func (ctx *avContext) populateThumbContext(frames chan *C.AVFrame, done <- chan struct{}) error {
	pkt := C.create_packet()
	var err C.int
	for i := C.int(1); i < ctx.thumbContext.max_frames; i++ {
		err = C.obtain_next_frame(ctx.formatContext, ctx.decoderContext, ctx.videoStream.index, &pkt, &ctx.outputFrame)
		if err < 0 {
			break
		}
		ctx.incrementDuration()
		frames <- ctx.outputFrame
		ctx.outputFrame = nil
	}
	close(frames)
	if pkt.buf != nil {
		C.av_packet_unref(&pkt)
	}
	if ctx.outputFrame != nil {
		C.av_frame_free(&ctx.outputFrame)
	}
	<-done
	if err != 0 && err != C.int(avErrEOF) {
		return avError(err)
	}
	return ctx.convertFrameToRGB()
}

func (ctx *avContext) convertFrameToRGB() error {
	ctx.outputFrame = C.convert_frame_to_rgb(C.process_frames(ctx.thumbContext), ctx.thumbContext.alpha)
	if ctx.outputFrame == nil {
		return avErrNoMem
	}
	ctx.AlphaChannel = ctx.thumbContext.alpha != 0
	return nil
}

func freeOutputFrame(outputFrame *C.AVFrame) {
	C.av_frame_free(&outputFrame)
}

func (ctx *avContext) thumbnail() <-chan error {
	errCh := make(chan error)
	go func() {
		err := ctx.thumbnailFromBytes(ctx.outputFrame.data[0])
		freeOutputFrame(ctx.outputFrame)
		errCh <- err
		close(errCh)
	}()
	return errCh
}

func ffmpegThumbnail(outsideCtx context.Context, content *Content) (bool, error) {
	ctx, err := createAVContext(outsideCtx, content)
	if err != nil {
		return false, err
	}
	if !ctx.HasVideo {
		return false, ctx.fullDuration()
	}
	if err = ctx.createDecoder(); err == errTooBig || err == avErrDecoderNotFound {
		return false, ctx.fullDuration()
	}
	if err != nil {
		ctx.freeFormatContext()
		return false, err
	}
	errCh := ctx.thumbnail()
	err = ctx.fullDuration()
	thumbErr := <-errCh
	if thumbErr != nil {
		return false, err
	}
	return true, err
}