// Package convert turns rAthena scripts into wAthena Lua and data
// (strategy section 5.6). It reads item scripts today. The grammar
// follows the parser in rAthena's src/map/script.cpp.
package convert

// Stmt is a statement of an rAthena script.
type Stmt interface{ stmt() }

// Expr is an expression of an rAthena script.
type Expr interface{ expr() }

// Block is { statements }.
type Block struct{ Stmts []Stmt }

// If is if (Cond) Then else Else. Else may be nil.
type If struct {
	Cond       Expr
	Then, Else Stmt
}

// Switch is switch (Value) { case ...: ... default: ... }.
type Switch struct {
	Value Expr
	Cases []*Case
}

// Case is one case label and the statements after it. Default has no
// values. Statements fall through to the next case without a break.
type Case struct {
	Values  []Expr
	Default bool
	Body    []Stmt
}

// For is for (Init; Cond; Post) Body. Any part may be nil.
type For struct {
	Init, Post Stmt
	Cond       Expr
	Body       Stmt
}

// While is while (Cond) Body, or do Body while (Cond) when Do is set.
type While struct {
	Cond Expr
	Body Stmt
	Do   bool
}

// Command is a command statement, such as bonus bStr,1; or getitem(501,1);.
type Command struct {
	Name string
	Args []Expr
}

// Assign is Target Op Value: =, +=, and the other compound operators.
// set .@x, 1; is an Assign too.
type Assign struct {
	Target *Var
	Op     string
	Value  Expr
}

// IncDec is ++x, x++, --x or x-- as a statement or an expression.
type IncDec struct {
	Target *Var
	Op     string // "++" or "--"
	Prefix bool
}

// Jump is break;, continue;, end; or close;.
type Jump struct{ Kind string }

// Return is return; or return Value;.
type Return struct{ Value Expr }

// Unsupported is a statement the converter cannot read, such as a label
// or goto. Text is what it found.
type Unsupported struct{ Text string }

// Num is an integer literal.
type Num struct{ Value int64 }

// Str is a string literal, with escapes resolved.
type Str struct{ Value string }

// Var is a variable, such as .@i or @menu$, with an optional array index.
type Var struct {
	Name  string // with its prefix and $ suffix
	Index Expr
}

// Ident is a bare name: a constant, a parameter such as BaseLevel, or a
// permanent character variable.
type Ident struct{ Name string }

// Call is a function call in an expression, such as getrefine().
type Call struct {
	Name string
	Args []Expr
}

// Unary is -X, !X or ~X.
type Unary struct {
	Op string
	X  Expr
}

// Binary is X Op Y.
type Binary struct {
	Op   string
	X, Y Expr
}

// Ternary is Cond ? Then : Else.
type Ternary struct{ Cond, Then, Else Expr }

func (*Block) stmt()       {}
func (*If) stmt()          {}
func (*Switch) stmt()      {}
func (*For) stmt()         {}
func (*While) stmt()       {}
func (*Command) stmt()     {}
func (*Assign) stmt()      {}
func (*IncDec) stmt()      {}
func (*Jump) stmt()        {}
func (*Return) stmt()      {}
func (*Unsupported) stmt() {}

func (*Num) expr()     {}
func (*Str) expr()     {}
func (*Var) expr()     {}
func (*Ident) expr()   {}
func (*Call) expr()    {}
func (*Unary) expr()   {}
func (*Binary) expr()  {}
func (*Ternary) expr() {}
func (*IncDec) expr()  {}
func (*Assign) expr()  {}
