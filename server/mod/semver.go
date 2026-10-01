package mod

import (
	"cmp"
	"fmt"
	"strconv"
	"strings"
)

// Version is a semantic version: major.minor.patch, with an optional
// pre-release part. Build metadata after "+" is ignored, as semver says.
type Version struct {
	Major, Minor, Patch int
	// Pre is the pre-release part, such as "beta.1", or empty.
	Pre string
}

// ParseVersion reads a full version, such as 1.2.0 or 1.2.0-beta.1.
func ParseVersion(s string) (Version, error) {
	v, parts, err := parseVersion(s)
	if err != nil {
		return Version{}, err
	}
	if parts != 3 {
		return Version{}, fmt.Errorf("%q is not a semantic version such as 1.2.0", s)
	}
	return v, nil
}

// parseVersion reads a version with 1 to 3 numbers, and returns how many
// it had. Ranges use the short forms: ">=1.0" means ">=1.0.0".
func parseVersion(s string) (Version, int, error) {
	bad := fmt.Errorf("%q is not a semantic version such as 1.2.0", s)
	s, _, _ = strings.Cut(s, "+")
	core, pre, hasPre := strings.Cut(s, "-")
	if hasPre && !validPre(pre) {
		return Version{}, 0, bad
	}
	nums := strings.Split(core, ".")
	if len(nums) == 0 || len(nums) > 3 {
		return Version{}, 0, bad
	}
	var n [3]int
	for i, p := range nums {
		if p == "" || len(p) > 1 && p[0] == '0' {
			return Version{}, 0, bad
		}
		v, err := strconv.Atoi(p)
		if err != nil || v < 0 {
			return Version{}, 0, bad
		}
		n[i] = v
	}
	return Version{Major: n[0], Minor: n[1], Patch: n[2], Pre: pre}, len(nums), nil
}

func validPre(s string) bool {
	if s == "" {
		return false
	}
	for id := range strings.SplitSeq(s, ".") {
		if id == "" {
			return false
		}
		for _, r := range id {
			if (r < '0' || r > '9') && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && r != '-' {
				return false
			}
		}
	}
	return true
}

func (v Version) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Pre != "" {
		s += "-" + v.Pre
	}
	return s
}

// Compare orders versions as semver does. A pre-release comes before its
// release.
func (v Version) Compare(o Version) int {
	if c := cmp.Compare(v.Major, o.Major); c != 0 {
		return c
	}
	if c := cmp.Compare(v.Minor, o.Minor); c != 0 {
		return c
	}
	if c := cmp.Compare(v.Patch, o.Patch); c != 0 {
		return c
	}
	switch {
	case v.Pre == o.Pre:
		return 0
	case v.Pre == "":
		return 1
	case o.Pre == "":
		return -1
	}
	a, b := strings.Split(v.Pre, "."), strings.Split(o.Pre, ".")
	for i := range min(len(a), len(b)) {
		ai, aerr := strconv.Atoi(a[i])
		bi, berr := strconv.Atoi(b[i])
		var c int
		switch {
		case aerr == nil && berr == nil:
			c = cmp.Compare(ai, bi)
		case aerr == nil:
			c = -1
		case berr == nil:
			c = 1
		default:
			c = strings.Compare(a[i], b[i])
		}
		if c != 0 {
			return c
		}
	}
	return cmp.Compare(len(a), len(b))
}

// Range is a set of versions, such as ">=1.0 <2.0" or "^1.2 || ^2.0".
//
// - Comparators separated by spaces must all match.
// - Groups separated by "||" are alternatives.
// - Operators are >=, >, <=, <, = and none (the same as =), ^ (same
// major version, or same minor version below 1.0), and ~ (same minor
// version).
// - A version may leave out its minor or patch number, which count as 0.
type Range struct {
	text   string
	groups [][]comparator
}

type comparator struct {
	op string
	v  Version
}

// ParseRange reads a version range.
func ParseRange(s string) (Range, error) {
	r := Range{text: strings.TrimSpace(s)}
	if r.text == "" {
		return Range{}, fmt.Errorf("the version range is empty")
	}
	for group := range strings.SplitSeq(r.text, "||") {
		fields := strings.Fields(group)
		if len(fields) == 0 {
			return Range{}, fmt.Errorf("%q has an empty alternative", s)
		}
		var cs []comparator
		for _, f := range fields {
			op := ""
			for _, o := range []string{">=", "<=", ">", "<", "=", "^", "~"} {
				if strings.HasPrefix(f, o) {
					op = o
					break
				}
			}
			v, _, err := parseVersion(f[len(op):])
			if err != nil {
				return Range{}, fmt.Errorf("%q: %w", s, err)
			}
			if op == "" {
				op = "="
			}
			cs = append(cs, comparator{op, v})
		}
		r.groups = append(r.groups, cs)
	}
	return r, nil
}

func (r Range) String() string { return r.text }

// Match reports whether v is in the range.
func (r Range) Match(v Version) bool {
	for _, g := range r.groups {
		ok := true
		for _, c := range g {
			if !c.match(v) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func (c comparator) match(v Version) bool {
	d := v.Compare(c.v)
	switch c.op {
	case ">=":
		return d >= 0
	case ">":
		return d > 0
	case "<=":
		return d <= 0
	case "<":
		return d < 0
	case "=":
		return d == 0
	case "~":
		return d >= 0 && v.Major == c.v.Major && v.Minor == c.v.Minor
	case "^":
		if d < 0 || v.Major != c.v.Major {
			return false
		}
		return c.v.Major > 0 || v.Minor == c.v.Minor
	}
	return false
}
