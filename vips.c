#include "vips.h"

int thumbnail_from_ffmpeg(unsigned char *data, size_t size, int width, int height, int bands) {
    VipsImage *in, *out;
    if (!(in = vips_image_new_from_memory(data, size, width, height, bands, VIPS_FORMAT_UCHAR))) {
        return -1;
    }
    if (vips_thumbnail_image(in, &out, 256, "size", VIPS_SIZE_DOWN, NULL)) {
        return -1;
    }
    g_object_unref(in);
    int ret = vips_image_write_to_file(out, "thumbnail.jpg", NULL);
    g_object_unref(out);
    return ret;
}