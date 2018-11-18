#include "vips.h"

static int has_alpha(VipsImage *in, gboolean *has_alpha) {
    if ((*has_alpha = vips_image_hasalpha(in)) == FALSE) {
        return 0;
    }
    VipsImage *out;
    if (vips_extract_band(in, &out, vips_image_get_bands(in) - 1, NULL)) {
        return -1;
    }
    if (vips_image_get_format(out) != VIPS_FORMAT_UCHAR) {
        VipsImage *down;
        int err = vips_cast_uchar(out, &down, NULL);
        g_object_unref(out);
        if (err) {
            return -1;
        }
        out = down;
    }
    double min;
    int err = vips_min(out, &min, NULL);
    g_object_unref(out);
    if (min == 255) {
        *has_alpha = FALSE;
    }
    return err;
}

int thumbnail(RawThumbnail *thumb) {
    VipsImage *in, *out;
    if (!thumb->input_path) {
        VipsImage *tmp;
        if (!(tmp = vips_image_new_from_memory(thumb->input, thumb->input_size, thumb->width, thumb->height,
                                              thumb->bands, VIPS_FORMAT_UCHAR))) {
            return -1;
        }
        GValue orientation = G_VALUE_INIT;
        g_value_init(&orientation, G_TYPE_INT);
        g_value_set_int(&orientation, thumb->orientation);
        vips_image_set(tmp, VIPS_META_ORIENTATION, &orientation);
        int err = vips_copy(tmp, &in, "interpretation", VIPS_INTERPRETATION_RGB, NULL);
        g_object_unref(tmp);
        if (err) {
            return -1;
        }

    } else {
        if (!(in = vips_image_new_from_file(thumb->input_path, NULL))) {
            return -1;
        }
        thumb->width = vips_image_get_width(in);
        thumb->height = vips_image_get_height(in);
        if (!vips_image_get_typeof(in, VIPS_META_ORIENTATION) ||
            vips_image_get_int(in, VIPS_META_ORIENTATION, &thumb->orientation)) {
            thumb->orientation = 1;
        }
    }

    int err = vips_thumbnail_image(in, &out, thumb->target_size, "size", VIPS_SIZE_DOWN, NULL);
    g_object_unref(in);
    if (err) {
        return -1;
    }
    thumb->thumb_width = vips_image_get_width(out);
    thumb->thumb_height = vips_image_get_height(out);
    if (has_alpha(out, &thumb->has_alpha)) {
        g_object_unref(out);
        return -1;
    }

    if (!thumb->output_path) {
        if (!thumb->has_alpha) {
            err = vips_jpegsave_buffer(out, (void **) &thumb->output, &thumb->output_size, "Q", thumb->quality, "strip",
                                       TRUE, "optimize-coding", TRUE, NULL);
        } else {
            err = vips_pngsave_buffer(out, (void **) &thumb->output, &thumb->output_size, "Q", thumb->quality, "strip",
                                      TRUE, "palette", TRUE, NULL);
        }
    } else {
        if (!thumb->has_alpha) {
            err = vips_jpegsave(out, thumb->output_path, "Q", thumb->quality, "strip", TRUE, "optimize-coding", TRUE,
                                NULL);
        } else {
            err = vips_pngsave(out, thumb->output_path, "Q", thumb->quality, "strip", TRUE, "palette", TRUE, NULL);
        }
    }
    g_object_unref(out);
    return err;
}
