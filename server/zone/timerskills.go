package zone

import sc "github.com/lxsmnsyc/wathena/server/statuschange"

// optionHidden reports whether a unit has OPTION_HIDE.
func optionHidden(e *entity) bool { return !e.sc.Empty() && e.sc.Option&sc.OptionHide != 0 }
