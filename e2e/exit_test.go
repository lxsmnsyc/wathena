package e2e

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/store/storetest"
)

// ExitMinutesEnv sets how long the moves of TestPhase5ExitCheck run, in
// minutes. scripts/check.sh's e2e step sets it to 30. Without it the
// moves run for exitShort.
const ExitMinutesEnv = "WATHENA_EXIT_MINUTES"

// exitShort is how long the moves run by default, the restart left out.
const exitShort = 40 * time.Second

// exitTokens are the tokens each shopper gets: enough for exitPurchases
// purchases of 3 Red Potions.
const exitTokens = exitPurchases * 3 * TokensPerPotion

// exitDuration reads ExitMinutesEnv.
func exitDuration(t *testing.T) time.Duration {
	t.Helper()
	v := os.Getenv(ExitMinutesEnv)
	if v == "" {
		return exitShort
	}
	m, err := strconv.ParseFloat(v, 64)
	if err != nil || m <= 0 {
		t.Fatalf("%s=%q is not a number of minutes above 0", ExitMinutesEnv, v)
	}
	return time.Duration(m * float64(time.Minute))
}

// exitCounts are the counters of the exit check's moves.
type exitCounts struct {
	market, shop, buying MarketCounts
	trades               TradeCounts
	personal             PersonalCounts
}

// exitPurchases is how many times at most each shopper buys Red Potions
// with tokens over the whole check.
const exitPurchases = 20

// phase5Exit is the exit check of phase 5, with 24 Merchants in 5
// groups:
//   - On e2e_field, 6 open vending shops from their carts and buy from
//     each other's, and mail each other Jellopy and zeny.
//   - On e2e_field, 6 trade Jellopy and zeny, and use their storages and
//     carts.
//   - On the start map, 4 buy at the Exchange: Jellopy for Jellopy, and
//     Red Potions for tokens.
//   - On the start map, 4 open buying stores and sell to each other's.
//   - On the start map, 4 drop Jellopy, pick up each other's, and close
//     their connections in the middle of drops and pick-ups.
//
// Every move but the drops and pick-ups closes the connection in the
// middle one time in 8 to 15, as in the duplication tests.
//
// The groups that use storages, carts and mail are on e2e_field. The
// audit reads the storages, carts and mails on the first map with
// players by name, which is e2e_field, so they agree with those
// players' inventories.
func phase5Exit(srv *Server, d time.Duration, c *exitCounts) ExitCheck {
	vend, buy, mail, take := VendMove(Jellopy, &c.market), VendBuyMove(&c.market), MailMove("ExitMarket", 6, &c.market), MailTakeMove(&c.market)
	vend.Weight, buy.Weight, mail.Weight, take.Weight = 10, 20, 8, 8
	trade := TradeMove("ExitTrader", 6, Jellopy, &c.trades)
	storage, cart := PersonalStorageMove(Jellopy, &c.personal), CartMove(Jellopy, &c.personal)
	trade.Weight, storage.Weight, cart.Weight = 40, 4, 6
	exchange, tokens := ExchangeMove(&c.shop), TokenMove(&c.shop, d/exitPurchases)
	exchange.Weight, tokens.Weight = 10, 10
	open, sell := BuyingStoreMove(&c.buying), SellToStoreMove(&c.buying)
	open.Weight, sell.Weight = 10, 20
	dropMove, pickUp, disconnect := DropMove(Jellopy), PickUpMove(), DisconnectMove(Jellopy)
	dropMove.Weight, pickUp.Weight, disconnect.Weight = 20, 40, 1
	return ExitCheck{
		Server:   srv,
		Prefix:   "Exit",
		Duration: d,
		Seed:     1,
		Groups: []ExitGroup{
			{Prefix: "ExitMarket", Bots: 6, Map: FieldMap, Moves: []Move{vend, buy, mail, take}, Setup: MarketSetup(5000)},
			{Prefix: "ExitTrader", Bots: 6, Map: FieldMap, Moves: []Move{trade, storage, cart}, Settle: SettleTrades, Setup: CartSetup(5000)},
			{Prefix: "ExitShopper", Bots: 4, Map: StartMap, Moves: []Move{exchange, tokens}, Setup: TokenSetup(exitTokens)},
			{Prefix: "ExitBuyer", Bots: 4, Map: StartMap, Moves: []Move{open, sell}, Setup: CartSetup(5000)},
			{Prefix: "ExitGround", Bots: 4, Map: StartMap, Moves: []Move{dropMove, pickUp, disconnect}, Settle: SettlePickUps(12, 12), Ground: true},
		},
	}
}

// TestPhase5ExitCheck runs the phase 5 exit check with PostgreSQL: the
// four groups of phase5Exit move at the same time on both maps, with a
// restart in the middle, for 40 seconds, or for the minutes of
// WATHENA_EXIT_MINUTES. Then it checks the saved totals and the tokens
// in the logs, and logs a summary.
func TestPhase5ExitCheck(t *testing.T) {
	t.Parallel()
	if os.Getenv(storetest.Env) == "" {
		t.Skip("the exit check restarts the server with its state, which needs PostgreSQL: set " + storetest.Env)
	}
	d := exitDuration(t)
	srv := Start(t, Options{StartItems: "909:30", StartJob: MerchantJob, Ruleset: "e2e.marketrules", Database: true, Race: true})
	var c exitCounts
	x := phase5Exit(srv, d, &c)
	began := time.Now()
	rep := x.Run(t)
	ran := time.Since(began)
	end := DupTest{Server: srv, Prefix: x.Prefix}
	start := rep.End
	// The serials of the start, so every other saved serial needs a
	// history in the item log.
	start.Serials = rep.Start.Serials
	end.CheckDatabase(t, DupReport{Start: start, Dropped: rep.Ground.Dropped, Added: rep.Ground.Added, Reconnects: rep.Reconnects + rep.Restarted})
	given, spent := x.CheckTokens(t, rep)
	logExitSummary(t, x, rep, &c, d, ran, given, spent)
	checkExitReport(t, rep, &c)
	srv.CheckLogs()
}

// logExitSummary logs the numbers of the exit check. With
// WATHENA_EXIT_REPORT set to a file, it also writes them there, so
// scripts/check.sh shows them without -v.
func logExitSummary(t *testing.T, x ExitCheck, rep ExitReport, c *exitCounts, d, ran time.Duration, given, spent int64) {
	t.Helper()
	var lines []string
	logf := func(format string, args ...any) {
		t.Helper()
		t.Logf(format, args...)
		lines = append(lines, fmt.Sprintf(format, args...))
	}
	defer func() {
		if path := os.Getenv("WATHENA_EXIT_REPORT"); path != "" {
			if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
				t.Error(err)
			}
		}
	}()
	disconnects := c.market.Dropped.Load() + c.shop.Dropped.Load() + c.buying.Dropped.Load() + c.trades.Dropped.Load() + c.personal.Dropped.Load() + int64(rep.Moves["disconnect"])
	logf("exit check: %d bots in %d groups on 2 maps (%s and %s), %.1f minutes of moves, %.1f minutes in all",
		x.bots(), len(x.Groups), StartMap, FieldMap, d.Minutes(), ran.Minutes())
	logf("trades: %d requests, %d opened, %d completed, %d closed otherwise",
		c.trades.Requested.Load(), c.trades.Opened.Load(), c.trades.Completed.Load(), c.trades.Cancelled.Load())
	logf("storage and cart moves: %d storages opened; %d puts, %d takes, %d cart puts, %d cart takes, %d cart to storage, %d storage to cart, %d refused",
		c.personal.Opened.Load(), c.personal.Put.Load(), c.personal.Take.Load(), c.personal.CartPut.Load(), c.personal.CartTake.Load(),
		c.personal.CartToStorage.Load(), c.personal.StorageToCart.Load(), c.personal.Refused.Load())
	logf("mails: %d sent, %d taken", c.market.Mailed.Load(), c.market.Taken.Load())
	logf("vending: %d shops opened, %d closed, %d sales", c.market.ShopsOpened.Load(), c.market.ShopsClosed.Load(), c.market.Bought.Load())
	logf("buying stores: %d opened, %d sales", c.buying.StoresOpened.Load(), c.buying.SoldToStores.Load())
	logf("shop purchases at the Exchange: %d of Jellopy for Jellopy, %d of Red Potions for tokens", c.shop.Exchanged.Load(), c.shop.TokenBuys.Load())
	logf("ground: %d drops and %d pick-ups seen", rep.Ground.Dropped, rep.Ground.Added)
	logf("refused steps: %d vending and mail, %d Exchange, %d buying store", c.market.Refused.Load(), c.shop.Refused.Load(), c.buying.Refused.Load())
	logf("disconnects: %d connections closed by the bots in the middle of a move, %d logins again, %d closed by the server outside the restart",
		disconnects, rep.Reconnects, len(rep.Closed))
	logf("restart: after %.1f minutes, %d logins again, %.1f seconds from the stop until every bot was back",
		(d / 2).Minutes(), rep.Restarted, rep.RestartTook.Seconds())
	logf("audits: %d with every bot online, %d with a bot away", rep.Audits, rep.Partial)
	logf("moves done by name: %v", rep.Moves)
	for _, s := range []struct {
		name string
		t    Totals
	}{{"start", rep.Start}, {"saved at the restart", rep.Saved}, {"after the restart", rep.Restored}, {"end", rep.End}} {
		logf("totals %s: %d Jellopy (%d in storages, carts and mails), %d Red Potions, %d zeny, %d tokens, %d serials",
			s.name, s.t.Items[Jellopy], s.t.Stored[Jellopy], s.t.Items[RedPotion], s.t.Zeny, s.t.Currencies[Tokens], len(s.t.Serials))
	}
	logf("tokens in the zeny log: %d given, %d spent, %d held at the end", given, spent, rep.End.Currencies[Tokens])
}

// checkExitReport checks that every kind of move happened.
func checkExitReport(t *testing.T, rep ExitReport, c *exitCounts) {
	t.Helper()
	for name, n := range map[string]int64{
		"completed trades":        c.trades.Completed.Load(),
		"storage puts":            c.personal.Put.Load(),
		"cart puts":               c.personal.CartPut.Load(),
		"mails sent":              c.market.Mailed.Load(),
		"mails taken":             c.market.Taken.Load(),
		"vending sales":           c.market.Bought.Load(),
		"buying store sales":      c.buying.SoldToStores.Load(),
		"Exchange purchases":      c.shop.Exchanged.Load(),
		"token purchases":         c.shop.TokenBuys.Load(),
		"drops":                   int64(rep.Ground.Dropped),
		"logins again":            int64(rep.Reconnects),
		"logins after a restart":  int64(rep.Restarted),
		"5 audits with every bot": int64(rep.Audits / 5),
	} {
		if n == 0 {
			t.Errorf("too little happened: no %s", name)
		}
	}
	// A move sent just before a connection closed can happen without its
	// bot seeing it.
	if diff := rep.Ground.Dropped - rep.Ground.Added; diff > rep.Reconnects+rep.Restarted || -diff > rep.Reconnects+rep.Restarted || rep.End.Ground != 0 {
		t.Errorf("%d drops and %d pick-ups seen with %d logins again, %d items left on the ground",
			rep.Ground.Dropped, rep.Ground.Added, rep.Reconnects+rep.Restarted, rep.End.Ground)
	}
}
