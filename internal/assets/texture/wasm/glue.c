// Glue between libwebp's encoder and the Go host in ../webp.go.

#include <stdint.h>
#include <stdlib.h>

#include "src/webp/encode.h"

__attribute__((export_name("wa_alloc"))) void *wa_alloc(size_t n) {
	return malloc(n);
}

__attribute__((export_name("wa_free"))) void wa_free(void *p) {
	free(p);
}

// wa_encode_lossless encodes w × h RGBA pixels as lossless WebP with the
// given effort level, 0 to 9. It stores a buffer that wa_free releases in
// *out and returns its size, or returns 0 on error. The colors of fully
// transparent pixels are kept, because the host sets them on purpose.
__attribute__((export_name("wa_encode_lossless"))) int wa_encode_lossless(
	const uint8_t *rgba, int w, int h, int level, uint8_t **out) {
	WebPConfig config;
	WebPPicture pic;
	WebPMemoryWriter writer;
	int ok;

	*out = NULL;
	if (!WebPConfigInit(&config) || !WebPConfigLosslessPreset(&config, level)) {
		return 0;
	}
	config.exact = 1;
	if (!WebPPictureInit(&pic)) {
		return 0;
	}
	pic.use_argb = 1;
	pic.width = w;
	pic.height = h;
	if (!WebPPictureImportRGBA(&pic, rgba, w * 4)) {
		WebPPictureFree(&pic);
		return 0;
	}
	WebPMemoryWriterInit(&writer);
	pic.writer = WebPMemoryWrite;
	pic.custom_ptr = &writer;
	ok = WebPEncode(&config, &pic);
	WebPPictureFree(&pic);
	if (!ok) {
		WebPMemoryWriterClear(&writer);
		return 0;
	}
	*out = writer.mem;
	return (int)writer.size;
}
