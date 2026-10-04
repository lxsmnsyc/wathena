package account_test

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/lxsmnsyc/wathena/server/account"
)

// fakeGuilds is a world where one character masters a guild.
type fakeGuilds struct {
	mu     sync.Mutex
	master int64
	got    []byte
}

func (f *fakeGuilds) MasterOf(charID int64) (int64, string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if charID == f.master && charID != 0 {
		return 42, "Emblemers", true
	}
	return 0, "", false
}

func (f *fakeGuilds) SetEmblem(guild int64, png []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.got = png
	return 1, nil
}

func emblemPNG(t *testing.T, n int) []byte {
	t.Helper()
	m := image.NewRGBA(image.Rect(0, 0, n, n))
	for y := range n {
		for x := range n {
			m.Set(x, y, color.RGBA{1, 2, 3, 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, m); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// TestGuildEmblemUpload checks the emblem page: only a guild's master
// may upload, the image passes server/emblem, and the world gets the
// re-encoded PNG.
func TestGuildEmblemUpload(t *testing.T) {
	guilds := &fakeGuilds{}
	e := setup(t, func(c *account.Config) { c.Guilds = guilds })
	e.signup(e.c, "emblemer", "correct horse battery")
	e.makeCharacter(e.c, "Emblemer", "0")
	body, _ := e.page(e.c, "/account/guild/emblem")
	if !strings.Contains(body, "None of your characters is a guild's master.") {
		t.Fatalf("page without a guild: %s", body)
	}
	var id int64
	if err := e.admin.QueryRow(context.Background(), `SELECT id FROM characters WHERE name = 'Emblemer'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	guilds.mu.Lock()
	guilds.master = id
	guilds.mu.Unlock()
	upload := func(img []byte) (int, string) {
		_, token := e.page(e.c, "/account/guild/emblem")
		var b bytes.Buffer
		w := multipart.NewWriter(&b)
		_ = w.WriteField("csrf", token)
		_ = w.WriteField("slot", "0")
		f, _ := w.CreateFormFile("emblem", "emblem.png")
		_, _ = f.Write(img)
		_ = w.Close()
		req, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/account/guild/emblem", &b)
		req.Header.Set("Content-Type", w.FormDataContentType())
		resp, err := e.c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		out, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(out)
	}
	if code, out := upload(emblemPNG(t, 32)); code != http.StatusBadRequest || !strings.Contains(out, "24 by 24") {
		t.Errorf("32 px: %d %s", code, out)
	}
	img := emblemPNG(t, 24)
	if code, out := upload(img); code != http.StatusOK || !strings.Contains(out, "new emblem") {
		t.Fatalf("upload: %d %s", code, out)
	}
	guilds.mu.Lock()
	got := guilds.got
	guilds.mu.Unlock()
	if len(got) == 0 || bytes.Equal(got, img) {
		t.Errorf("the world got %d bytes; the upload must be re-encoded", len(got))
	}
}
