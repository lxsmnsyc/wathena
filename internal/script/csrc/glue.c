/*
 * Binding between Go and C Lua 5.4. all.c includes this file after Lua, so
 * it is one translation unit with Lua. The same code runs through cgo and,
 * compiled to WebAssembly, in wazero.
 *
 * Rules:
 *   - Go and C pass values through an exchange: an array of glue_value and
 *     a byte buffer, both in C memory. Go writes them without a call into C.
 *   - Each call from Lua into Go crosses once, through host_call.
 *   - Lua errors and yields never cross Go frames. A host function returns
 *     to C, and C raises the error or yields.
 *   - Each state has a memory limit (the allocator) and a CPU budget per
 *     call (a count hook, allocations, and costly library functions).
 *     pcall and coroutine.resume cannot catch a call that hit a limit.
 */
#include "glue.h"

#ifdef __wasm__
#define HOST(name) __attribute__((import_module("env"), import_name(#name)))
#define EXPORT(name) __attribute__((export_name(#name)))
#else
#define HOST(name)
#define EXPORT(name)
#endif

/* host_call runs host function id with nargs values from the exchange. */
HOST(host_call) int32_t host_call(int64_t handle, int32_t id, int32_t nargs, int32_t flags);

/* The CPU hook runs every GLUE_HOOK_EVERY instructions. */
#define GLUE_HOOK_EVERY 1000

/* Tables passed between Go and Lua are copied up to this depth and size. */
#define GLUE_MAX_DEPTH 16
#define GLUE_MAX_VALUES (1 << 20)

/* The host function ID of the require loader. Go registers it first. */
#define GLUE_REQUIRE_ID 0

/* Cost classes of library functions, for glue_charged. */
#define GLUE_CSTR 1  /* scans its string arguments: 1 unit per 2 bytes */
#define GLUE_CRES 2  /* returns many values */
#define GLUE_CTAB 4  /* walks the table in argument 1 */
#define GLUE_CSORT 8 /* sorts the table in argument 1 */
#define GLUE_CMOVE 16 /* moves elements f to e of table.move */

static const char glue_loading = 0; /* marks a module that is loading */

/* ---- Time ---- */

#ifdef _WIN32
__declspec(dllimport) int __stdcall QueryPerformanceCounter(int64_t *count);
__declspec(dllimport) int __stdcall QueryPerformanceFrequency(int64_t *freq);

static int64_t glue_now(void) {
	int64_t c, f;
	QueryPerformanceCounter(&c);
	QueryPerformanceFrequency(&f);
	return (int64_t)((double)c * 1e9 / (double)f);
}
#elif defined(__APPLE__)
#include <mach/mach_time.h>

/* mach_absolute_time is several times cheaper than clock_gettime here. */
static int64_t glue_now(void) {
	static mach_timebase_info_data_t tb;
	if (tb.denom == 0)
		mach_timebase_info(&tb);
	return (int64_t)(mach_absolute_time() * tb.numer / tb.denom);
}
#else
/* The coarse clock is cheaper, and a few milliseconds of resolution are
 * enough for a limit of a second. */
#ifdef CLOCK_MONOTONIC_COARSE
#define GLUE_CLOCK CLOCK_MONOTONIC_COARSE
#else
#define GLUE_CLOCK CLOCK_MONOTONIC
#endif
static int64_t glue_now(void) {
	struct timespec ts;
	clock_gettime(GLUE_CLOCK, &ts);
	return (int64_t)ts.tv_sec * 1000000000 + (int64_t)ts.tv_nsec;
}
#endif

/* ---- Limits ---- */

/*
 * glue_alloc is the state's allocator. While a call runs, it refuses to grow
 * past the memory limit, and it charges the CPU budget one unit per 64 bytes.
 * When the budget is used up it refuses, and Lua raises a memory error that
 * pcall cannot catch.
 */
static void *glue_alloc(void *ud, void *ptr, size_t osize, size_t nsize) {
	glue_state *st = ud;
	void *p;
	if (ptr == NULL)
		osize = 0;
	if (nsize == 0) {
		st->h.mem -= (int64_t)osize;
		free(ptr);
		return NULL;
	}
	if (nsize > osize && st->limited) {
		size_t grow = nsize - osize;
		if (st->mem_limit && (size_t)st->h.mem + grow > st->mem_limit) {
			st->memhit = 1;
			return NULL;
		}
		st->budget -= (int64_t)(grow >> 6);
		if (st->budget <= 0 && st->h.fatal == GLUE_FNONE) {
			st->h.fatal = GLUE_FCPU;
			return NULL;
		}
	}
	p = realloc(ptr, nsize);
	if (p != NULL) {
		st->h.mem += (int64_t)nsize - (int64_t)osize;
		st->memhit = 0;
	}
	return p;
}

/*
 * glue_stop stops the running call. The fatal flag is set first, so the
 * message can be allocated. level is where the error points: 0 for the
 * function the hook interrupted, 1 for the caller of a C function.
 */
static void glue_stop(lua_State *L, int level) {
	glue_state *st = glue_of(L);
	if (st->h.fatal == GLUE_FNONE)
		st->h.fatal = GLUE_FCPU;
	luaL_where(L, level);
	lua_pushstring(L, st->h.fatal == GLUE_FTIME ? "time limit exceeded" : "CPU limit exceeded");
	lua_concat(L, 2);
	lua_error(L);
}

void glue_limit_error(lua_State *L) {
	glue_stop(L, 1);
}

static void glue_hook(lua_State *L, lua_Debug *ar) {
	glue_state *st = glue_of(L);
	(void)ar;
	if (!st->limited)
		return;
	st->budget -= GLUE_HOOK_EVERY;
	if (st->budget <= 0)
		glue_stop(L, 0);
	if (st->timeout > 0) {
		/* The clock starts at the first hook, so short calls never read it. */
		int64_t now = glue_now();
		if (st->deadline < 0)
			st->deadline = now + st->timeout;
		else if (now > st->deadline) {
			st->h.fatal = GLUE_FTIME;
			glue_stop(L, 0);
		}
	}
}

static void glue_begin(glue_state *st) {
	st->h.fatal = GLUE_FNONE;
	st->memhit = 0;
	st->budget = st->cpu_limit > 0 ? st->cpu_limit : INT64_MAX;
	st->deadline = -1;
	st->limited = 1;
}

static void glue_end(glue_state *st) {
	st->limited = 0;
}

/* ---- The exchange ---- */

static glue_value *glue_vals(glue_state *st) {
	return (glue_value *)(intptr_t)st->h.vals;
}

static int glue_reserve(glue_state *st, int64_t nvals, int64_t nbytes) {
	if (nvals < 0 || nbytes < 0 || nvals > GLUE_MAX_VALUES || nbytes > INT32_MAX)
		return 0;
	if (nvals > st->h.vcap) {
		int64_t n = st->h.vcap * 2;
		void *p;
		if (n < nvals)
			n = nvals;
		p = realloc(glue_vals(st), (size_t)n * sizeof(glue_value));
		if (p == NULL)
			return 0;
		st->h.vals = (int64_t)(intptr_t)p;
		st->h.vcap = n;
	}
	if (nbytes > st->h.bcap) {
		int64_t n = st->h.bcap * 2;
		void *p;
		if (n < nbytes)
			n = nbytes;
		p = realloc((void *)(intptr_t)st->h.buf, (size_t)n);
		if (p == NULL)
			return 0;
		st->h.buf = (int64_t)(intptr_t)p;
		st->h.bcap = n;
	}
	return 1;
}

static void glue_set(glue_state *st, int64_t at, int32_t type, int64_t i, int32_t len) {
	glue_value *v = glue_vals(st) + at;
	v->i = i;
	v->type = type;
	v->len = len;
}

/* glue_tmp_put stores a value that Go cannot copy, such as a function, in
 * the temporary table and returns its index. Go can keep it with OKEEP until
 * the next operation clears the table. */
static int glue_tmp_put(glue_state *st, lua_State *L, int idx) {
	int n;
	idx = lua_absindex(L, idx);
	lua_rawgeti(L, LUA_REGISTRYINDEX, st->tmp_ref);
	lua_pushvalue(L, idx);
	n = ++st->ntmp;
	lua_rawseti(L, -2, n);
	lua_pop(L, 1);
	if (n > st->maxtmp)
		st->maxtmp = n;
	return n;
}

static void glue_tmp_clear(glue_state *st) {
	lua_State *L = st->L;
	int i;
	if (st->maxtmp > 0) {
		lua_rawgeti(L, LUA_REGISTRYINDEX, st->tmp_ref);
		for (i = 1; i <= st->maxtmp; i++) {
			lua_pushnil(L);
			lua_rawseti(L, -2, i);
		}
		lua_pop(L, 1);
	}
	st->ntmp = 0;
	st->maxtmp = 0;
}

/*
 * glue_put appends the value at idx to the exchange. Strings are passed by
 * address, so Go must read them before Lua runs again. Tables are copied.
 * It returns 0 when the value is too deep or too large.
 */
static int glue_put(glue_state *st, lua_State *L, int idx, int depth, int64_t *pos) {
	int64_t at = *pos;
	int t = lua_type(L, idx);
	if (!glue_reserve(st, at + 1, 0))
		return 0;
	(*pos)++;
	switch (t) {
	case LUA_TNIL:
	case LUA_TNONE:
		glue_set(st, at, GLUE_TNIL, 0, 0);
		break;
	case LUA_TBOOLEAN:
		glue_set(st, at, GLUE_TBOOL, lua_toboolean(L, idx), 0);
		break;
	case LUA_TNUMBER:
		if (lua_isinteger(L, idx))
			glue_set(st, at, GLUE_TINT, (int64_t)lua_tointeger(L, idx), 0);
		else {
			double d = (double)lua_tonumber(L, idx);
			int64_t bits;
			memcpy(&bits, &d, sizeof bits);
			glue_set(st, at, GLUE_TFLOAT, bits, 0);
		}
		break;
	case LUA_TSTRING: {
		size_t n;
		const char *s = lua_tolstring(L, idx, &n);
		if (n > INT32_MAX)
			return 0;
		glue_set(st, at, GLUE_TSTRING, (int64_t)(intptr_t)s, (int32_t)n);
		break;
	}
	case LUA_TTABLE: {
		int32_t npairs = 0;
		if (depth >= GLUE_MAX_DEPTH || !lua_checkstack(L, 4))
			return 0;
		idx = lua_absindex(L, idx);
		lua_pushnil(L);
		while (lua_next(L, idx)) {
			if (!glue_put(st, L, -2, depth + 1, pos) || !glue_put(st, L, -1, depth + 1, pos)) {
				lua_pop(L, 2);
				return 0;
			}
			lua_pop(L, 1);
			npairs++;
		}
		glue_set(st, at, GLUE_TTABLE, 0, npairs);
		break;
	}
	default:
		if (!lua_checkstack(L, 2))
			return 0;
		glue_set(st, at, GLUE_TTMP, glue_tmp_put(st, L, idx), t);
		break;
	}
	return 1;
}

/* glue_put_msg passes a constant message as value 0. */
static void glue_put_msg(glue_state *st, const char *msg) {
	glue_set(st, 0, GLUE_TSTRING, (int64_t)(intptr_t)msg, (int32_t)strlen(msg));
	st->h.nres = 1;
}

/* glue_put_results passes n values from the top of L's stack. */
static int glue_put_results(glue_state *st, lua_State *L, int n) {
	int64_t pos = 0;
	int i, base = lua_gettop(L) - n + 1;
	st->ntmp = 0;
	for (i = 0; i < n; i++) {
		if (!glue_put(st, L, base + i, 0, &pos)) {
			glue_put_msg(st, "the results are too large or too deeply nested to pass to the host");
			return GLUE_EENCODE;
		}
	}
	st->h.nres = n;
	return LUA_OK;
}

/* glue_put_error passes the error value at the top of L as a string. */
static void glue_put_error(glue_state *st, lua_State *L) {
	int64_t pos = 0;
	int t = lua_type(L, -1);
	if (t != LUA_TSTRING && t != LUA_TNUMBER)
		lua_pushfstring(L, "(error object is a %s value)", lua_typename(L, t));
	lua_tostring(L, -1);
	st->ntmp = 0;
	glue_put(st, L, -1, 0, &pos);
	st->h.nres = 1;
}

/*
 * glue_get pushes the next value Go wrote. Values outside what Go wrote,
 * and strings outside the buffer, become nil.
 */
static void glue_get(glue_state *st, lua_State *L, int64_t *pos, int depth) {
	glue_value v;
	if (*pos >= st->h.nin || *pos >= st->h.vcap) {
		lua_pushnil(L);
		return;
	}
	v = glue_vals(st)[(*pos)++];
	switch (v.type) {
	case GLUE_TBOOL:
		lua_pushboolean(L, v.i != 0);
		break;
	case GLUE_TINT:
		lua_pushinteger(L, (lua_Integer)v.i);
		break;
	case GLUE_TFLOAT: {
		double d;
		memcpy(&d, &v.i, sizeof d);
		lua_pushnumber(L, (lua_Number)d);
		break;
	}
	case GLUE_TSTRING: {
		int64_t off = v.i - st->h.buf;
		if (v.len < 0 || off < 0 || off + v.len > st->h.bcap)
			lua_pushnil(L);
		else
			lua_pushlstring(L, (const char *)(intptr_t)v.i, (size_t)v.len);
		break;
	}
	case GLUE_TTABLE: {
		int32_t i;
		if (v.len < 0 || depth >= GLUE_MAX_DEPTH || !lua_checkstack(L, 4)) {
			*pos = st->h.nin;
			lua_pushnil(L);
			break;
		}
		lua_createtable(L, 0, v.len < 64 ? v.len : 64);
		for (i = 0; i < v.len && *pos < st->h.nin; i++) {
			int bad;
			glue_get(st, L, pos, depth + 1);
			glue_get(st, L, pos, depth + 1);
			bad = lua_isnil(L, -2) ||
			      (lua_type(L, -2) == LUA_TNUMBER && lua_tonumber(L, -2) != lua_tonumber(L, -2));
			if (bad)
				lua_pop(L, 2);
			else
				lua_rawset(L, -3);
		}
		break;
	}
	case GLUE_TTMP:
		if (v.i >= 1 && v.i <= st->maxtmp) {
			lua_rawgeti(L, LUA_REGISTRYINDEX, st->tmp_ref);
			lua_rawgeti(L, -1, v.i);
			lua_remove(L, -2);
		} else
			lua_pushnil(L);
		break;
	case GLUE_TREF:
		/* Only references Go made. The main thread, the globals, and the
		 * temporary table are the binding's own. Encoding the temporary
		 * table would add to it while it is traversed. */
		if (v.i > LUA_RIDX_LAST && v.i <= INT32_MAX && v.i != st->tmp_ref)
			lua_rawgeti(L, LUA_REGISTRYINDEX, v.i);
		else
			lua_pushnil(L);
		break;
	default:
		lua_pushnil(L);
		break;
	}
}

/* ---- Host functions ---- */

static int glue_host_result(glue_state *st, lua_State *L, int32_t r) {
	int64_t pos = 0;
	int i, n = (int)st->h.nres;
	if (r == GLUE_HERROR) {
		luaL_where(L, 1);
		glue_get(st, L, &pos, 0);
		lua_concat(L, 2);
		return lua_error(L);
	}
	if (n < 0 || n > 4096)
		return luaL_error(L, "too many results from the host");
	luaL_checkstack(L, n, "too many results from the host");
	for (i = 0; i < n; i++)
		glue_get(st, L, &pos, 0);
	if (r == GLUE_HYIELD)
		return lua_yield(L, n);
	return n;
}

static int glue_call_host(glue_state *st, lua_State *L, int32_t id, int first, int nargs) {
	int64_t pos = 0;
	int i, r, flags = 0;
	lua_State *prev;
	st->ntmp = 0;
	for (i = 0; i < nargs; i++) {
		if (!glue_put(st, L, first + i, 0, &pos))
			return luaL_error(L, "argument #%d is too large or too deeply nested to pass to the host", i + 1);
	}
	st->h.nres = nargs;
	if (L == st->resumed && lua_isyieldable(L))
		flags |= GLUE_CANYIELD;
	prev = st->cur;
	st->cur = L;
	r = host_call(st->handle, id, nargs, flags);
	st->cur = prev;
	return r;
}

/* glue_hostfn is the Lua function for each host function. Upvalue 1 is its ID. */
static int glue_hostfn(lua_State *L) {
	glue_state *st = glue_of(L);
	int32_t id = (int32_t)lua_tointeger(L, lua_upvalueindex(1));
	int32_t r = glue_call_host(st, L, id, 1, lua_gettop(L));
	return glue_host_result(st, L, r);
}

/* ---- Sandbox ---- */

/*
 * glue_fatal reports whether an error with this status comes from a limit.
 * A memory error counts when the allocator refused for the memory limit and
 * nothing was allocated since. coroutine.wrap turns memory errors into
 * plain errors, so the status alone is not enough.
 */
static int glue_fatal(glue_state *st, int status) {
	if (st->limited && st->h.fatal == GLUE_FNONE && (status == LUA_ERRMEM || st->memhit))
		st->h.fatal = GLUE_FMEM;
	return st->h.fatal != GLUE_FNONE;
}

/* pcall and xpcall that let errors from a hit limit through. */
static int glue_finishpcall(lua_State *L, int status, lua_KContext extra) {
	if (status != LUA_OK && status != LUA_YIELD) {
		if (glue_fatal(glue_of(L), status))
			return lua_error(L);
		lua_pushboolean(L, 0);
		lua_pushvalue(L, -2);
		return 2;
	}
	return lua_gettop(L) - (int)extra;
}

static int glue_pcall(lua_State *L) {
	int status;
	luaL_checkany(L, 1);
	lua_pushboolean(L, 1);
	lua_insert(L, 1);
	status = lua_pcallk(L, lua_gettop(L) - 2, LUA_MULTRET, 0, 0, glue_finishpcall);
	return glue_finishpcall(L, status, 0);
}

/*
 * glue_msgh runs the message handler of xpcall, upvalue 1, unless a limit
 * stopped the call. An error from the count hook calls the handler while
 * Lua has turned hooks off, so a handler that loops would never stop.
 */
static int glue_msgh(lua_State *L) {
	if (glue_of(L)->h.fatal != GLUE_FNONE)
		return 1;
	lua_pushvalue(L, lua_upvalueindex(1));
	lua_insert(L, 1);
	lua_call(L, lua_gettop(L) - 1, 1);
	return 1;
}

static int glue_xpcall(lua_State *L) {
	int status;
	int n = lua_gettop(L);
	luaL_checktype(L, 2, LUA_TFUNCTION);
	lua_pushvalue(L, 2);
	lua_pushcclosure(L, glue_msgh, 1);
	lua_replace(L, 2);
	lua_pushboolean(L, 1);
	lua_pushvalue(L, 1);
	lua_rotate(L, 3, 2);
	status = lua_pcallk(L, n - 2, LUA_MULTRET, 2, 2, glue_finishpcall);
	return glue_finishpcall(L, status, 2);
}

/*
 * coroutine.resume and coroutine.close return errors as values. These
 * versions raise the error again when the coroutine hit a limit.
 */
static int glue_coerror(lua_State *L, lua_State *co, int r) {
	glue_state *st = glue_of(L);
	if (r >= 1 && !lua_toboolean(L, -r) && glue_fatal(st, co != NULL ? lua_status(co) : LUA_OK))
		return lua_error(L);
	return r;
}

static int glue_coresume(lua_State *L) {
	lua_State *co = lua_tothread(L, 1);
	return glue_coerror(L, co, lua_tocfunction(L, lua_upvalueindex(1))(L));
}

static int glue_coclose(lua_State *L) {
	lua_State *co = lua_tothread(L, 1);
	return glue_coerror(L, co, lua_tocfunction(L, lua_upvalueindex(1))(L));
}

/* load accepts text chunks only. */
static int glue_lload(lua_State *L) {
	int n = lua_gettop(L), r;
	size_t len;
	if (n >= 3 && !lua_isnil(L, 3)) {
		const char *mode = luaL_checkstring(L, 3);
		if (strcmp(mode, "t") != 0)
			return luaL_error(L, "load: only text chunks are allowed");
	}
	if (n < 3)
		lua_settop(L, 3);
	lua_pushliteral(L, "t");
	lua_replace(L, 3);
	/* Compiling costs about as much as a few hundred instructions, and
	 * more for each byte of source. */
	if (lua_type(L, 1) == LUA_TSTRING) {
		lua_tolstring(L, 1, &len);
		glue_charge(L, 256 + (int64_t)(len / 2));
	} else
		glue_charge(L, 256);
	r = lua_tocfunction(L, lua_upvalueindex(1))(L);
	if (r == 2 && lua_isnil(L, -2)) {
		const char *msg = lua_tostring(L, -1);
		int mem = msg != NULL && strcmp(msg, "not enough memory") == 0;
		if (glue_fatal(glue_of(L), mem ? LUA_ERRMEM : LUA_ERRRUN))
			return lua_error(L);
	}
	return r;
}

/* setmetatable refuses __gc, so no script code runs at collection time. */
static int glue_setmetatable(lua_State *L) {
	if (lua_type(L, 2) == LUA_TTABLE) {
		lua_pushliteral(L, "__gc");
		if (lua_rawget(L, 2) != LUA_TNIL)
			return luaL_error(L, "setmetatable: __gc metamethods are not allowed");
		lua_pop(L, 1);
	}
	return lua_tocfunction(L, lua_upvalueindex(1))(L);
}

/* collectgarbage only reports memory use. Other options could defeat the
 * limits or cost a full collection per call. */
static int glue_collectgarbage(lua_State *L) {
	const char *opt = luaL_optstring(L, 1, "collect");
	if (strcmp(opt, "count") != 0)
		return luaL_error(L, "collectgarbage: only \"count\" is allowed");
	lua_pushnumber(L, (lua_Number)lua_gc(L, LUA_GCCOUNT, 0) + (lua_Number)lua_gc(L, LUA_GCCOUNTB, 0) / 1024);
	return 1;
}

static int64_t glue_log2(int64_t n) {
	int64_t r = 0;
	while (n > 1) {
		n >>= 1;
		r++;
	}
	return r;
}

/*
 * glue_charged runs a library function after charging the CPU budget for
 * work that runs in C, which the count hook does not see.
 * Upvalue 1 is the function and upvalue 2 its cost class.
 */
static int glue_charged(lua_State *L) {
	int cls = (int)lua_tointeger(L, lua_upvalueindex(2));
	lua_CFunction f = lua_tocfunction(L, lua_upvalueindex(1));
	int i, r, n = lua_gettop(L);
	int64_t cost = 1;
	if (cls & GLUE_CSTR) {
		for (i = 1; i <= n; i++)
			if (lua_type(L, i) == LUA_TSTRING)
				cost += (int64_t)(lua_rawlen(L, i) / 2);
	}
	if ((cls & (GLUE_CTAB | GLUE_CSORT)) && lua_type(L, 1) == LUA_TTABLE) {
		int64_t len = (int64_t)lua_rawlen(L, 1);
		cost += len;
		if (cls & GLUE_CSORT)
			cost += len * glue_log2(len);
	}
	if ((cls & GLUE_CMOVE) && lua_isinteger(L, 2) && lua_isinteger(L, 3)) {
		lua_Integer f0 = lua_tointeger(L, 2), e = lua_tointeger(L, 3);
		if (e >= f0)
			cost += (e - f0 < INT64_MAX / 2) ? (int64_t)(e - f0) : INT64_MAX / 2;
	}
	glue_charge(L, cost);
	r = f(L);
	if (cls & GLUE_CRES)
		glue_charge(L, r);
	return r;
}

static void glue_wrap(lua_State *L, const char *lib, const char *name, lua_CFunction w, int cls) {
	if (lib != NULL)
		lua_getglobal(L, lib);
	else
		lua_pushglobaltable(L);
	lua_getfield(L, -1, name);
	if (cls >= 0) {
		lua_pushinteger(L, cls);
		lua_pushcclosure(L, glue_charged, 2);
	} else
		lua_pushcclosure(L, w, 1);
	lua_setfield(L, -2, name);
	lua_pop(L, 1);
}

static void glue_remove(lua_State *L, const char *lib, const char *name) {
	if (lib != NULL)
		lua_getglobal(L, lib);
	else
		lua_pushglobaltable(L);
	lua_pushnil(L);
	lua_setfield(L, -2, name);
	lua_pop(L, 1);
}

/*
 * require loads modules from sources that Go gives, never from files.
 * Upvalue 1 is the table of loaded modules. A module that requires itself
 * while it loads is an error.
 */
static int glue_require(lua_State *L) {
	glue_state *st = glue_of(L);
	const char *name = luaL_checkstring(L, 1);
	size_t len;
	const char *src;
	int32_t r;
	int status;
	lua_settop(L, 1);
	lua_pushvalue(L, 1);
	lua_rawget(L, lua_upvalueindex(1));
	if (lua_touserdata(L, -1) == (void *)&glue_loading)
		return luaL_error(L, "require: module '%s' requires itself while it loads", name);
	if (!lua_isnil(L, -1))
		return 1;
	lua_pop(L, 1);
	r = glue_call_host(st, L, GLUE_REQUIRE_ID, 1, 1);
	if (r != GLUE_HRETURN || st->h.nres != 1)
		return glue_host_result(st, L, r == GLUE_HRETURN ? GLUE_HERROR : r);
	glue_host_result(st, L, r); /* pushes the source, or nil */
	if (lua_type(L, -1) != LUA_TSTRING)
		return luaL_error(L, "require: module '%s' not found", name);
	src = lua_tolstring(L, -1, &len);
	lua_pushvalue(L, 1);
	lua_pushlightuserdata(L, (void *)&glue_loading);
	lua_rawset(L, lua_upvalueindex(1));
	lua_pushfstring(L, "=%s", name);
	status = luaL_loadbufferx(L, src, len, lua_tostring(L, -1), "t");
	if (status == LUA_OK) {
		lua_pushvalue(L, 1);
		status = lua_pcall(L, 1, 1, 0);
	}
	if (status != LUA_OK) {
		glue_fatal(st, status);
		lua_pushvalue(L, 1);
		lua_pushnil(L);
		lua_rawset(L, lua_upvalueindex(1));
		return lua_error(L);
	}
	if (lua_isnil(L, -1)) {
		lua_pop(L, 1);
		lua_pushboolean(L, 1);
	}
	lua_pushvalue(L, 1);
	lua_pushvalue(L, -2);
	lua_rawset(L, lua_upvalueindex(1));
	return 1;
}

static const luaL_Reg glue_libs[] = {
	{LUA_GNAME, luaopen_base},
	{LUA_COLIBNAME, luaopen_coroutine},
	{LUA_TABLIBNAME, luaopen_table},
	{LUA_STRLIBNAME, luaopen_string},
	{LUA_MATHLIBNAME, luaopen_math},
	{LUA_UTF8LIBNAME, luaopen_utf8},
	{NULL, NULL},
};

/* glue_setup opens the libraries and applies the sandbox. */
static int glue_setup(lua_State *L) {
	glue_state *st = glue_of(L);
	const luaL_Reg *lib;
	for (lib = glue_libs; lib->func; lib++) {
		luaL_requiref(L, lib->name, lib->func, 1);
		lua_pop(L, 1);
	}

	/* Files, output, and bytecode. */
	glue_remove(L, NULL, "dofile");
	glue_remove(L, NULL, "loadfile");
	glue_remove(L, NULL, "print");
	glue_remove(L, NULL, "warn");
	glue_remove(L, LUA_STRLIBNAME, "dump");
	glue_wrap(L, NULL, "load", glue_lload, -1);

	/* Scripts use the host's random numbers, so seeded tests repeat. */
	glue_remove(L, LUA_MATHLIBNAME, "random");
	glue_remove(L, LUA_MATHLIBNAME, "randomseed");

	/* Limits that scripts cannot catch or defeat. */
	lua_pushcfunction(L, glue_pcall);
	lua_setglobal(L, "pcall");
	lua_pushcfunction(L, glue_xpcall);
	lua_setglobal(L, "xpcall");
	lua_pushcfunction(L, glue_collectgarbage);
	lua_setglobal(L, "collectgarbage");
	glue_wrap(L, NULL, "setmetatable", glue_setmetatable, -1);
	glue_wrap(L, LUA_COLIBNAME, "resume", glue_coresume, -1);
	glue_wrap(L, LUA_COLIBNAME, "close", glue_coclose, -1);

	/* Library functions whose work in C is charged to the CPU budget. */
	glue_wrap(L, NULL, "tonumber", NULL, GLUE_CSTR);
	glue_wrap(L, NULL, "rawequal", NULL, GLUE_CSTR);
	glue_wrap(L, LUA_STRLIBNAME, "byte", NULL, GLUE_CRES);
	glue_wrap(L, LUA_STRLIBNAME, "unpack", NULL, GLUE_CRES);
	glue_wrap(L, LUA_TABLIBNAME, "concat", NULL, GLUE_CTAB);
	glue_wrap(L, LUA_TABLIBNAME, "insert", NULL, GLUE_CTAB);
	glue_wrap(L, LUA_TABLIBNAME, "remove", NULL, GLUE_CTAB);
	glue_wrap(L, LUA_TABLIBNAME, "move", NULL, GLUE_CMOVE);
	glue_wrap(L, LUA_TABLIBNAME, "sort", NULL, GLUE_CSORT);
	glue_wrap(L, LUA_TABLIBNAME, "unpack", NULL, GLUE_CRES);
	glue_wrap(L, LUA_UTF8LIBNAME, "len", NULL, GLUE_CSTR);
	glue_wrap(L, LUA_UTF8LIBNAME, "offset", NULL, GLUE_CSTR);
	glue_wrap(L, LUA_UTF8LIBNAME, "codepoint", NULL, GLUE_CSTR | GLUE_CRES);

	/* Scripts cannot reach the string metatable. */
	lua_pushliteral(L, "");
	lua_getmetatable(L, -1);
	lua_pushboolean(L, 0);
	lua_setfield(L, -2, "__metatable");
	lua_pop(L, 2);

	/* require, with its table of loaded modules. */
	lua_newtable(L);
	lua_pushcclosure(L, glue_require, 1);
	lua_setglobal(L, "require");

	lua_newtable(L);
	st->tmp_ref = luaL_ref(L, LUA_REGISTRYINDEX);
	return 0;
}

/* ---- Operations ---- */

EXPORT(glue_new) glue_hdr *glue_new(int64_t handle, int64_t cpu, int64_t mem, int64_t timeout) {
	glue_state *st = calloc(1, sizeof *st);
	lua_State *L;
	if (st == NULL)
		return NULL;
	st->handle = handle;
	st->cpu_limit = cpu > 0 ? cpu : 0;
	st->mem_limit = mem > 0 ? (size_t)mem : 0;
	st->timeout = timeout > 0 ? timeout : 0;
	if (!glue_reserve(st, 64, 1024)) {
		free((void *)(intptr_t)st->h.vals);
		free(st);
		return NULL;
	}
	L = lua_newstate(glue_alloc, st);
	if (L == NULL) {
		free((void *)(intptr_t)st->h.vals);
		free((void *)(intptr_t)st->h.buf);
		free(st);
		return NULL;
	}
	*(glue_state **)lua_getextraspace(L) = st;
	st->L = L;
	lua_pushcfunction(L, glue_setup);
	if (lua_pcall(L, 0, 0, 0) != LUA_OK) {
		glue_close(&st->h);
		return NULL;
	}
	/* New threads copy the hook from the thread that creates them. */
	if (st->cpu_limit > 0 || st->timeout > 0)
		lua_sethook(L, glue_hook, LUA_MASKCOUNT, GLUE_HOOK_EVERY);
	return &st->h;
}

EXPORT(glue_close) void glue_close(glue_hdr *h) {
	glue_state *st = (glue_state *)h;
	lua_close(st->L);
	free((void *)(intptr_t)st->h.vals);
	free((void *)(intptr_t)st->h.buf);
	free(st);
}

/* glue_function pushes value 0 as a function, looking up a global when it
 * is a string. */
static void glue_function(glue_state *st, lua_State *L, int64_t *pos) {
	glue_get(st, L, pos, 0);
	if (lua_type(L, -1) == LUA_TSTRING) {
		lua_pushglobaltable(L);
		lua_insert(L, -2);
		lua_rawget(L, -2);
		lua_remove(L, -2);
	}
}

static int32_t glue_call(glue_state *st, int nargs) {
	lua_State *L = st->L;
	int64_t pos = 0;
	int i, status;
	lua_settop(L, 0);
	if (nargs < 0 || !lua_checkstack(L, nargs + LUA_MINSTACK)) {
		glue_put_msg(st, "too many arguments");
		return GLUE_EARG;
	}
	glue_function(st, L, &pos);
	for (i = 0; i < nargs; i++)
		glue_get(st, L, &pos, 0);
	glue_tmp_clear(st);
	glue_begin(st);
	status = lua_pcall(L, nargs, LUA_MULTRET, 0);
	glue_end(st);
	if (status != LUA_OK) {
		glue_put_error(st, L);
		return status;
	}
	return glue_put_results(st, L, lua_gettop(L));
}

static int32_t glue_resume(glue_state *st, int ref, int nargs) {
	lua_State *L = st->L, *T, *prev;
	int64_t pos = 0;
	int i, status, nres = 0;
	lua_settop(L, 0);
	lua_rawgeti(L, LUA_REGISTRYINDEX, ref);
	T = lua_tothread(L, -1);
	if (T == NULL || T == L) {
		glue_put_msg(st, "not a thread");
		return GLUE_EARG;
	}
	status = lua_status(T);
	if ((status != LUA_OK && status != LUA_YIELD) || (status == LUA_OK && lua_gettop(T) == 0)) {
		glue_put_msg(st, "cannot resume a thread that has finished");
		return GLUE_EARG;
	}
	if (nargs < 0 || !lua_checkstack(T, nargs + LUA_MINSTACK)) {
		glue_put_msg(st, "too many arguments");
		return GLUE_EARG;
	}
	for (i = 0; i < nargs; i++)
		glue_get(st, T, &pos, 0);
	glue_tmp_clear(st);
	prev = st->resumed;
	st->resumed = T;
	glue_begin(st);
	status = lua_resume(T, L, nargs, &nres);
	glue_end(st);
	st->resumed = prev;
	if (status == LUA_OK || status == LUA_YIELD) {
		/* The values move to the main thread, which keeps them alive until
		 * the next operation. */
		if (!lua_checkstack(L, nres + LUA_MINSTACK)) {
			lua_settop(T, 0);
			glue_put_msg(st, "too many results");
			return GLUE_EENCODE;
		}
		lua_xmove(T, L, nres);
		i = glue_put_results(st, L, nres);
		return i == LUA_OK ? status : i;
	}
	lua_xmove(T, L, 1);
	lua_closethread(T, L);
	glue_put_error(st, L);
	return status;
}

static int32_t glue_load(glue_state *st) {
	lua_State *L = st->L;
	int64_t pos = 0;
	glue_value src;
	int64_t off;
	int status;
	lua_settop(L, 0);
	glue_tmp_clear(st);
	if (st->h.nin < 2) {
		glue_put_msg(st, "no source");
		return GLUE_EARG;
	}
	src = glue_vals(st)[1];
	off = src.i - st->h.buf;
	if (src.type != GLUE_TSTRING || src.len < 0 || off < 0 || off + src.len > st->h.bcap) {
		glue_put_msg(st, "no source");
		return GLUE_EARG;
	}
	lua_pushliteral(L, "=");
	glue_get(st, L, &pos, 0);
	if (lua_type(L, -1) != LUA_TSTRING) {
		glue_put_msg(st, "no chunk name");
		return GLUE_EARG;
	}
	lua_concat(L, 2);
	glue_begin(st);
	status = luaL_loadbufferx(L, (const char *)(intptr_t)src.i, (size_t)src.len, lua_tostring(L, 1), "t");
	if (status == LUA_OK)
		status = lua_pcall(L, 0, LUA_MULTRET, 0);
	glue_end(st);
	if (status != LUA_OK) {
		glue_put_error(st, L);
		return status;
	}
	return glue_put_results(st, L, lua_gettop(L) - 1);
}

/* glue_target is the thread for operations that Go runs during a host call. */
static lua_State *glue_target(glue_state *st) {
	return st->cur != NULL ? st->cur : st->L;
}

EXPORT(glue_op) int32_t glue_op(glue_hdr *h, int32_t op, int64_t a, int64_t b) {
	glue_state *st = (glue_state *)h;
	lua_State *L = glue_target(st);
	int64_t pos = 0;
	int ref;
	switch (op) {
	case GLUE_OCALL:
		return glue_call(st, (int)a);
	case GLUE_ORESUME:
		return glue_resume(st, (int)a, (int)b);
	case GLUE_OLOAD:
		return glue_load(st);
	case GLUE_ONEWTHREAD: {
		lua_State *T;
		L = st->L;
		lua_settop(L, 0);
		glue_function(st, L, &pos);
		if (lua_type(L, -1) != LUA_TFUNCTION) {
			lua_settop(L, 0);
			return -1;
		}
		T = lua_newthread(L);
		lua_insert(L, -2);
		lua_xmove(L, T, 1);
		ref = luaL_ref(L, LUA_REGISTRYINDEX);
		lua_settop(L, 0);
		return ref;
	}
	case GLUE_OKEEP:
		if (!lua_checkstack(L, 2))
			return -1;
		glue_get(st, L, &pos, 0);
		if (lua_isnil(L, -1)) {
			lua_pop(L, 1);
			return -1;
		}
		return luaL_ref(L, LUA_REGISTRYINDEX);
	case GLUE_OUNREF:
		if (a > LUA_RIDX_LAST && a <= INT32_MAX && a != st->tmp_ref)
			luaL_unref(L, LUA_REGISTRYINDEX, (int)a);
		return 0;
	case GLUE_OCLOSETHREAD: {
		lua_State *T;
		if (a <= LUA_RIDX_LAST || a > INT32_MAX || a == st->tmp_ref || !lua_checkstack(L, 2))
			return -1;
		lua_rawgeti(L, LUA_REGISTRYINDEX, (int)a);
		T = lua_tothread(L, -1);
		lua_pop(L, 1);
		if (T != NULL && T != st->resumed && T != L) {
			/* Pending to-be-closed variables run with the call's limits. */
			int was = st->limited;
			if (!was)
				glue_begin(st);
			lua_closethread(T, L);
			if (!was)
				glue_end(st);
		}
		luaL_unref(L, LUA_REGISTRYINDEX, (int)a);
		return 0;
	}
	case GLUE_OSETGLOBAL:
		if (!lua_checkstack(L, 3))
			return -1;
		lua_pushglobaltable(L);
		glue_get(st, L, &pos, 0);
		glue_get(st, L, &pos, 0);
		if (lua_type(L, -2) != LUA_TSTRING) {
			lua_pop(L, 3);
			return -1;
		}
		lua_rawset(L, -3);
		lua_pop(L, 1);
		return 0;
	case GLUE_OGETGLOBAL: {
		int r;
		L = st->L;
		lua_settop(L, 0);
		glue_get(st, L, &pos, 0);
		if (lua_type(L, -1) != LUA_TSTRING) {
			lua_settop(L, 0);
			glue_put_msg(st, "not a name");
			return GLUE_EARG;
		}
		glue_tmp_clear(st);
		lua_pushglobaltable(L);
		lua_insert(L, -2);
		lua_rawget(L, -2);
		r = glue_put_results(st, L, 1);
		return r;
	}
	case GLUE_OHOSTFN:
		if (!lua_checkstack(L, 2))
			return -1;
		lua_pushinteger(L, (lua_Integer)a);
		lua_pushcclosure(L, glue_hostfn, 1);
		return luaL_ref(L, LUA_REGISTRYINDEX);
	case GLUE_ORESERVE:
		return glue_reserve(st, a, b) ? 0 : -1;
	case GLUE_OGC:
		lua_gc(st->L, LUA_GCCOLLECT);
		return 0;
	}
	return -1;
}
