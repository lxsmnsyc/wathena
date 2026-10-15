package account

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// samplesEnv names a folder where TestPageSamples writes each page with
// sample data and style.css. The client's browser checks read them to
// check the pages at phone width (decision 0034).
const samplesEnv = "WATHENA_PAGE_SAMPLES"

// samplePage is a page with every field filled, and long values where a
// phone's narrow screen could overflow.
func samplePage() *page {
	long := "AVeryLongCharacterNameWithoutSpaces"
	return &page{
		Title: "Sample", CSRF: "csrf", Error: "Something went wrong, and this line says what.", Notice: "Done.",
		Account: "alexis_with_a_long_account_name", Factors: "an authenticator app",
		Script: true, Passkey: "register",
		TOTPOn: true, TOTPAvailable: true, PasskeysAvailable: true, RecoveryLeft: 8,
		Passkeys: []passkeyView{{ID: 1, Name: "Phone", Created: "2026-10-01 12:00", LastUsed: "2026-10-14 08:30"}},
		QR:       `<svg class="qr" viewBox="0 0 10 10"></svg>`,
		Key:      "JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP",
		Codes:    []string{"abcd-efgh", "ijkl-mnop", "qrst-uvwx", "yzab-cdef"},
		Characters: []characterView{
			{Slot: 0, Name: "Alice", Job: "Swordman", BaseLevel: 50, JobLevel: 40, Map: "prontera"},
			{Slot: 1, Name: long, Job: "Lord Knight", BaseLevel: 99, JobLevel: 70, Map: "prt_fild08"},
		},
		Guilds:     []guildView{{Slot: 0, Name: "Wolves", Character: "Alice"}},
		FreeSlots:  []int{2, 3},
		Hairs:      []int{1, 2, 3},
		HairColors: []int{0, 1, 2},
		Blocks:     []blockView{{ID: 1, Name: "Alice", Online: true, Names: []string{"Bob", long}}},
		Reports: []reportView{{
			Date: "2026-10-14 12:00", Character: "Alice", Target: long, Kind: "Chat line",
			Reason: "insults and a long reason that goes on for a while", Status: "Action taken",
		}},
		Sanctions: []sanctionView{{
			Kind: "Mute", Who: long, Reason: "spam in #trade", Start: "2026-10-14 12:00",
			End: "2026-10-15 12:00", State: "Active",
		}},
		Recent: true, LastExport: "2026-10-13 10:00", NextExport: "2026-10-14 10:00",
		Deletion: &deletionView{Requested: "2026-10-14", After: "2026-10-28"},
		Contact:  "owner@example.com", OwnerText: []string{"We keep data as the table says."},
		Retention: []retentionRow{
			{Data: "Login records with IP addresses", Kept: "90 days", Notes: "For the safety of accounts."},
		},
		DeletionDays: 14,
	}
}

// TestPageSamples renders every page with sample data. With
// WATHENA_PAGE_SAMPLES set, it writes them there.
func TestPageSamples(t *testing.T) {
	out := os.Getenv(samplesEnv)
	for _, tmpl := range pages.Templates() {
		name := tmpl.Name()
		if !strings.HasSuffix(name, ".html") {
			continue
		}
		var b bytes.Buffer
		if err := pages.ExecuteTemplate(&b, name, samplePage()); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if out == "" {
			continue
		}
		if err := os.WriteFile(filepath.Join(out, name), b.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if out == "" {
		return
	}
	css, err := files.ReadFile("static/style.css")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "style.css"), css, 0o600); err != nil {
		t.Fatal(err)
	}
}
