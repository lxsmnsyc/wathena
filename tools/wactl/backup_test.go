package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"filippo.io/age"
	"filippo.io/age/armor"

	"github.com/lxsmnsyc/wathena/server/store"
)

// fakeDump stands in for pg_dump's custom format.
var fakeDump = append([]byte("PGDMP"), bytes.Repeat([]byte("the world's rows and password hashes "), 5000)...)

func newIdentity(t *testing.T) *age.X25519Identity {
	t.Helper()
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// writeTestBackup writes data as a backup file, encrypted when
// recipients are given, and returns its manifest.
func writeTestBackup(t *testing.T, file string, data []byte, recipients ...age.Recipient) *backupManifest {
	t.Helper()
	size, sum, err := writeAtomic(file, func(w io.Writer) error {
		enc, err := encryptTo(w, recipients)
		if err != nil {
			return err
		}
		if _, err := enc.Write(data); err != nil {
			return err
		}
		return enc.Close()
	})
	if err != nil {
		t.Fatal(err)
	}
	return &backupManifest{Format: manifestFormat, File: filepath.Base(file), Encrypted: len(recipients) > 0, Size: size, SHA256: sum}
}

func readBackup(t *testing.T, file string, ids ...age.Identity) ([]byte, bool, error) {
	t.Helper()
	f, err := os.Open(file)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	src, encrypted, err := openBackup(f, ids)
	if err != nil {
		return nil, encrypted, err
	}
	b, err := io.ReadAll(src)
	return b, encrypted, err
}

// TestBackupEncryptionRoundTrip: a backup written through age holds no
// plain bytes, is readable by its owner only, matches its manifest, and
// decrypts to the stream that went in. Armored age files and plain
// backups open too.
func TestBackupEncryptionRoundTrip(t *testing.T) {
	dir := t.TempDir()
	id := newIdentity(t)
	file := filepath.Join(dir, "world-20261018T030000Z.dump.age")
	m := writeTestBackup(t, file, fakeDump, id.Recipient())
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(raw, ageHeader) || bytes.Contains(raw, []byte("PGDMP")) || bytes.Contains(raw, []byte("password hashes")) {
		t.Errorf("the file is not an age file, or holds plain bytes: %q", raw[:40])
	}
	if st, _ := os.Stat(file); st.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", st.Mode().Perm())
	}
	if err := checkFile(file, m); err != nil || m.Size != int64(len(raw)) {
		t.Errorf("check: %v, size %d of %d", err, m.Size, len(raw))
	}
	got, encrypted, err := readBackup(t, file, id)
	if err != nil || !encrypted || !bytes.Equal(got, fakeDump) {
		t.Errorf("decrypted %d bytes, encrypted %v, %v", len(got), encrypted, err)
	}
	// Two recipients: either identity opens it.
	other := newIdentity(t)
	two := filepath.Join(dir, "two.dump.age")
	writeTestBackup(t, two, fakeDump, id.Recipient(), other.Recipient())
	if got, _, err := readBackup(t, two, other); err != nil || !bytes.Equal(got, fakeDump) {
		t.Errorf("second recipient: %v", err)
	}
	// An armored file, as `age --armor` writes it.
	var armored bytes.Buffer
	aw := armor.NewWriter(&armored)
	enc, _ := age.Encrypt(aw, id.Recipient())
	_, _ = enc.Write(fakeDump)
	_ = enc.Close()
	_ = aw.Close()
	armoredFile := filepath.Join(dir, "armored.dump.age")
	if err := os.WriteFile(armoredFile, armored.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, encrypted, err := readBackup(t, armoredFile, id); err != nil || !encrypted || !bytes.Equal(got, fakeDump) {
		t.Errorf("armored: %v", err)
	}
	// A plain backup opens with or without an identity.
	plain := filepath.Join(dir, "world-20261018T030000Z.dump")
	writeTestBackup(t, plain, fakeDump)
	for _, ids := range [][]age.Identity{nil, {id}} {
		if got, encrypted, err := readBackup(t, plain, ids...); err != nil || encrypted || !bytes.Equal(got, fakeDump) {
			t.Errorf("plain with %d identities: %v", len(ids), err)
		}
	}
}

// TestBackupNeedsRecipient: a backup without a recipient is refused
// unless --plain, and --plain refuses recipients.
func TestBackupNeedsRecipient(t *testing.T) {
	id := newIdentity(t)
	dir := t.TempDir()
	file := func(name, text string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	good := file("good", "# the owner's key\n"+id.Recipient().String()+"\n\n")
	empty := file("empty", "# Put the recipient here.\n")
	secret := file("secret", id.String()+"\n")
	for _, c := range []struct {
		name          string
		values, files []string
		plain         bool
		n             int
		err           string
	}{
		{"none", nil, nil, false, 0, "--plain makes an unencrypted backup"},
		{"plain", nil, nil, true, 0, ""},
		{"plain and a recipient", []string{id.Recipient().String()}, nil, true, 0, "--plain cannot go"},
		{"plain and a file", nil, []string{good}, true, 0, "--plain cannot go"},
		{"a recipient", []string{id.Recipient().String()}, nil, false, 1, ""},
		{"a file and a recipient", []string{id.Recipient().String()}, []string{good}, false, 2, ""},
		{"a bad recipient", []string{"age1nope"}, nil, false, 0, "not an age recipient"},
		{"a file without recipients", nil, []string{empty}, false, 0, "a backup needs --recipient"},
		{"a file without recipients and a good one", nil, []string{empty, good}, false, 1, ""},
		{"an identity as a recipient", nil, []string{secret}, false, 0, "unknown recipient"},
		{"a missing file", nil, []string{filepath.Join(dir, "missing")}, false, 0, "--recipients-file"},
	} {
		recs, err := loadRecipients(c.values, c.files, c.plain)
		if c.err == "" && (err != nil || len(recs) != c.n) || c.err != "" && (err == nil || !strings.Contains(err.Error(), c.err)) {
			t.Errorf("%s: %d recipients, %v", c.name, len(recs), err)
		}
		if err != nil && strings.Contains(err.Error(), "AGE-SECRET-KEY") {
			t.Errorf("%s: the error quotes the identity", c.name)
		}
	}
	// The command refuses before it reads the database URL.
	t.Setenv(migrateEnv, "")
	err := worldBackup([]string{"--dir", t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "--recipient") {
		t.Errorf("world backup without a recipient: %v", err)
	}
	// The schedule refuses too, unless it waits for a recipient.
	none := file("no-recipient", "# no recipient yet\n")
	err = worldBackup([]string{"--dir", t.TempDir(), "--every", "24h", "--recipients-file", none})
	if !errors.Is(err, errNoRecipient) {
		t.Errorf("a schedule without a recipient: %v", err)
	}
	for _, args := range [][]string{
		{"--dir", t.TempDir(), "--wait-for-recipient", "--recipients-file", none},
		{"--dir", t.TempDir(), "--every", "24h", "--wait-for-recipient", "--plain"},
	} {
		if err := worldBackup(args); err == nil || !strings.Contains(err.Error(), "--wait-for-recipient needs --every") {
			t.Errorf("%v: %v", args, err)
		}
	}
}

// TestWaitForRecipient: without a recipient, the schedule logs a warning
// at once and then every interval, makes nothing, and stops cleanly.
func TestWaitForRecipient(t *testing.T) {
	logs := &syncBuffer{}
	log := slog.New(slog.NewTextHandler(logs, nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		waitForRecipient(ctx, log, 10*time.Millisecond)
		close(done)
	}()
	for i := 0; strings.Count(logs.String(), "level=WARN msg=\"backups are off") < 3; i++ {
		if i == 500 {
			t.Fatalf("fewer than 3 warnings:\n%s", logs.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done
	if !strings.Contains(logs.String(), "stopped") {
		t.Errorf("no stop in the log:\n%s", logs.String())
	}
}

// TestRemoveStaleTemps: only hidden temporary files of wactl's pattern
// that are older than the limit are removed.
func TestRemoveStaleTemps(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	old := now.Add(-25 * time.Hour)
	// A name as writeAtomic makes it.
	made, err := os.CreateTemp(dir, ".world-20261018T030000Z.dump.age.*.tmp")
	if err != nil {
		t.Fatal(err)
	}
	_ = made.Close()
	stale := []string{filepath.Base(made.Name()), ".world-20261018T030000Z.json.42.tmp", ".world-20261018T030000Z.dump.7.tmp"}
	kept := []string{
		"world-20261017T030000Z.dump.age",          // a backup
		"world-20261017T030000Z.json",              // its manifest
		".world-20261018T030000Z.dump.age.9.tmp.x", // another suffix
		".world-notes.tmp",                         // not wactl's name
		".other-20261018T030000Z.json.1.tmp",       // not a backup's name
		"world-20261018T030000Z.json.1.tmp",        // not hidden
	}
	for _, name := range append(slices.Clone(stale), kept...) {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	// A fresh file of the pattern, such as a backup in progress, stays.
	fresh := ".world-20261018T040000Z.dump.age.5.tmp"
	if err := os.WriteFile(filepath.Join(dir, fresh), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	// So does a folder of the pattern.
	folder := ".world-20261018T030000Z.json.8.tmp"
	if err := os.Mkdir(filepath.Join(dir, folder), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(dir, folder), old, old); err != nil {
		t.Fatal(err)
	}
	removed, err := removeStaleTemps(dir, now, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(removed)
	slices.Sort(stale)
	if !slices.Equal(removed, stale) {
		t.Errorf("removed %v, want %v", removed, stale)
	}
	for _, name := range append(kept, fresh, folder) {
		if _, err := os.Lstat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if removed, err := removeStaleTemps(filepath.Join(dir, "missing"), now, 24*time.Hour); err != nil || len(removed) != 0 {
		t.Errorf("a missing folder: %v, %v", removed, err)
	}
}

// TestBackupWrongIdentity: an identity of another key cannot open a
// backup, and an encrypted backup without --identity is refused.
func TestBackupWrongIdentity(t *testing.T) {
	file := filepath.Join(t.TempDir(), "world.dump.age")
	writeTestBackup(t, file, fakeDump, newIdentity(t).Recipient())
	if _, _, err := readBackup(t, file, newIdentity(t)); err == nil || !strings.Contains(err.Error(), "another recipient") {
		t.Errorf("wrong identity: %v", err)
	}
	if _, encrypted, err := readBackup(t, file); err == nil || !encrypted || !strings.Contains(err.Error(), "--identity") {
		t.Errorf("no identity: %v", err)
	}
}

// TestBackupManifestMismatch: a file whose SHA-256 or size is not the
// manifest's fails the check, and the manifest round trips.
func TestBackupManifestMismatch(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "world-20261018T030000Z.dump.age")
	m := writeTestBackup(t, file, fakeDump, newIdentity(t).Recipient())
	m.Release, m.Migration, m.Time = "v0.1.0", 35, time.Date(2026, 10, 18, 3, 0, 0, 0, time.UTC)
	m.Counts = store.WorldCounts{Accounts: 2, Characters: 3, ItemSerials: 4, Zeny: 5}
	path := manifestPath(file)
	if err := writeManifest(path, m); err != nil {
		t.Fatal(err)
	}
	back, err := readManifest(path)
	if err != nil || *back != *m {
		t.Fatalf("manifest %+v, %v", back, err)
	}
	if text, _ := os.ReadFile(path); bytes.Contains(text, []byte("AGE-SECRET")) || !bytes.Contains(text, []byte(`"item_serials": 4`)) {
		t.Errorf("manifest text %s", text)
	}
	wrongSum := *m
	wrongSum.SHA256 = strings.Repeat("0", 64)
	if err := checkFile(file, &wrongSum); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Errorf("wrong SHA-256: %v", err)
	}
	wrongSize := *m
	wrongSize.Size++
	if err := checkFile(file, &wrongSize); err == nil || !strings.Contains(err.Error(), "bytes") {
		t.Errorf("wrong size: %v", err)
	}
	// A manifest of another format is refused.
	other := filepath.Join(dir, "other.json")
	if err := os.WriteFile(other, []byte(`{"format": 2}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readManifest(other); err == nil || !strings.Contains(err.Error(), "format 2") {
		t.Errorf("format 2: %v", err)
	}
}

// TestBackupTampered: a changed or cut encrypted file fails the hash
// check, and fails to decrypt even with a manifest that matches it.
func TestBackupTampered(t *testing.T) {
	id := newIdentity(t)
	file := filepath.Join(t.TempDir(), "world.dump.age")
	m := writeTestBackup(t, file, fakeDump, id.Recipient())
	raw, _ := os.ReadFile(file)
	for _, c := range []struct {
		name string
		data []byte
	}{
		{"a byte changed in the body", func() []byte { b := slices.Clone(raw); b[len(b)/2] ^= 1; return b }()},
		{"the last byte changed", func() []byte { b := slices.Clone(raw); b[len(b)-1] ^= 1; return b }()},
		{"the end cut", raw[:len(raw)-100]},
		{"the last chunk cut", raw[:len(raw)-(len(raw)%(64<<10))]},
	} {
		if err := os.WriteFile(file, c.data, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := checkFile(file, m); err == nil {
			t.Errorf("%s: the hash check passed", c.name)
		}
		if _, _, err := readBackup(t, file, id); err == nil {
			t.Errorf("%s: decrypted", c.name)
		}
	}
}

// TestKeepRule: pruning keeps the newest backup of each of the newest
// Daily UTC days, and of each of the newest Weekly ISO weeks.
func TestKeepRule(t *testing.T) {
	at := func(s string) time.Time {
		v, err := time.Parse("2006-01-02 15:04", s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	// daily lists the backups of every day from first to last at 03:00.
	daily := func(first, last string) []time.Time {
		var out []time.Time
		for d := at(first + " 03:00"); !d.After(at(last + " 03:00")); d = d.AddDate(0, 0, 1) {
			out = append(out, d)
		}
		return out
	}
	for _, c := range []struct {
		name  string
		keep  keepRule
		times []time.Time
		want  []string
	}{
		{
			name:  "one backup",
			keep:  keepRule{7, 4},
			times: []time.Time{at("2026-10-18 03:00")},
			want:  []string{"2026-10-18 03:00"},
		},
		{
			// 2026-10-18 is a Sunday. Days 12 to 18 are daily. The weeks
			// before end on Sundays 11, 4 and 27, which are kept as weekly.
			name:  "40 days of daily backups",
			keep:  keepRule{7, 4},
			times: daily("2026-09-09", "2026-10-18"),
			want: []string{"2026-09-27 03:00", "2026-10-04 03:00", "2026-10-11 03:00", "2026-10-12 03:00", "2026-10-13 03:00",
				"2026-10-14 03:00", "2026-10-15 03:00", "2026-10-16 03:00", "2026-10-17 03:00", "2026-10-18 03:00"},
		},
		{
			// On Monday 19, a new week starts. Its daily backup is also its
			// weekly one, and the oldest weekly backup goes.
			name:  "a new week",
			keep:  keepRule{7, 4},
			times: daily("2026-09-09", "2026-10-19"),
			want: []string{"2026-10-04 03:00", "2026-10-11 03:00", "2026-10-13 03:00", "2026-10-14 03:00", "2026-10-15 03:00",
				"2026-10-16 03:00", "2026-10-17 03:00", "2026-10-18 03:00", "2026-10-19 03:00"},
		},
		{
			name: "several a day keep the newest of each day",
			keep: keepRule{2, 0},
			times: []time.Time{at("2026-10-16 23:59"), at("2026-10-17 00:00"), at("2026-10-17 12:00"), at("2026-10-17 23:59"),
				at("2026-10-18 06:00"), at("2026-10-18 00:00")},
			want: []string{"2026-10-17 23:59", "2026-10-18 06:00"},
		},
		{
			// Days without backups do not count: the newest 3 days that
			// have one are kept.
			name:  "gaps",
			keep:  keepRule{3, 0},
			times: []time.Time{at("2026-09-01 03:00"), at("2026-09-20 03:00"), at("2026-10-01 03:00"), at("2026-10-18 03:00")},
			want:  []string{"2026-09-20 03:00", "2026-10-01 03:00", "2026-10-18 03:00"},
		},
		{
			// ISO weeks cross the year: Monday 2026-12-28 to Sunday
			// 2027-01-03 is week 53 of 2026, so 01-03 is its weekly backup.
			// 2026-12-27 is in week 52, the third week, and goes.
			name:  "a week across the new year",
			keep:  keepRule{1, 2},
			times: []time.Time{at("2026-12-27 03:00"), at("2026-12-28 03:00"), at("2026-12-31 03:00"), at("2027-01-03 03:00"), at("2027-01-04 03:00")},
			want:  []string{"2027-01-03 03:00", "2027-01-04 03:00"},
		},
		{
			name:  "weekly only beyond the newest",
			keep:  keepRule{1, 3},
			times: daily("2026-09-28", "2026-10-18"),
			want:  []string{"2026-10-04 03:00", "2026-10-11 03:00", "2026-10-18 03:00"},
		},
	} {
		kept := c.keep.keepTimes(c.times)
		var got []string
		for _, tm := range c.times {
			if kept[tm] {
				got = append(got, tm.Format("2006-01-02 15:04"))
			}
		}
		slices.Sort(got)
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: kept %v, want %v", c.name, got, c.want)
		}
	}
	for _, bad := range []keepRule{{0, 4}, {7, -1}, {10001, 0}} {
		if bad.check() == nil {
			t.Errorf("%+v was taken", bad)
		}
	}
}

// TestPruneDir: pruning removes old backups and their manifests, and
// only files of wactl's names.
func TestPruneDir(t *testing.T) {
	dir := t.TempDir()
	write := func(name string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for d := 1; d <= 10; d++ {
		stamp := time.Date(2026, 10, d, 3, 0, 0, 0, time.UTC).Format(backupStamp)
		write("world-" + stamp + ".dump.age")
		write("world-" + stamp + ".json")
	}
	others := []string{
		"notes.txt", "world-20261001T030000Z.dump.age.bak", "world-2026-10-01.dump",
		".world-20261001T030000Z.dump.age.123.tmp", "world-20261001T030000Z.sql",
	}
	for _, name := range others {
		write(name)
	}
	if err := os.Mkdir(filepath.Join(dir, "world-20261001T040000Z.dump.age"), 0o700); err != nil {
		t.Fatal(err)
	}
	// A plain backup without a manifest counts, and a manifest without a
	// backup goes.
	write("world-20261009T120000Z.dump")
	write("world-20260901T030000Z.json")
	removed, err := pruneDir(dir, keepRule{Daily: 3, Weekly: 0})
	if err != nil {
		t.Fatal(err)
	}
	// Days 1 to 7, the 03:00 backup of day 9 and the lone manifest go.
	if len(removed) != 2*8+1 {
		t.Errorf("removed %d: %v", len(removed), removed)
	}
	entries, _ := os.ReadDir(dir)
	var left []string
	for _, e := range entries {
		left = append(left, e.Name())
	}
	want := append(slices.Clone(others),
		"world-20261001T040000Z.dump.age",
		"world-20261008T030000Z.dump.age", "world-20261008T030000Z.json",
		"world-20261009T120000Z.dump",
		"world-20261010T030000Z.dump.age", "world-20261010T030000Z.json")
	slices.Sort(want)
	if !slices.Equal(left, want) {
		t.Errorf("left %v\nwant %v", left, want)
	}
	sets, _ := scanDir(dir)
	if s, ok := newestBackup(sets, true); !ok || s.dump != "world-20261010T030000Z.dump.age" {
		t.Errorf("newest %+v", s)
	}
}

// TestWriteAtomic: a file appears under its name only when complete.
func TestWriteAtomic(t *testing.T) {
	dir := t.TempDir()
	final := filepath.Join(dir, "world-20261018T030000Z.dump.age")
	listing := func() []string {
		entries, _ := os.ReadDir(dir)
		var out []string
		for _, e := range entries {
			out = append(out, e.Name())
		}
		return out
	}
	// While fill runs, only a hidden temporary file exists.
	var during []string
	failed := errors.New("pg_dump: exit status 1")
	_, _, err := writeAtomic(final, func(w io.Writer) error {
		_, _ = w.Write(bytes.Repeat([]byte("x"), 3<<20))
		during = listing()
		return failed
	})
	if !errors.Is(err, failed) {
		t.Errorf("error %v", err)
	}
	if len(during) != 1 || !strings.HasPrefix(during[0], ".world-20261018T030000Z.dump.age.") || !strings.HasSuffix(during[0], ".tmp") {
		t.Errorf("during the write: %v", during)
	}
	if got := listing(); len(got) != 0 {
		t.Errorf("a failed write left %v", got)
	}
	size, sum, err := writeAtomic(final, func(w io.Writer) error {
		_, err := w.Write([]byte("complete"))
		return err
	})
	want := sha256.Sum256([]byte("complete"))
	if err != nil || size != 8 || sum != hex.EncodeToString(want[:]) {
		t.Fatalf("%d %s %v", size, sum, err)
	}
	if got := listing(); !slices.Equal(got, []string{filepath.Base(final)}) {
		t.Errorf("after the write: %v", got)
	}
	// A file that exists is never replaced.
	if _, _, err := writeAtomic(final, func(w io.Writer) error { _, err := w.Write([]byte("new")); return err }); err == nil {
		t.Error("an existing file was replaced")
	}
	if b, _ := os.ReadFile(final); string(b) != "complete" || len(listing()) != 1 {
		t.Errorf("after a refused write: %q %v", b, listing())
	}
	// publish refuses a name that appeared during the write.
	tmp := filepath.Join(dir, ".tmp")
	_ = os.WriteFile(tmp, []byte("late"), 0o600)
	if err := publish(tmp, final); err == nil {
		t.Error("publish replaced a file")
	}
}

// TestBackupNames: backup and manifest names.
func TestBackupNames(t *testing.T) {
	at := time.Date(2026, 10, 18, 15, 30, 5, 0, time.FixedZone("PHT", 8*3600))
	file, man := backupPaths("/backups", at, true)
	if file != "/backups/world-20261018T073005Z.dump.age" || man != "/backups/world-20261018T073005Z.json" {
		t.Errorf("%s %s", file, man)
	}
	if file, _ := backupPaths("b", at, false); file != filepath.Join("b", "world-20261018T073005Z.dump") {
		t.Errorf("plain %s", file)
	}
	for in, want := range map[string]string{
		"a/world-20261018T073005Z.dump.age": "a/world-20261018T073005Z.json",
		"x.dump":                            "x.json",
		"x.backup":                          "x.backup.json",
	} {
		if got := manifestPath(in); got != want {
			t.Errorf("%s: %s", in, got)
		}
	}
	if u, err := databaseURL("postgres://v:p%40ss@db:5432/postgres?sslmode=disable", "wathena_verify_1"); err != nil ||
		u != "postgres://v:p%40ss@db:5432/wathena_verify_1?sslmode=disable" {
		t.Errorf("%s %v", u, err)
	}
}

// TestBackupKeyNew: the identity file is private, never overwritten, and
// holds the recipient it prints.
func TestBackupKeyNew(t *testing.T) {
	out := filepath.Join(t.TempDir(), "identity.txt")
	if err := backupKeyNew([]string{"--out", out}); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(out)
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("%v %v", st, err)
	}
	ids, err := loadIdentities(out)
	if err != nil || len(ids) != 1 {
		t.Fatal(err)
	}
	text, _ := os.ReadFile(out)
	recipient := ids[0].(*age.X25519Identity).Recipient().String()
	if !strings.Contains(string(text), "# public key: "+recipient+"\n") {
		t.Errorf("file %q", text)
	}
	if err := backupKeyNew([]string{"--out", out}); err == nil {
		t.Error("an identity was overwritten")
	}
	if again, _ := os.ReadFile(out); !bytes.Equal(again, text) {
		t.Error("the identity changed")
	}
}

// TestRestoredProblems: a restored copy must have the manifest's
// migration and counts, and no migration newer than this wactl's.
func TestRestoredProblems(t *testing.T) {
	m := &backupManifest{Migration: 35, Counts: store.WorldCounts{Accounts: 2, Characters: 3, ItemSerials: 4, Zeny: 5}}
	if p := restoredProblems(35, 35, m.Counts, m); len(p) != 0 {
		t.Errorf("%v", p)
	}
	if p := restoredProblems(35, 40, m.Counts, m); len(p) != 0 {
		t.Errorf("an older backup: %v", p)
	}
	p := restoredProblems(36, 35, store.WorldCounts{Accounts: 2, Characters: 2, ItemSerials: 4, Zeny: 6}, m)
	if len(p) != 4 || !strings.Contains(p[0], "newer") || !strings.Contains(p[2], "2 characters, but the manifest says 3") || !strings.Contains(p[3], "zeny") {
		t.Errorf("%v", p)
	}
}

// TestBackupLoop: the schedule makes a backup when none is due later,
// prunes after it, and stops cleanly when its context ends.
func TestBackupLoop(t *testing.T) {
	dir := t.TempDir()
	// The schedule waits on the real clock.
	now := time.Now().UTC().Truncate(time.Second)
	for d := 1; d <= 9; d++ {
		file, man := backupPaths(dir, now.AddDate(0, 0, -d), true)
		_ = os.WriteFile(file, []byte("x"), 0o600)
		_ = os.WriteFile(man, []byte("{}"), 0o600)
	}
	_ = os.WriteFile(filepath.Join(dir, "keep-me.txt"), []byte("x"), 0o600)
	// A crash's temporary file of two days ago.
	crashed := filepath.Join(dir, ".world-20261016T030000Z.dump.age.123.tmp")
	_ = os.WriteFile(crashed, []byte("x"), 0o600)
	_ = os.Chtimes(crashed, now.Add(-48*time.Hour), now.Add(-48*time.Hour))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var runs atomic.Int32
	job := &backupJob{now: func() time.Time { return now }}
	job.fake = func(ctx context.Context, dir string) (*backupManifest, error) {
		runs.Add(1)
		file, man := backupPaths(dir, now, true)
		_ = os.WriteFile(file, []byte("x"), 0o600)
		_ = os.WriteFile(man, []byte("{}"), 0o600)
		return &backupManifest{File: filepath.Base(file), Time: now}, nil
	}
	logs := &syncBuffer{}
	log := slog.New(slog.NewTextHandler(logs, nil))
	done := make(chan error)
	go func() { done <- backupLoop(ctx, log, job, dir, 24*time.Hour, keepRule{Daily: 7, Weekly: 0}) }()
	// Stop it once it waits for the second run.
	for i := 0; strings.Count(logs.String(), "next backup") < 2; i++ {
		if i == 500 {
			t.Fatalf("no second wait:\n%s", logs.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	sets, _ := scanDir(dir)
	if runs.Load() != 1 || len(sets) != 7 {
		t.Errorf("%d runs, %d backups left", runs.Load(), len(sets))
	}
	if _, err := os.Stat(filepath.Join(dir, "keep-me.txt")); err != nil {
		t.Error(err)
	}
	if _, err := os.Lstat(crashed); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the crash's temporary file: %v", err)
	}
	for _, want := range []string{"backup made", "removed an old backup", "removed a temporary file that a crash left", "stopped"} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("no %q in the log:\n%s", want, logs.String())
		}
	}
	// The newest backup is less than a day old, so the next run waits,
	// and a stop during the wait makes none.
	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	if err := backupLoop(ctx, log, job, dir, 48*time.Hour, keepRule{Daily: 7}); err != nil || runs.Load() != 1 {
		t.Errorf("%d runs, %v", runs.Load(), err)
	}
}

// syncBuffer is a log buffer that a test reads while a loop writes.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
