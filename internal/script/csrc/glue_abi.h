/*
 * The interface between Go and glue.c. The cgo build includes this file in
 * its preamble. The WebAssembly build reads the same layouts from memory.
 * Every layout here is the same on 64-bit hosts and in wasm32.
 */
#ifndef glue_abi_h
#define glue_abi_h

#include <stdint.h>

/* One value passed between Go and C. */
typedef struct glue_value {
	int64_t i;    /* integer, boolean, float bits, address, or index */
	int32_t type; /* one of GLUE_T* */
	int32_t len;  /* string length, pairs that follow a table, or Lua type */
} glue_value;

enum {
	GLUE_TNIL = 0,
	GLUE_TBOOL = 1,
	GLUE_TINT = 2,
	GLUE_TFLOAT = 3,
	GLUE_TSTRING = 4, /* i is the address of len bytes */
	GLUE_TTABLE = 5,  /* len key and value pairs follow */
	GLUE_TTMP = 6,    /* i is an index in the temporary table */
	GLUE_TREF = 7     /* i is a registry reference */
};

/* The start of each state. Go reads and writes it. */
typedef struct glue_hdr {
	int64_t vals;  /* address of the value array */
	int64_t vcap;  /* values it can hold */
	int64_t buf;   /* address of the byte buffer for strings from Go */
	int64_t bcap;  /* bytes it can hold */
	int64_t nin;   /* values Go wrote, counting every slot */
	int64_t nres;  /* top-level values in the exchange, from either side */
	int64_t fatal; /* the limit the last operation hit, GLUE_F* */
	int64_t mem;   /* bytes the Lua state has allocated */
} glue_hdr;

/* Limits that stop a call. pcall cannot catch them. */
enum {
	GLUE_FNONE = 0,
	GLUE_FCPU = 1,
	GLUE_FMEM = 2,
	GLUE_FTIME = 3
};

/* Operations of glue_op. */
enum {
	GLUE_OCALL = 1,        /* a: arguments. Calls value 0 with the rest. */
	GLUE_ORESUME = 2,      /* a: thread reference, b: arguments. */
	GLUE_OLOAD = 3,        /* Runs source value 1 named value 0. */
	GLUE_ONEWTHREAD = 4,   /* Returns a reference to a thread for value 0. */
	GLUE_OKEEP = 5,        /* Returns a registry reference to value 0. */
	GLUE_OUNREF = 6,       /* a: reference to release. */
	GLUE_OCLOSETHREAD = 7, /* a: thread reference to close and release. */
	GLUE_OSETGLOBAL = 8,   /* Sets global value 0 to value 1. */
	GLUE_OGETGLOBAL = 9,   /* Returns global value 0. */
	GLUE_OHOSTFN = 10,     /* a: host function ID. Returns a reference. */
	GLUE_ORESERVE = 11,    /* a: values, b: bytes the exchange must hold. */
	GLUE_OGC = 12,         /* Runs a full garbage collection. */
	GLUE_ONESTED = 13      /* a: arguments. Calls value 0 inside a host call. */
};

/* Results of glue_op besides Lua's status codes and references. */
enum {
	GLUE_EENCODE = 10, /* the results cannot be passed to Go */
	GLUE_EARG = 11     /* the operation's input is not valid */
};

/* What a host function returns to C. Counts are in nres. */
enum {
	GLUE_HRETURN = 0,
	GLUE_HERROR = 1, /* raise value 0 as a Lua error */
	GLUE_HYIELD = 2  /* pause the thread Go resumed */
};

/* Bit 0 of the flags passed to host_call: the host function may pause. */
#define GLUE_CANYIELD 1

glue_hdr *glue_new(int64_t handle, int64_t cpu, int64_t mem, int64_t timeout);
void glue_close(glue_hdr *h);
int32_t glue_op(glue_hdr *h, int32_t op, int64_t a, int64_t b);

#endif
