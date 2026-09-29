/*
 * Binding between Go and C Lua 5.4, shared by the WebAssembly build and the
 * cgo build. Each state has its own CPU budget and memory limit.
 *
 * Host functions are imported from Go. They take plain integers and strings,
 * so each call from Lua into Go crosses the boundary once.
 */
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

#include "lua.h"
#include "lauxlib.h"
#include "lualib.h"

#ifdef __wasm__
#define HOST(name) __attribute__((import_module("env"), import_name(#name)))
#define EXPORT(name) __attribute__((export_name(#name)))
#else
#define HOST(name)
#define EXPORT(name)
#endif

HOST(host_add) void host_add(int64_t h, int64_t k, int64_t v);
HOST(host_get) int64_t host_get(int64_t h, char *s, int32_t n);
HOST(host_set) void host_set(int64_t h, char *s, int32_t n, int64_t v);
HOST(host_chance) int32_t host_chance(int64_t h, int64_t n, int64_t d);

/* The CPU hook runs every HOOK_EVERY instructions and charges the budget. */
#define HOOK_EVERY 1000

typedef struct {
	int64_t host;
	int64_t cpu_limit; /* instructions per call, 0 for none */
	int64_t budget;
	size_t mem;
	size_t mem_limit; /* bytes, 0 for none */
} glue_state;

static glue_state *state_of(lua_State *L) {
	return *(glue_state **)lua_getextraspace(L);
}

static void *glue_allocf(void *ud, void *ptr, size_t osize, size_t nsize) {
	glue_state *st = ud;
	if (ptr == NULL)
		osize = 0;
	if (nsize == 0) {
		st->mem -= osize;
		free(ptr);
		return NULL;
	}
	if (st->mem_limit && nsize > osize && st->mem + (nsize - osize) > st->mem_limit)
		return NULL;
	void *p = realloc(ptr, nsize);
	if (p != NULL)
		st->mem = st->mem - osize + nsize;
	return p;
}

static void glue_hook(lua_State *L, lua_Debug *ar) {
	(void)ar;
	glue_state *st = state_of(L);
	st->budget -= HOOK_EVERY;
	if (st->budget <= 0)
		luaL_error(L, "CPU limit exceeded");
}

static int l_wa_add(lua_State *L) {
	host_add(state_of(L)->host, luaL_checkinteger(L, 1), luaL_checkinteger(L, 2));
	return 0;
}

static int l_pc_get(lua_State *L) {
	size_t n;
	const char *s = luaL_checklstring(L, 1, &n);
	lua_pushinteger(L, host_get(state_of(L)->host, (char *)s, (int32_t)n));
	return 1;
}

static int l_pc_set(lua_State *L) {
	size_t n;
	const char *s = luaL_checklstring(L, 1, &n);
	host_set(state_of(L)->host, (char *)s, (int32_t)n, luaL_checkinteger(L, 2));
	return 0;
}

static int l_wa_chance(lua_State *L) {
	lua_pushboolean(L, host_chance(state_of(L)->host, luaL_checkinteger(L, 1), luaL_checkinteger(L, 2)));
	return 1;
}

static const luaL_Reg libs[] = {
	{LUA_GNAME, luaopen_base},
	{LUA_COLIBNAME, luaopen_coroutine},
	{LUA_TABLIBNAME, luaopen_table},
	{LUA_STRLIBNAME, luaopen_string},
	{LUA_MATHLIBNAME, luaopen_math},
	{LUA_UTF8LIBNAME, luaopen_utf8},
	{NULL, NULL},
};

/* glue_new creates a state. It returns NULL when out of memory. */
EXPORT(glue_new) lua_State *glue_new(int64_t host, int64_t cpu_limit, int64_t mem_limit) {
	glue_state *st = calloc(1, sizeof *st);
	if (st == NULL)
		return NULL;
	st->host = host;
	st->cpu_limit = cpu_limit;
	st->mem_limit = (size_t)mem_limit;
	lua_State *L = lua_newstate(glue_allocf, st);
	if (L == NULL) {
		free(st);
		return NULL;
	}
	*(glue_state **)lua_getextraspace(L) = st;
	/* Loading the libraries and host functions is not limited. */
	size_t limit = st->mem_limit;
	st->mem_limit = 0;
	for (const luaL_Reg *lib = libs; lib->func; lib++) {
		luaL_requiref(L, lib->name, lib->func, 1);
		lua_pop(L, 1);
	}
	lua_register(L, "wa_add", l_wa_add);
	lua_register(L, "pc_get", l_pc_get);
	lua_register(L, "pc_set", l_pc_set);
	lua_register(L, "wa_chance", l_wa_chance);
	st->mem_limit = limit;
	/* New coroutines copy the hook from the thread that creates them. */
	if (cpu_limit > 0)
		lua_sethook(L, glue_hook, LUA_MASKCOUNT, HOOK_EVERY);
	return L;
}

EXPORT(glue_close) void glue_close(lua_State *L) {
	glue_state *st = state_of(L);
	lua_close(L);
	free(st);
}

/* glue_load runs a chunk. On error, glue_error returns the message. */
EXPORT(glue_load) int32_t glue_load(lua_State *L, char *src, int32_t len, char *name) {
	lua_settop(L, 0);
	glue_state *st = state_of(L);
	st->budget = st->cpu_limit;
	int status = luaL_loadbufferx(L, src, (size_t)len, name, "t");
	if (status == LUA_OK)
		status = lua_pcall(L, 0, 0, 0);
	return status;
}

/* glue_call calls a global function with one integer. */
EXPORT(glue_call) int32_t glue_call(lua_State *L, char *fn, int64_t arg, int64_t *out) {
	lua_settop(L, 0);
	glue_state *st = state_of(L);
	st->budget = st->cpu_limit;
	lua_getglobal(L, fn);
	lua_pushinteger(L, arg);
	int status = lua_pcall(L, 1, 1, 0);
	if (status == LUA_OK)
		*out = lua_tointeger(L, -1);
	return status;
}

EXPORT(glue_error) char *glue_error(lua_State *L, int32_t *len) {
	size_t n = 0;
	const char *s = lua_tolstring(L, -1, &n);
	*len = (int32_t)n;
	return (char *)s;
}

/* glue_memory returns the bytes the state has allocated. */
EXPORT(glue_memory) int64_t glue_memory(lua_State *L) {
	return (int64_t)state_of(L)->mem;
}

#ifdef __wasm__
EXPORT(glue_malloc) void *glue_malloc(int32_t n) { return malloc((size_t)n); }
EXPORT(glue_free) void glue_free(void *p) { free(p); }
#endif
