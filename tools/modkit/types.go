package modkit

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/lxsmnsyc/wathena/server/constants"
	"github.com/lxsmnsyc/wathena/server/modscript/luaapi"
)

// Editor support (strategy section 6.9): the Lua Language Server reads
// the definitions of server/modscript/luaapi from TypesDir, which
// .luarc.json names. Names that start with "." are not part of a mod, so
// the server and wactl mod pack skip them.
const (
	TypesDir  = ".wathena/types"
	LuaRCFile = ".luarc.json"
)

// LuaRC is the Lua Language Server settings of a new mod.
const LuaRC = `{
  "$schema": "https://raw.githubusercontent.com/LuaLS/vscode-lua/master/setting/schema.json",
  "runtime.version": "Lua 5.4",
  "runtime.builtin": {
    "debug": "disable",
    "io": "disable",
    "os": "disable"
  },
  "workspace.library": [".wathena/types"],
  "workspace.checkThirdParty": false
}
`

// WriteTypes writes the editor definitions into a mod's TypesDir, in
// place of the ones there, and .luarc.json when the mod has none. It
// returns how many definition files it wrote.
func WriteTypes(dir string, consts *constants.Data) (int, error) {
	types := filepath.Join(dir, filepath.FromSlash(TypesDir))
	if err := os.RemoveAll(types); err != nil {
		return 0, err
	}
	files := luaapi.LuaLS(consts)
	for p, s := range files {
		if err := writeFile(filepath.Join(types, filepath.FromSlash(p)), []byte(s)); err != nil {
			return 0, err
		}
	}
	rc := filepath.Join(dir, LuaRCFile)
	if _, err := os.Stat(rc); errors.Is(err, fs.ErrNotExist) {
		if err := writeFile(rc, []byte(LuaRC)); err != nil {
			return 0, err
		}
	}
	return len(files), nil
}
