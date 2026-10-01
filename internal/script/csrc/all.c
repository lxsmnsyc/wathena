/*
 * Lua 5.4 and the binding in one translation unit, in the style of Lua's
 * own onelua.c. The cgo build includes this file from ../lua_cgo.c, and
 * ../wasm/build.sh compiles it to lua.wasm.
 *
 * The include path must contain ../lua/src.
 *
 * Only the libraries scripts may use are compiled: base, coroutine, math,
 * string, table, and utf8. The interpreter, the compiler, and the debug,
 * io, os, and package libraries are left out.
 */
/*
 * On POSIX hosts Lua uses _setjmp and _longjmp, which do not save the
 * signal mask. Plain setjmp makes a system call on macOS for every
 * protected call. Windows and WebAssembly keep the ISO C setjmp.
 */
#if !defined(_WIN32) && !defined(__wasm__)
#define LUA_USE_POSIX
#endif

#include "lprefix.h"

#include <assert.h>
#include <ctype.h>
#include <errno.h>
#include <float.h>
#include <limits.h>
#include <locale.h>
#include <math.h>
#include <setjmp.h>
#include <stdarg.h>
#include <stddef.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

/* Setup for luaconf.h, as in onelua.c. */
#define LUA_CORE
#define LUA_LIB
#define ltable_c
#define lvm_c
#include "luaconf.h"

/* Internal symbols stay inside this file. */
#undef LUAI_FUNC
#undef LUAI_DDEC
#undef LUAI_DDEF
#define LUAI_FUNC static
#define LUAI_DDEC(def) /* empty */
#define LUAI_DDEF static

/* The core. */
#include "lzio.c"
#include "lctype.c"
#include "lopcodes.c"
#include "lmem.c"
#include "lundump.c"
#include "ldump.c"
#include "lstate.c"
#include "lgc.c"
#include "llex.c"
#include "lcode.c"
#include "lparser.c"
#include "ldebug.c"
#include "lfunc.c"
#include "lobject.c"
#include "ltm.c"
#include "lstring.c"
#include "ltable.c"
#include "ldo.c"
#include "lvm.c"
#include "lapi.c"

/* The auxiliary library and the libraries scripts may use. */
#include "lauxlib.c"
#include "lbaselib.c"
#include "lcorolib.c"
#include "lmathlib.c"
#include "ltablib.c"
#include "lutf8lib.c"

/* The string library, with pattern matching charged to the CPU budget. */
#include "glue.h"
#include "lstrlib_charged.c"

#include "glue.c"
