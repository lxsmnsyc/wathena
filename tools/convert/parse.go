package convert

import (
	"fmt"
	"strconv"
	"strings"
)

// The lexer and parser follow rAthena's parse_line, parse_syntax and
// parse_subexpr in src/map/script.cpp:
//   - Names of commands, functions and constants are matched without
//     case, as rAthena's str_data does.
//   - Variables carry their prefix (.@, ., @, $, $@, #, ##, ') and their $
//     suffix in the name.
//   - Operator precedence is C's, which rAthena's parse_subexpr follows.

type tokKind int

const (
	tokEOF tokKind = iota
	tokNum
	tokStr
	tokName // a name or a variable
	tokOp
)

type token struct {
	kind tokKind
	text string
	num  int64
	pos  int
}

// lex splits a script into tokens. Comments are skipped.
func lex(src string) ([]token, error) {
	var out []token
	i := 0
	for i < len(src) {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case strings.HasPrefix(src[i:], "//"):
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case strings.HasPrefix(src[i:], "/*"):
			end := strings.Index(src[i+2:], "*/")
			if end < 0 {
				return nil, fmt.Errorf("unclosed comment at %d", i)
			}
			i += end + 4
		case c >= '0' && c <= '9':
			j := i
			if strings.HasPrefix(src[i:], "0x") || strings.HasPrefix(src[i:], "0X") {
				j += 2
				for j < len(src) && isHex(src[j]) {
					j++
				}
			} else {
				for j < len(src) && src[j] >= '0' && src[j] <= '9' {
					j++
				}
			}
			n, err := strconv.ParseInt(src[i:j], 0, 64)
			if err != nil {
				return nil, fmt.Errorf("bad number %q", src[i:j])
			}
			out = append(out, token{kind: tokNum, num: n, text: src[i:j], pos: i})
			i = j
		case c == '"':
			var b strings.Builder
			j := i + 1
			for ; j < len(src) && src[j] != '"'; j++ {
				if src[j] == '\\' && j+1 < len(src) {
					j++
					switch src[j] {
					case 'n':
						b.WriteByte('\n')
					case 't':
						b.WriteByte('\t')
					default:
						b.WriteByte(src[j])
					}
					continue
				}
				b.WriteByte(src[j])
			}
			if j >= len(src) {
				return nil, fmt.Errorf("unclosed string at %d", i)
			}
			out = append(out, token{kind: tokStr, text: b.String(), pos: i})
			i = j + 1
		case isNameStart(c) || c == '.' && i+1 < len(src) && (src[i+1] == '@' || isNameStart(src[i+1])) ||
			c == '@' || c == '$' || c == '#' || c == '\'':
			j := i
			// Prefixes: .@ . @ $@ $ ## # '
			for j < len(src) && strings.IndexByte(".@$#'", src[j]) >= 0 && j-i < 2 {
				j++
			}
			for j < len(src) && (isNameStart(src[j]) || src[j] >= '0' && src[j] <= '9') {
				j++
			}
			if j < len(src) && src[j] == '$' {
				j++
			}
			out = append(out, token{kind: tokName, text: src[i:j], pos: i})
			i = j
		default:
			op := ""
			for _, o := range ops {
				if strings.HasPrefix(src[i:], o) {
					op = o
					break
				}
			}
			if op == "" {
				return nil, fmt.Errorf("unexpected %q at %d", c, i)
			}
			out = append(out, token{kind: tokOp, text: op, pos: i})
			i += len(op)
		}
	}
	return append(out, token{kind: tokEOF, pos: len(src)}), nil
}

// ops are the operators, longest first.
var ops = []string{
	"<<=", ">>=", "&&", "||", "==", "!=", "<=", ">=", "<<", ">>", "++", "--",
	"+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "::",
	"+", "-", "*", "/", "%", "<", ">", "&", "|", "^", "!", "~", "?", ":", "=",
	"(", ")", "{", "}", "[", "]", ",", ";",
}

func isHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

func isNameStart(c byte) bool { return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

// isVariable reports whether a name is a variable by its prefix.
func isVariable(name string) bool { return strings.IndexByte(".@$#'", name[0]) >= 0 }

type parser struct {
	toks []token
	i    int
}

// Parse reads a whole script.
func Parse(src string) (*Block, error) {
	toks, err := lex(src)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks}
	b := &Block{}
	for p.peek().kind != tokEOF {
		s, err := p.stmt()
		if err != nil {
			return nil, err
		}
		if s != nil {
			b.Stmts = append(b.Stmts, s)
		}
	}
	return b, nil
}

func (p *parser) peek() token { return p.toks[p.i] }
func (p *parser) peekAt(n int) token {
	if p.i+n < len(p.toks) {
		return p.toks[p.i+n]
	}
	return p.toks[len(p.toks)-1]
}
func (p *parser) next() token {
	t := p.toks[p.i]
	if t.kind != tokEOF {
		p.i++
	}
	return t
}

func (p *parser) isOp(s string) bool { t := p.peek(); return t.kind == tokOp && t.text == s }

func (p *parser) isWord(s string) bool {
	t := p.peek()
	return t.kind == tokName && strings.EqualFold(t.text, s)
}

func (p *parser) expect(s string) error {
	if !p.isOp(s) {
		return p.errorf("want %q", s)
	}
	p.next()
	return nil
}

func (p *parser) errorf(format string, args ...any) error {
	t := p.peek()
	got := t.text
	if t.kind == tokEOF {
		got = "the end"
	}
	return fmt.Errorf("at %d near %q: %s", t.pos, got, fmt.Sprintf(format, args...))
}

var assignOps = map[string]bool{"=": true, "+=": true, "-=": true, "*=": true, "/=": true, "%=": true, "<<=": true, ">>=": true, "&=": true, "|=": true, "^=": true}

func (p *parser) stmt() (Stmt, error) {
	t := p.peek()
	switch {
	case t.kind == tokOp && t.text == ";":
		p.next()
		return nil, nil
	case t.kind == tokOp && t.text == "{":
		p.next()
		b := &Block{}
		for !p.isOp("}") {
			if p.peek().kind == tokEOF {
				return nil, p.errorf("unclosed block")
			}
			s, err := p.stmt()
			if err != nil {
				return nil, err
			}
			if s != nil {
				b.Stmts = append(b.Stmts, s)
			}
		}
		p.next()
		return b, nil
	case t.kind == tokOp && (t.text == "++" || t.text == "--"):
		e, err := p.expr()
		if err != nil {
			return nil, err
		}
		return p.endStmt(e)
	case t.kind != tokName:
		return nil, p.errorf("want a statement")
	}
	word := strings.ToLower(t.text)
	if isVariable(t.text) {
		e, err := p.expr()
		if err != nil {
			return nil, err
		}
		return p.endStmt(e)
	}
	switch word {
	case "if":
		p.next()
		cond, err := p.parenExpr()
		if err != nil {
			return nil, err
		}
		then, err := p.body()
		if err != nil {
			return nil, err
		}
		s := &If{Cond: cond, Then: then}
		if p.isWord("else") {
			p.next()
			if s.Else, err = p.body(); err != nil {
				return nil, err
			}
		}
		return s, nil
	case "switch":
		return p.switchStmt()
	case "for":
		return p.forStmt()
	case "while":
		p.next()
		cond, err := p.parenExpr()
		if err != nil {
			return nil, err
		}
		body, err := p.body()
		if err != nil {
			return nil, err
		}
		return &While{Cond: cond, Body: body}, nil
	case "do":
		p.next()
		body, err := p.body()
		if err != nil {
			return nil, err
		}
		if !p.isWord("while") {
			return nil, p.errorf("want while after do")
		}
		p.next()
		cond, err := p.parenExpr()
		if err != nil {
			return nil, err
		}
		if p.isOp(";") {
			p.next()
		}
		return &While{Cond: cond, Body: body, Do: true}, nil
	case "break", "continue", "end", "close":
		p.next()
		if p.isOp(";") {
			p.next()
		}
		return &Jump{Kind: word}, nil
	case "return":
		p.next()
		s := &Return{}
		if !p.isOp(";") {
			v, err := p.expr()
			if err != nil {
				return nil, err
			}
			s.Value = v
		}
		if p.isOp(";") {
			p.next()
		}
		return s, nil
	case "set":
		p.next()
		paren := p.isOp("(")
		if paren {
			p.next()
		}
		target, err := p.unary()
		if err != nil {
			return nil, err
		}
		v, ok := target.(*Var)
		if !ok {
			if id, isIdent := target.(*Ident); isIdent {
				v = &Var{Name: id.Name}
			} else {
				return nil, p.errorf("set needs a variable")
			}
		}
		if err := p.expect(","); err != nil {
			return nil, err
		}
		val, err := p.expr()
		if err != nil {
			return nil, err
		}
		if paren {
			if err := p.expect(")"); err != nil {
				return nil, err
			}
		}
		if p.isOp(";") {
			p.next()
		}
		return &Assign{Target: v, Op: "=", Value: val}, nil
	case "function", "goto", "callsub", "menu":
		return p.skipUnsupported()
	}
	// A label.
	if n := p.peekAt(1); n.kind == tokOp && n.text == ":" {
		p.next()
		p.next()
		return &Unsupported{Text: t.text + ":"}, nil
	}
	// An assignment to a permanent variable or parameter, such as
	// Zeny += 100;.
	if n := p.peekAt(1); n.kind == tokOp && (assignOps[n.text] || n.text == "++" || n.text == "--" || n.text == "[") {
		e, err := p.expr()
		if err != nil {
			return nil, err
		}
		return p.endStmt(e)
	}
	return p.command()
}

// endStmt makes a statement from an expression statement.
func (p *parser) endStmt(e Expr) (Stmt, error) {
	if p.isOp(";") {
		p.next()
	} else if !p.isOp("}") && p.peek().kind != tokEOF {
		return nil, p.errorf("want ;")
	}
	switch s := e.(type) {
	case *Assign:
		return s, nil
	case *IncDec:
		return s, nil
	case *Call:
		return &Command{Name: s.Name, Args: s.Args}, nil
	}
	return nil, p.errorf("an expression is not a statement")
}

func (p *parser) skipUnsupported() (Stmt, error) {
	start := p.peek().pos
	depth := 0
	for {
		t := p.next()
		if t.kind == tokEOF {
			break
		}
		if t.kind == tokOp {
			switch t.text {
			case "{":
				depth++
			case "}":
				depth--
			}
			if t.text == ";" && depth == 0 || t.text == "}" && depth == 0 {
				break
			}
		}
	}
	return &Unsupported{Text: fmt.Sprintf("statement at %d", start)}, nil
}

// command reads name args; where args are separated by commas, or are
// a parenthesized list, as rAthena reads both forms.
func (p *parser) command() (Stmt, error) {
	name := p.next().text
	c := &Command{Name: name}
	if p.isOp(";") {
		p.next()
		return c, nil
	}
	if p.isOp("}") || p.peek().kind == tokEOF {
		return c, nil
	}
	if p.isOp("(") {
		save := p.i
		p.next()
		args, err := p.args(")")
		if err == nil && (p.isOp(";") || p.isOp("}") || p.peek().kind == tokEOF) {
			if p.isOp(";") {
				p.next()
			}
			c.Args = args
			return c, nil
		}
		p.i = save
	}
	args, err := p.args(";")
	if err != nil {
		return nil, err
	}
	c.Args = args
	return c, nil
}

// args reads a comma-separated list up to and including end. A list
// that ends at "}" or at the end of the script without end is accepted
// for ";", since rAthena's last statement may lack it.
func (p *parser) args(end string) ([]Expr, error) {
	var out []Expr
	if p.isOp(end) {
		p.next()
		return out, nil
	}
	for {
		e, err := p.expr()
		if err != nil {
			return nil, err
		}
		out = append(out, e)
		if p.isOp(",") {
			p.next()
			continue
		}
		if p.isOp(end) {
			p.next()
			return out, nil
		}
		if end == ";" && (p.isOp("}") || p.peek().kind == tokEOF) {
			return out, nil
		}
		return nil, p.errorf("want , or %s", end)
	}
}

func (p *parser) parenExpr() (Expr, error) {
	if err := p.expect("("); err != nil {
		return nil, err
	}
	e, err := p.expr()
	if err != nil {
		return nil, err
	}
	return e, p.expect(")")
}

// body reads the statement of an if, a loop or an else. An empty body
// is an empty block.
func (p *parser) body() (Stmt, error) {
	s, err := p.stmt()
	if err != nil {
		return nil, err
	}
	if s == nil {
		return &Block{}, nil
	}
	return s, nil
}

func (p *parser) switchStmt() (Stmt, error) {
	p.next()
	v, err := p.parenExpr()
	if err != nil {
		return nil, err
	}
	if err := p.expect("{"); err != nil {
		return nil, err
	}
	s := &Switch{Value: v}
	var cur *Case
	for !p.isOp("}") {
		if p.peek().kind == tokEOF {
			return nil, p.errorf("unclosed switch")
		}
		if p.isWord("case") {
			p.next()
			val, err := p.expr()
			if err != nil {
				return nil, err
			}
			if err := p.expect(":"); err != nil {
				return nil, err
			}
			// Consecutive labels share one body.
			if cur != nil && len(cur.Body) == 0 {
				cur.Values = append(cur.Values, val)
				continue
			}
			cur = &Case{Values: []Expr{val}}
			s.Cases = append(s.Cases, cur)
			continue
		}
		if p.isWord("default") {
			p.next()
			if err := p.expect(":"); err != nil {
				return nil, err
			}
			if cur != nil && len(cur.Body) == 0 {
				cur.Default = true
				continue
			}
			cur = &Case{Default: true}
			s.Cases = append(s.Cases, cur)
			continue
		}
		if cur == nil {
			return nil, p.errorf("statement before the first case")
		}
		st, err := p.stmt()
		if err != nil {
			return nil, err
		}
		if st != nil {
			cur.Body = append(cur.Body, st)
		}
	}
	p.next()
	return s, nil
}

func (p *parser) forStmt() (Stmt, error) {
	p.next()
	if err := p.expect("("); err != nil {
		return nil, err
	}
	f := &For{}
	simple := func(end string) (Stmt, error) {
		if p.isOp(end) {
			p.next()
			return nil, nil
		}
		var s Stmt
		var err error
		if p.isWord("set") {
			// set reads its own ; but not the ) of the loop's step.
			if s, err = p.stmt(); err != nil || end != ")" {
				return s, err
			}
			return s, p.expect(")")
		}
		e, err := p.expr()
		if err != nil {
			return nil, err
		}
		switch e := e.(type) {
		case *Assign:
			s = e
		case *IncDec:
			s = e
		default:
			return nil, p.errorf("want an assignment in for")
		}
		return s, p.expect(end)
	}
	var err error
	if f.Init, err = simple(";"); err != nil {
		return nil, err
	}
	if !p.isOp(";") {
		if f.Cond, err = p.expr(); err != nil {
			return nil, err
		}
	}
	if err := p.expect(";"); err != nil {
		return nil, err
	}
	if f.Post, err = simple(")"); err != nil {
		return nil, err
	}
	if f.Body, err = p.body(); err != nil {
		return nil, err
	}
	return f, nil
}

// Binary operator precedence, as C's.
var precedence = map[string]int{
	"||": 1, "&&": 2, "|": 3, "^": 4, "&": 5,
	"==": 6, "!=": 6,
	"<": 7, "<=": 7, ">": 7, ">=": 7,
	"<<": 8, ">>": 8,
	"+": 9, "-": 9,
	"*": 10, "/": 10, "%": 10,
}

func (p *parser) expr() (Expr, error) {
	lhs, err := p.ternary()
	if err != nil {
		return nil, err
	}
	if t := p.peek(); t.kind == tokOp && assignOps[t.text] {
		target, ok := lhs.(*Var)
		if !ok {
			if id, isIdent := lhs.(*Ident); isIdent {
				target = &Var{Name: id.Name}
			} else {
				return nil, p.errorf("can only assign to a variable")
			}
		}
		p.next()
		v, err := p.expr()
		if err != nil {
			return nil, err
		}
		return &Assign{Target: target, Op: t.text, Value: v}, nil
	}
	return lhs, nil
}

func (p *parser) ternary() (Expr, error) {
	cond, err := p.binary(1)
	if err != nil {
		return nil, err
	}
	if !p.isOp("?") {
		return cond, nil
	}
	p.next()
	a, err := p.ternary()
	if err != nil {
		return nil, err
	}
	if err := p.expect(":"); err != nil {
		return nil, err
	}
	b, err := p.ternary()
	if err != nil {
		return nil, err
	}
	return &Ternary{Cond: cond, Then: a, Else: b}, nil
}

func (p *parser) binary(minPrec int) (Expr, error) {
	lhs, err := p.unary()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		prec, ok := precedence[t.text]
		if t.kind != tokOp || !ok || prec < minPrec {
			return lhs, nil
		}
		p.next()
		rhs, err := p.binary(prec + 1)
		if err != nil {
			return nil, err
		}
		lhs = &Binary{Op: t.text, X: lhs, Y: rhs}
	}
}

func (p *parser) unary() (Expr, error) {
	t := p.peek()
	if t.kind == tokOp {
		switch t.text {
		case "-", "!", "~", "+":
			p.next()
			x, err := p.unary()
			if err != nil {
				return nil, err
			}
			if t.text == "+" {
				return x, nil
			}
			if n, ok := x.(*Num); ok && t.text == "-" {
				return &Num{Value: -n.Value}, nil
			}
			return &Unary{Op: t.text, X: x}, nil
		case "++", "--":
			p.next()
			x, err := p.unary()
			if err != nil {
				return nil, err
			}
			v, ok := x.(*Var)
			if !ok {
				return nil, p.errorf("%s needs a variable", t.text)
			}
			return &IncDec{Target: v, Op: t.text, Prefix: true}, nil
		}
	}
	x, err := p.primary()
	if err != nil {
		return nil, err
	}
	if t := p.peek(); t.kind == tokOp && (t.text == "++" || t.text == "--") {
		switch v := x.(type) {
		case *Var:
			p.next()
			return &IncDec{Target: v, Op: t.text}, nil
		case *Ident:
			p.next()
			return &IncDec{Target: &Var{Name: v.Name}, Op: t.text}, nil
		}
	}
	return x, nil
}

func (p *parser) primary() (Expr, error) {
	t := p.next()
	switch t.kind {
	case tokNum:
		return &Num{Value: t.num}, nil
	case tokStr:
		return &Str{Value: t.text}, nil
	case tokOp:
		if t.text == "(" {
			e, err := p.expr()
			if err != nil {
				return nil, err
			}
			return e, p.expect(")")
		}
	case tokName:
		if isVariable(t.text) {
			v := &Var{Name: t.text}
			if p.isOp("[") {
				p.next()
				idx, err := p.expr()
				if err != nil {
					return nil, err
				}
				if err := p.expect("]"); err != nil {
					return nil, err
				}
				v.Index = idx
			}
			return v, nil
		}
		if p.isOp("(") {
			p.next()
			args, err := p.args(")")
			if err != nil {
				return nil, err
			}
			return &Call{Name: t.text, Args: args}, nil
		}
		if p.isOp("[") {
			p.next()
			idx, err := p.expr()
			if err != nil {
				return nil, err
			}
			if err := p.expect("]"); err != nil {
				return nil, err
			}
			return &Var{Name: t.text, Index: idx}, nil
		}
		return &Ident{Name: t.text}, nil
	case tokEOF:
		return nil, p.errorf("want an expression")
	}
	p.i--
	return nil, p.errorf("want an expression")
}
