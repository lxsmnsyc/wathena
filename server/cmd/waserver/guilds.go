package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lxsmnsyc/wathena/server/emblem"
	"github.com/lxsmnsyc/wathena/server/social"
	"github.com/lxsmnsyc/wathena/server/world"
)

// Guild emblems and the development endpoints of guilds (decision 0021).

// accountGuilds answers the account pages' questions about guilds.
type accountGuilds struct{ reg *social.Registry }

func (a accountGuilds) MasterOf(charID int64) (int64, string, bool) {
	return a.reg.GuildMasterOf(charID)
}

func (a accountGuilds) SetEmblem(guild int64, png []byte) (int, error) {
	v, _, err := a.reg.SetEmblem(guild, png)
	return v, err
}

// emblemHandler serves a guild's emblem: GET /emblems/<guild>/<version>.png.
// Only the current version is served. Its URL changes with each version,
// so it may be cached for good. Any origin may draw it.
func emblemHandler(reg *social.Registry) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("guild"), 10, 64)
		ver, ok := strings.CutSuffix(r.PathValue("file"), ".png")
		v, verr := strconv.Atoi(ver)
		if err != nil || !ok || verr != nil || id <= 0 {
			http.NotFound(rw, r)
			return
		}
		e, found := reg.EmblemOf(id)
		if !found || e.Version != v {
			http.NotFound(rw, r)
			return
		}
		h := rw.Header()
		h.Set("Content-Type", "image/png")
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Content-Security-Policy", "default-src 'none'")
		_, _ = rw.Write(e.PNG)
	}
}

// devEmblemHandler sets a guild's emblem from the request body, as the
// account pages do: POST /debug/wathena/emblem?guild=<name>. It is for
// --dev, which has no account pages.
func devEmblemHandler(reg *social.Registry) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		id, ok := reg.FindGuild(r.URL.Query().Get("guild"))
		if !ok {
			http.Error(rw, "there is no such guild", http.StatusNotFound)
			return
		}
		data, err := io.ReadAll(io.LimitReader(r.Body, emblem.MaxUpload+1))
		if err != nil {
			http.Error(rw, err.Error(), http.StatusBadRequest)
			return
		}
		png, err := emblem.Process(data)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusBadRequest)
			return
		}
		v, _, err := reg.SetEmblem(id, png)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusConflict)
			return
		}
		rw.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(rw).Encode(map[string]int64{"guild": id, "version": int64(v)})
	}
}

// devGuildStorageHandler opens a player's guild storage as an NPC would:
// POST /debug/wathena/guild-storage?name=<character>. The e2e tests and
// the development stack use it, since the guild storage NPCs stand on
// town maps.
func devGuildStorageHandler(w *world.World) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		name := r.URL.Query().Get("name")
		for _, z := range w.Zones() {
			o, found, err := z.OpenGuildStorageFor(ctx, name)
			if err != nil {
				http.Error(rw, err.Error(), http.StatusServiceUnavailable)
				return
			}
			if found {
				rw.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(rw).Encode(map[string]int{"outcome": int(o)})
				return
			}
		}
		http.Error(rw, "no such player is online", http.StatusNotFound)
	}
}

// devStorageHandler opens a player's storage as an NPC would:
// POST /debug/wathena/storage?name=<character>, with &storage=<record>
// for an extra storage of data/storages.yml. The development stack uses
// it, since the placeholder maps have no Kafra.
func devStorageHandler(w *world.World) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		q := r.URL.Query()
		for _, z := range w.Zones() {
			opened, found, err := z.OpenStorageFor(ctx, q.Get("name"), q.Get("storage"))
			if err != nil {
				http.Error(rw, err.Error(), http.StatusServiceUnavailable)
				return
			}
			if found {
				rw.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(rw).Encode(map[string]bool{"opened": opened})
				return
			}
		}
		http.Error(rw, "no such player is online", http.StatusNotFound)
	}
}

// devCartHandler gives a player a cart, as setcart:
// POST /debug/wathena/cart?name=<character>&look=<1 to 5, or 0>. The
// player needs Pushcart.
func devCartHandler(w *world.World) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		q := r.URL.Query()
		look, err := strconv.Atoi(q.Get("look"))
		if err != nil {
			http.Error(rw, "look is 0 to 5", http.StatusBadRequest)
			return
		}
		for _, z := range w.Zones() {
			ok, found, err := z.SetCartFor(ctx, q.Get("name"), look)
			if err != nil {
				http.Error(rw, err.Error(), http.StatusServiceUnavailable)
				return
			}
			if found {
				rw.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(rw).Encode(map[string]bool{"changed": ok})
				return
			}
		}
		http.Error(rw, "no such player is online", http.StatusNotFound)
	}
}
