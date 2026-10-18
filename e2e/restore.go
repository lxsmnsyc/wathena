package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/url"
	"strings"
	"sync/atomic"

	"github.com/lxsmnsyc/wathena/tools/botclient"
)

// RestoreCounts counts the restores of a duplication test (decision
// 0029).
type RestoreCounts struct {
	// Moved counts the restores that moved a serial into a mail. Gone
	// counts the serials that were gone when the restore looked, which
	// the test's staff account may not copy. Busy counts the restores
	// that gave up on items that kept moving.
	Moved, Gone, Busy atomic.Int64
}

// Restore restores a serial to a character by mail, through the
// server's POST /debug/wathena/restore, as the admin API's POST /restore
// does without items.create. It returns the mail's ID, or the error
// code of a restore that did not happen.
func (s *Server) Restore(ctx context.Context, serial, to string, amount int) (int64, string, error) {
	q := url.Values{"serial": {serial}, "to": {to}}
	if amount > 0 {
		q.Set("amount", fmt.Sprint(amount))
	}
	body, err := s.post(ctx, s.Debug+"/debug/wathena/restore?"+q.Encode(), nil)
	if err != nil {
		return 0, "", err
	}
	var out struct {
		Mail  int64  `json:"mail"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return 0, "", err
	}
	return out.Mail, out.Error, nil
}

// RestoreMove restores a random serial that a bot, a storage, a cart or
// a mail holds to a random bot, while the bots trade, use their storages
// and carts, and lose their connections. The serial moves with whatever
// its holder does, so it may be gone, in a trade, or with a bot that is
// logging in again.
func RestoreMove(srv *Server, prefix string, bots int, c *RestoreCounts) Move {
	return Move{Name: "restore", Do: func(ctx context.Context, _ *botclient.Bot, rnd *rand.Rand) (bool, error) {
		zones, err := srv.Audit(ctx)
		if err != nil {
			return false, ignoreDone(ctx, err)
		}
		var serials []string
		for _, z := range zones {
			for _, p := range z.Players {
				if !strings.HasPrefix(p.Name, prefix) && z.Map != GuildStorageMap {
					continue
				}
				for _, it := range p.Items {
					serials = append(serials, it.Serial)
				}
			}
		}
		if len(serials) == 0 {
			return false, nil
		}
		serial := serials[rnd.IntN(len(serials))]
		to := fmt.Sprintf("%s%d", prefix, 1+rnd.IntN(bots))
		// Half the restores take part of the stack.
		amount := 0
		if rnd.IntN(2) == 0 {
			amount = 1
		}
		// The request outlives the run's end, so a restore that the
		// server commits as the run stops is still counted.
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), DefaultTimeout)
		mail, code, err := srv.Restore(rctx, serial, to, amount)
		cancel()
		switch {
		case err != nil:
			return false, ignoreDone(ctx, err)
		case mail > 0:
			c.Moved.Add(1)
			return true, nil
		case code == "forbidden":
			c.Gone.Add(1)
		case code == "conflict" || code == "bad_request":
			c.Busy.Add(1)
		default:
			return false, fmt.Errorf("restore of %s to %s: %s", serial, to, code)
		}
		return false, nil
	}}
}
