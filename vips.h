#include <vips/vips.h>

typedef struct RawThumbnail {
    int width, height;
    int thumb_width, thumb_height;
    int orientation, target_size, bands, quality;
    unsigned char *input, *output;
    size_t input_size, output_size;
    char *input_path, *output_path;
    gboolean has_alpha;
} RawThumbnail;

void vips_error_push_back(char *domain, char *fmt);

int init_vips();

void shutdown_vips();

void shutdown_vips_thread_on_error();

int thumbnail(RawThumbnail *thumb);