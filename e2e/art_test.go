package e2e

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"testing"

	"github.com/andybalholm/brotli"
	"golang.org/x/image/webp"

	"github.com/lxsmnsyc/wathena/internal/assets/build"
	"github.com/lxsmnsyc/wathena/internal/assets/wspr"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/modassets"
)

// TestModArt: the client data names the fixture's art for the Test
// Charm's icon and the Dummy's sprite, by the IDs the server gave them,
// and the server serves both from the fixture's build, as the client
// loads them (decision 0033). Version 2 of the client data names both
// records, and holds the charm's description (review decision 40).
func TestModArt(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{})
	p := srv.Join(t, "Painter")
	dummy := p.Find(wathenav1.EntityKind_ENTITY_KIND_MONSTER, "Dummy")

	var doc modassets.ClientData
	getJSON(t, srv.HTTP()+modassets.ClientDataPath, &doc)
	if len(doc.Mods) != 1 || doc.Mods[0].ID != "e2e.fixture" || doc.Mods[0].Assets != "/assets/mods/e2e.fixture/0.1.0/" {
		t.Fatalf("mods %+v", doc.Mods)
	}
	if doc.Version != 2 {
		t.Errorf("client data version %d", doc.Version)
	}
	charm := doc.Items[strconv.FormatUint(uint64(Charm), 10)]
	icon := charm.Icon
	if icon != "mod/e2e.fixture/icon/item/Charm" || charm.Name != "Test Charm" ||
		len(charm.Description) != 1 || charm.Description[0] != "A charm for the end-to-end tests." {
		t.Errorf("the charm is %+v", charm)
	}
	mob := doc.Monsters[strconv.FormatUint(uint64(dummy.Body), 10)]
	sprite := mob.Sprite
	if sprite != "mod/e2e.fixture/sprite/mob/Dummy" || mob.Name != "Dummy" {
		t.Errorf("the dummy, body %d, is %+v", dummy.Body, mob)
	}

	base := srv.HTTP() + doc.Mods[0].Assets
	data, typ := fetchAsset(t, base, icon)
	if c, err := webp.DecodeConfig(bytes.NewReader(data)); err != nil || typ != "texture" || c.Width != 24 {
		t.Errorf("icon: %s, %v, %+v", typ, err, c)
	}
	data, typ = fetchAsset(t, base, sprite)
	s, err := wspr.Decode(data)
	if err != nil || typ != "sprite" || len(s.Actions) != 5*8 {
		t.Errorf("sprite: %s, %v", typ, err)
	}
	srv.CheckLogs()
}

// fetchAsset resolves a name through a build's manifest and shard, as the
// client does, downloads it, and checks its hash.
func fetchAsset(t *testing.T, base, name string) ([]byte, string) {
	t.Helper()
	var m build.Manifest
	getJSON(t, base+"manifest.json", &m)
	h := m.Shards[build.ShardOf(name)]
	var shard map[string]struct {
		H string `json:"h"`
		T string `json:"t"`
	}
	getJSON(t, fmt.Sprintf("%s%s/%s.json", base, h[:2], h), &shard)
	e, ok := shard[name]
	if !ok {
		t.Fatalf("%s is not in the build", name)
	}
	ext := map[string]string{"texture": "webp", "sprite": "wspr"}[e.T]
	data := getBody(t, fmt.Sprintf("%s%s/%s.%s", base, e.H[:2], e.H, ext))
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != e.H {
		t.Fatalf("%s does not match its hash", name)
	}
	return data, e.T
}

// getBody fetches a file as a browser does, with Brotli.
func getBody(t *testing.T, url string) []byte {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept-Encoding", "br")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("%s: %s", url, res.Status)
	}
	var r io.Reader = res.Body
	if res.Header.Get("Content-Encoding") == "br" {
		r = brotli.NewReader(res.Body)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func getJSON(t *testing.T, url string, v any) {
	t.Helper()
	if err := json.Unmarshal(getBody(t, url), v); err != nil {
		t.Fatalf("%s: %v", url, err)
	}
}
