package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/lxsmnsyc/wathena/tools/botclient"
)

// RestoreCounts counts the restores of a duplication test (decision
// 0029).
type RestoreCounts struct {
	// Moved counts the restores that moved a serial into a mail. Gone
	// counts the serials that were gone when the restore looked, which
	// the test's staff account may not copy. Busy counts the restores
	// that gave up on items that kept moving. Cut counts the requests
	// that gave up before the answer came, which may have moved a serial
	// or not.
	Moved, Gone, Busy, Cut atomic.Int64
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
	return restoreMove(srv, prefix, bots, c, 0)
}

// CutRestoreMove is RestoreMove on a machine that stalls, with requests
// that give up early, as a staff member's browser that closes or a proxy
// that times out:
//   - It prefers serials in the saved inventories of bots that are away,
//     which a restore loads under a store hold.
//   - Soon after the request starts, it stalls the start zone for twice
//     cut, so the restore often waits for the zone. Half the stalls come
//     after the zone takes its next call and before it runs it, which
//     is often the restore itself.
//   - The request gives up after between cut/2 and cut.
//
// The server must make such a restore whole or not at all: a restore
// that the zone made after its request gave up must not leave its items
// where they were too.
func CutRestoreMove(srv *Server, prefix string, bots int, c *RestoreCounts, cut time.Duration) Move {
	m := restoreMove(srv, prefix, bots, c, cut)
	m.Name = "cut_restore"
	return m
}

// awaySerials lists the serials in the saved inventories of the bots
// that are not in an audit.
func awaySerials(ctx context.Context, srv *Server, prefix string, zones []AuditZone) ([]string, error) {
	online := []string{}
	for _, z := range zones {
		for _, p := range z.Players {
			online = append(online, p.Name)
		}
	}
	rows, err := srv.DB.Query(ctx, `SELECT i.serial::text FROM inventory i JOIN characters c ON c.id = i.character_id
		WHERE c.name LIKE $1 AND NOT c.name = ANY($2) ORDER BY 1`, prefix+"%", online)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func restoreMove(srv *Server, prefix string, bots int, c *RestoreCounts, cut time.Duration) Move {
	return Move{Name: "restore", Do: func(ctx context.Context, _ *botclient.Bot, rnd *rand.Rand) (bool, error) {
		zones, err := srv.Audit(ctx)
		if err != nil {
			return false, ignoreDone(ctx, err)
		}
		var serials []string
		if cut > 0 && srv.DB != nil {
			if serials, err = awaySerials(ctx, srv, prefix, zones); err != nil {
				return false, ignoreDone(ctx, err)
			}
		}
		if len(serials) == 0 {
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
		// The request outlives the run's end, so the test learns what
		// the server did with every restore it started, and the count of
		// audit rows is exact. The totals do not depend on it.
		limit := DefaultTimeout
		if cut > 0 {
			limit = cut/2 + time.Duration(rnd.Int64N(int64(cut/2)))
			delay := time.Duration(rnd.Int64N(int64(20 * time.Millisecond)))
			next := rnd.IntN(2) == 0
			go func() {
				time.Sleep(delay)
				_ = srv.Stall(context.WithoutCancel(ctx), 2*cut, next)
			}()
		}
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), limit)
		mail, code, err := srv.Restore(rctx, serial, to, amount)
		cancel()
		switch {
		case err != nil && cut > 0 && rctx.Err() != nil:
			c.Cut.Add(1)
			return false, nil
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

// Stall holds the start zone's goroutine for d, as a machine that
// stalls does, through the server's POST /debug/wathena/stall. With
// next, the zone stalls after it takes its next call from another
// goroutine, before it runs it, and Stall returns at once.
func (s *Server) Stall(ctx context.Context, d time.Duration, next bool) error {
	u := s.Debug + "/debug/wathena/stall?ms=" + fmt.Sprint(d.Milliseconds())
	if next {
		u += "&next=1"
	}
	_, err := s.post(ctx, u, nil)
	return err
}

// LeaveMove closes the bot's connection and stays away for a random
// time below away, before the test logs the bot in again. Restores of
// its items meanwhile find them in an offline inventory, which they
// load under a store hold.
func LeaveMove(away time.Duration) Move {
	return Move{Name: "leave", Do: func(ctx context.Context, b *botclient.Bot, rnd *rand.Rand) (bool, error) {
		_ = b.CloseNow()
		select {
		case <-ctx.Done():
		case <-time.After(time.Duration(rnd.Int64N(int64(away)))):
		}
		return true, errLeft
	}}
}
