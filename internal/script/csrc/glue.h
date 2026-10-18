/*
 * Shared declarations of the binding between Go and C Lua 5.4.
 * glue.c has the code. The cgo build and lua.wasm both compile all.c,
 * which includes Lua and glue.c in one translation unit.
 */
#ifndef glue_h
#define glue_h

#include <stdint.h>
#include <stddef.h>

#include "lua.h"
#include "glue_abi.h"

typedef struct glue_state {
	glue_hdr h;
	lua_State *L;
	int64_t handle;     /* the Go state's handle */
	int64_t cpu_limit;  /* instruction budget per call, 0 for none */
	int64_t budget;     /* what is left of it */
	int64_t timeout;    /* nanoseconds per call, 0 for none */
	int64_t deadline;   /* -1 until the first hook of a call */
	size_t mem_limit;   /* bytes, 0 for none */
	int limited;        /* 1 while a call runs */
	int memhit;         /* the last allocation was refused for the memory limit */
	lua_State *resumed; /* the thread Go resumed, if any */
	lua_State *cur;     /* the thread that called Go, during a host call */
	int host_top;       /* the top of cur's stack when the host call began */
	int tmp_ref;        /* registry reference of the temporary table */
	int ntmp;           /* entries used in it since the last reset */
	int maxtmp;         /* highest entry used since the operation began */
} glue_state;

static inline glue_state *glue_of(lua_State *L) {
	return *(glue_state **)lua_getextraspace(L);
}

void glue_limit_error(lua_State *L);

/*
 * glue_charge takes n units from the CPU budget of the running call and
 * stops the call when the budget is used up. One unit is about one Lua
 * instruction. The patched lstrlib.c calls it while matching patterns.
 */
static inline void glue_charge(lua_State *L, int64_t n) {
	glue_state *st = glue_of(L);
	st->budget -= n;
	st->h.spent += n;
	if (st->budget <= 0 && st->limited)
		glue_limit_error(L);
}

#endif
