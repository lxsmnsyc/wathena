package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/lxsmnsyc/wathena/server/world"
)

// auditItem is one inventory slot in the item audit.
type auditItem struct {
	Slot   int    `json:"slot"`
	Item   uint32 `json:"item"`
	Amount int    `json:"amount"`
	Serial string `json:"serial"`
}

// auditPlayer is what one player holds.
type auditPlayer struct {
	Name        string      `json:"name"`
	CharacterID int64       `json:"character_id"`
	Zeny        int64       `json:"zeny"`
	Items       []auditItem `json:"items"`
}

// auditGround is one ground item.
type auditGround struct {
	Item   uint32 `json:"item"`
	Amount int    `json:"amount"`
	// Origin is the serial of the stack it was dropped from, or empty.
	Origin string `json:"origin,omitempty"`
}

type auditZone struct {
	Map     string        `json:"map"`
	Players []auditPlayer `json:"players"`
	Ground  []auditGround `json:"ground"`
}

// guildStorageMap is the map name of the audit entry that lists the
// guild storages, one "player" per guild named guild:<ID>, and the
// storages and carts in memory, one per storage named storage:<key>.
const guildStorageMap = "guild_storage"

// auditHandler lists every item and all zeny that players and the
// ground hold, zone by zone, with serials, for the duplication tests
// (decision 0017): GET /debug/wathena/items. It is for development
// only, since it shows serials.
func auditHandler(w *world.World) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		out := []auditZone{}
		// The guild storages come from the audit of the first zone with
		// players, read on its goroutine, so they agree with its players.
		var storages *auditZone
		for _, z := range w.Zones() {
			a, err := z.Audit(ctx)
			if err != nil {
				http.Error(rw, err.Error(), http.StatusServiceUnavailable)
				return
			}
			az := auditZone{Map: a.Map, Players: []auditPlayer{}, Ground: []auditGround{}}
			for _, p := range a.Players {
				ap := auditPlayer{Name: p.Name, CharacterID: p.Owner.ID, Zeny: p.Zeny, Items: []auditItem{}}
				for _, it := range p.Items {
					ap.Items = append(ap.Items, auditItem{Slot: it.Slot, Item: it.ID, Amount: it.Amount, Serial: it.Serial.String()})
				}
				az.Players = append(az.Players, ap)
			}
			for _, g := range a.Ground {
				ag := auditGround{Item: g.ItemID, Amount: g.Amount}
				if !g.Origin.IsZero() {
					ag.Origin = g.Origin.String()
				}
				az.Ground = append(az.Ground, ag)
			}
			out = append(out, az)
			if storages == nil && (len(a.Players) > 0 || z == w.Zones()[len(w.Zones())-1]) {
				st := auditZone{Map: guildStorageMap, Players: []auditPlayer{}, Ground: []auditGround{}}
				for _, s := range a.Storages {
					ap := auditPlayer{Name: fmt.Sprintf("guild:%d", s.Guild), Items: []auditItem{}}
					for _, it := range s.Items {
						ap.Items = append(ap.Items, auditItem{Slot: it.Slot, Item: it.ID, Amount: it.Amount, Serial: it.Serial.String()})
					}
					st.Players = append(st.Players, ap)
				}
				for _, s := range a.Personal {
					ap := auditPlayer{Name: "storage:" + s.Key.String(), Items: []auditItem{}}
					for _, it := range s.Items {
						ap.Items = append(ap.Items, auditItem{Slot: it.Slot, Item: it.ID, Amount: it.Amount, Serial: it.Serial.String()})
					}
					st.Players = append(st.Players, ap)
				}
				storages = &st
			}
		}
		if storages != nil {
			out = append(out, *storages)
		}
		rw.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(rw).Encode(out)
	}
}
