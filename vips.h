#include <vips/vips.h>
#include <vips/foreign.h>

typedef struct RawThumbnail {
    int width, height, target_size, bands, quality;
    unsigned char *input, *output;
    size_t input_size, output_size;
    char *input_path, *output_path;
    gboolean has_alpha;
} RawThumbnail;

int init_vips();

void shutdown_vips_thread_on_error();

int thumbnail(RawThumbnail *thumb);