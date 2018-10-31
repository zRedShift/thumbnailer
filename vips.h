#include <vips/vips.h>
#include <vips/foreign.h>

int thumbnail_from_ffmpeg(unsigned char *data, size_t size, int width, int height, int bands);