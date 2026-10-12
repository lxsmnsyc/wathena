package modscript

import (
	"errors"
	"fmt"
	"strings"

	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/server/rules"
)

// Rule reading and limit expansion (proposal 0002, review decision 32):
//   - rules.is, rules.get and rules.get_limit read the world's ruleset.
//     They need no permission.
//   - player:expand raises or lowers one of a player's limits, within the
//     limit's base and max. It needs player.expand_limits.

// MaxExpandAmount bounds player:expand's amount: the largest engine
// ceiling of a limit a player holds.
const MaxExpandAmount = 10_000

// LimitHost is what player:expand needs from the zone.
type LimitHost interface {
	// ExpandLimit changes one of a player's limits by amount, within the
	// limit's base and max, saves it, and returns the new value. limit
	// is one of rules.ExpandableLimits.
	ExpandLimit(player int64, limit string, amount int) (int, error)
}

// worldRules are the world's ruleset, or rAthena's renewal rules
// without one.
func (r *Runtime) worldRules() *rules.Rules {
	if r.opts.Rules != nil {
		return r.opts.Rules
	}
	return rules.Of(true)
}

// rulesRead is rules.is, rules.get and rules.get_limit. Arguments: the
// call ("is", "get" or "limit") and the name.
func (r *Runtime) rulesRead(c *script.Call) error {
	op, name := c.String(0), c.String(1)
	rl := r.worldRules()
	switch op {
	case "is":
		on, ok := rl.Is(name)
		if !ok {
			return fmt.Errorf("rules.is: there is no flag %q; the flags are %s, and %s<skill> for a rebalanced skill or a song, such as %sMG_FIREBALL",
				name, strings.Join(rules.FlagNames, ", "), rules.SkillFlagPrefix, rules.SkillFlagPrefix)
		}
		c.Return(script.Bool(on))
	case "get":
		v, ok := rl.Rule(name)
		if !ok {
			return fmt.Errorf("rules.get: there is no rule %q; the rules are %s", name, strings.Join(rules.RuleNames(), ", "))
		}
		if b, _ := rules.FindBound(rules.RuleBounds, name); b.Bool {
			c.Return(script.Bool(v == 1))
			return nil
		}
		c.Return(script.Int(int64(v)))
	case "limit":
		rg, ok := rules.LimitsOrDefault(rl).RangeOf(name)
		if !ok {
			return fmt.Errorf("rules.get_limit: there is no limit %q; the limits are %s", name, strings.Join(rules.LimitNames(), ", "))
		}
		c.Return(script.Int(int64(rg.Base)), script.Int(int64(rg.Max)))
	default:
		return fmt.Errorf("rules: no call %q", op)
	}
	return nil
}

// playerExpand is player:expand(limit, amount).
func (r *Runtime) playerExpand(c *script.Call) error {
	const what = "player:expand"
	id, err := playerID(c)
	if err != nil {
		return err
	}
	limit := c.String(1)
	if !rules.Expandable(limit) {
		return fmt.Errorf("%s: %q is not a limit a player expands; the limits are %s", what, limit, strings.Join(rules.ExpandableLimits, ", "))
	}
	amount, ok := c.Arg(2).AsInt()
	if !ok || amount < -MaxExpandAmount || amount > MaxExpandAmount {
		return fmt.Errorf("%s: the amount must be an integer from %d to %d", what, -MaxExpandAmount, MaxExpandAmount)
	}
	if err := r.mutating(what); err != nil {
		return err
	}
	if err := r.need(what, "player.expand_limits"); err != nil {
		return err
	}
	h, ok := r.host.(LimitHost)
	if !ok {
		return errors.New(what + ": limits cannot be expanded here")
	}
	v, err := h.ExpandLimit(id, limit, int(amount))
	if err != nil {
		return err
	}
	c.Return(script.Int(int64(v)))
	return nil
}
