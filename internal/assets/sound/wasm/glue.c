// Glue between libopus, the speexdsp resampler, and the Go host in
// ../opus.go.

#include <stdint.h>
#include <stdlib.h>
#include <string.h>

#include "opus.h"
#include "speex_resampler.h"

#define RATE 48000
// One Opus packet holds 20 ms.
#define FRAME 960
#define MAX_PACKET 4000

__attribute__((export_name("wa_alloc"))) void *wa_alloc(size_t n) {
	return malloc(n);
}

__attribute__((export_name("wa_free"))) void wa_free(void *p) {
	free(p);
}

// resample converts frames of interleaved float samples at rate to 48 kHz.
// It returns a buffer of *outFrames frames, or NULL on error. The output
// has exactly frames × 48000 / rate frames, rounded, and is aligned with
// the input: the resampler's delay is removed.
static float *resample(const float *in, int frames, int channels, int rate, int *outFrames) {
	int64_t want = ((int64_t)frames * RATE + rate / 2) / rate;
	float *out = malloc((size_t)(want + 1) * channels * sizeof(float));
	if (out == NULL) {
		return NULL;
	}
	if (rate == RATE) {
		memcpy(out, in, (size_t)frames * channels * sizeof(float));
		*outFrames = frames;
		return out;
	}
	int err = 0;
	SpeexResamplerState *st = speex_resampler_init(channels, rate, RATE, SPEEX_RESAMPLER_QUALITY_MAX, &err);
	if (st == NULL) {
		free(out);
		return NULL;
	}
	// Skipping the zeros the filter starts with aligns the output with the
	// input. Feeding the filter's latency in zeros at the end flushes it.
	speex_resampler_skip_zeros(st);
	int latency = speex_resampler_get_input_latency(st);
	float *zeros = calloc((size_t)latency * channels + channels, sizeof(float));
	int64_t done = 0;
	const float *src = in;
	spx_uint32_t left = (spx_uint32_t)frames;
	for (int pass = 0; pass < 2 && done < want; pass++) {
		while (left > 0 && done < want) {
			spx_uint32_t inLen = left;
			spx_uint32_t outLen = (spx_uint32_t)(want - done);
			speex_resampler_process_interleaved_float(st, src, &inLen, out + done * channels, &outLen);
			if (inLen == 0 && outLen == 0) {
				break;
			}
			src += inLen * channels;
			left -= inLen;
			done += outLen;
		}
		src = zeros;
		left = (spx_uint32_t)latency;
	}
	speex_resampler_destroy(st);
	free(zeros);
	// Any frames still missing are silence.
	for (int64_t i = done * channels; i < want * channels; i++) {
		out[i] = 0;
	}
	*outFrames = (int)want;
	return out;
}

// wa_encode encodes frames of interleaved float samples at rate as Opus
// packets at 48 kHz. The output is a list of packets, each a 2-byte
// little-endian length and the packet. It stores the list in *out, which
// wa_free releases, and returns its size, or a negative number on error.
// info receives the encoder's pre-skip and the 48 kHz frame count, before
// the last packet was padded.
__attribute__((export_name("wa_encode"))) int wa_encode(
	const float *pcm, int frames, int channels, int rate, int bitrate,
	uint8_t **out, int32_t *info) {
	*out = NULL;
	if (frames <= 0 || channels < 1 || channels > 2 || rate < 1000 || rate > 384000) {
		return -1;
	}
	int n = 0;
	float *in = resample(pcm, frames, channels, rate, &n);
	if (in == NULL) {
		return -2;
	}
	int err = 0;
	OpusEncoder *enc = opus_encoder_create(RATE, channels, OPUS_APPLICATION_AUDIO, &err);
	if (enc == NULL) {
		free(in);
		return -3;
	}
	opus_encoder_ctl(enc, OPUS_SET_BITRATE(bitrate));
	opus_encoder_ctl(enc, OPUS_SET_COMPLEXITY(10));
	opus_encoder_ctl(enc, OPUS_SET_VBR(1));
	opus_int32 lookahead = 0;
	opus_encoder_ctl(enc, OPUS_GET_LOOKAHEAD(&lookahead));

	// The encoder's lookahead delays the output, so the input is padded
	// with that much silence at the end, then to a whole packet.
	int total = n + lookahead;
	int packets = (total + FRAME - 1) / FRAME;
	size_t cap = (size_t)packets * (MAX_PACKET + 2);
	uint8_t *buf = malloc(cap);
	float *frame = malloc((size_t)FRAME * channels * sizeof(float));
	size_t size = 0;
	int ret = 0;
	if (buf == NULL || frame == NULL) {
		ret = -2;
	}
	for (int p = 0; p < packets && ret == 0; p++) {
		for (int i = 0; i < FRAME; i++) {
			int s = p * FRAME + i;
			for (int c = 0; c < channels; c++) {
				frame[i * channels + c] = s < n ? in[(size_t)s * channels + c] : 0;
			}
		}
		opus_int32 len = opus_encode_float(enc, frame, FRAME, buf + size + 2, MAX_PACKET);
		if (len < 0) {
			ret = -4;
			break;
		}
		buf[size] = (uint8_t)(len & 0xff);
		buf[size + 1] = (uint8_t)(len >> 8);
		size += 2 + (size_t)len;
	}
	opus_encoder_destroy(enc);
	free(in);
	free(frame);
	if (ret != 0) {
		free(buf);
		return ret;
	}
	info[0] = lookahead;
	info[1] = n;
	*out = buf;
	return (int)size;
}
