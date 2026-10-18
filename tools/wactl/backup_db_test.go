package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/store/storetest"
)

// backupDatabase makes a migrated test database with some players, and
// returns its URL and pool. It skips without PostgreSQL or its client
// tools.
func backupDatabase(t *testing.T) (string, *pgxpool.Pool) {
	t.Helper()
	for _, tool := range []string{"pg_dump", "pg_restore", "createdb", "dropdb"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " is not installed")
		}
	}
	pool := storetest.Migrated(t)
	url, err := databaseURL(os.Getenv(storetest.Env), pool.Config().ConnConfig.Database)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for a, name := range []string{"Alpha", "Bravo"} {
		var account int64
		if err := pool.QueryRow(ctx, "INSERT INTO accounts (name) VALUES ($1) RETURNING id", name).Scan(&account); err != nil {
			t.Fatal(err)
		}
		for slot := range a + 1 {
			var char int64
			if err := pool.QueryRow(ctx, `INSERT INTO characters (account_id, slot, name, job, sex, base_level, job_level,
				str, agi, vit, int, dex, luk, hp, sp, map, x, y, zeny)
				VALUES ($1, $2, $3, 0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 40, 11, 'map/one', 5, 5, $4) RETURNING id`,
				account, slot, name+string(rune('A'+slot)), 100*(a+slot+1)).Scan(&char); err != nil {
				t.Fatal(err)
			}
			for i := range 2 {
				if _, err := pool.Exec(ctx, "INSERT INTO inventory (character_id, slot, item_id, amount) VALUES ($1, $2, 501, 3)", char, i); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	return url, pool
}

// verifyDatabases counts the temporary databases of verify-backup.
func verifyDatabases(t *testing.T) int {
	t.Helper()
	ctx := context.Background()
	c, err := pgx.Connect(ctx, os.Getenv(storetest.Env))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close(ctx) }()
	var n int
	if err := c.QueryRow(ctx, "SELECT count(*) FROM pg_database WHERE datname LIKE 'wathena\\_verify\\_%'").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestBackupVerifyRestoreWithDatabase: an encrypted backup of a world
// records its counts, verifies in a temporary database that is dropped
// after, and restores into an empty database with the same rows. A wrong
// identity restores nothing, and a plain backup still restores.
func TestBackupVerifyRestoreWithDatabase(t *testing.T) {
	url, pool := backupDatabase(t)
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "backups")
	identity := filepath.Join(t.TempDir(), "identity.txt")
	if err := backupKeyNew([]string{"--out", identity}); err != nil {
		t.Fatal(err)
	}
	text, _ := os.ReadFile(identity)
	_, after, _ := strings.Cut(string(text), "# public key: ")
	recipient, _, _ := strings.Cut(after, "\n")
	recipients := filepath.Join(t.TempDir(), "recipients")
	if err := os.WriteFile(recipients, []byte("# the owner\n"+recipient+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := worldBackup([]string{"--database", url, "--dir", dir, "--recipients-file", recipients}); err != nil {
		t.Fatal(err)
	}
	file, manifest, err := latestVerifiable(dir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := readManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	want := store.WorldCounts{Accounts: 2, Characters: 3, ItemSerials: 6, Zeny: 100 + 200 + 300}
	mig, _ := store.NewMigrator(pool)
	_, latest, _ := mig.Versions(ctx)
	_ = mig.Close()
	if m.Counts != want || !m.Encrypted || m.Migration != latest || m.Release == "" || m.File != filepath.Base(file) ||
		time.Since(m.Time) > time.Minute || !strings.HasSuffix(file, ".dump.age") {
		t.Errorf("manifest %+v", m)
	}
	if st, _ := os.Stat(file); st.Mode().Perm() != 0o600 || st.Size() != m.Size {
		t.Errorf("file %v", st)
	}
	before := verifyDatabases(t)
	t.Setenv(verifyEnv, os.Getenv(storetest.Env))
	if err := worldVerifyBackup([]string{"--dir", dir, "--identity", identity}); err != nil {
		t.Fatal(err)
	}
	if n := verifyDatabases(t); n != before {
		t.Errorf("%d temporary databases left", n-before)
	}
	// Without the identity, verify fails before it makes a database.
	if err := worldVerifyBackup([]string{"--in", file}); err == nil || !strings.Contains(err.Error(), "--identity") {
		t.Errorf("verify without an identity: %v", err)
	}

	// Restore into an empty database.
	empty := storetest.Empty(t)
	target, _ := databaseURL(url, empty.Config().ConnConfig.Database)
	other := filepath.Join(t.TempDir(), "other.txt")
	if err := backupKeyNew([]string{"--out", other}); err != nil {
		t.Fatal(err)
	}
	if err := worldRestore([]string{"--database", target, "--in", file, "--identity", other}); err == nil || !strings.Contains(err.Error(), "another recipient") {
		t.Errorf("restore with another identity: %v", err)
	}
	if err := worldRestore([]string{"--database", target, "--in", file}); err == nil || !strings.Contains(err.Error(), "--identity") {
		t.Errorf("restore without an identity: %v", err)
	}
	// A changed byte in the middle fails the decryption while
	// pg_restore runs. It is killed, so its transaction never commits.
	raw, _ := os.ReadFile(file)
	raw[len(raw)/2] ^= 1
	tampered := filepath.Join(t.TempDir(), "tampered.dump.age")
	if err := os.WriteFile(tampered, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := worldRestore([]string{"--database", target, "--in", tampered, "--identity", identity}); err == nil || !strings.Contains(err.Error(), "nothing was restored") {
		t.Errorf("restore a tampered backup: %v", err)
	}
	if has, err := store.HasSchema(ctx, empty); err != nil || has {
		t.Fatalf("a refused restore changed the database: %v %v", has, err)
	}
	if err := worldRestore([]string{"--database", target, "--in", file, "--identity", identity}); err != nil {
		t.Fatal(err)
	}
	if got, err := store.CountWorld(ctx, empty); err != nil || got != want {
		t.Errorf("restored %+v, %v", got, err)
	}
	if err := worldRestore([]string{"--database", target, "--in", file, "--identity", identity}); err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Errorf("a second restore: %v", err)
	}

	// A plain backup, as older releases made, restores without an
	// identity and verifies.
	plain := filepath.Join(t.TempDir(), "world.dump")
	if err := worldBackup([]string{"--database", url, "--out", plain, "--plain"}); err != nil {
		t.Fatal(err)
	}
	if err := worldVerifyBackup([]string{"--in", plain}); err != nil {
		t.Errorf("verify a plain backup: %v", err)
	}
	second := storetest.Empty(t)
	target, _ = databaseURL(url, second.Config().ConnConfig.Database)
	if err := worldRestore([]string{"--database", target, "--in", plain}); err != nil {
		t.Fatal(err)
	}
	if got, err := store.CountWorld(ctx, second); err != nil || got != want {
		t.Errorf("restored a plain backup: %+v, %v", got, err)
	}
}

// TestVerifyBackupMismatchWithDatabase: verify fails on counts or a
// migration that differ from the manifest, and on a manifest hash that
// does not match, and drops its temporary database each time.
func TestVerifyBackupMismatchWithDatabase(t *testing.T) {
	url, _ := backupDatabase(t)
	dir := t.TempDir()
	if err := worldBackup([]string{"--database", url, "--dir", dir, "--plain"}); err != nil {
		t.Fatal(err)
	}
	file, manifest, err := latestVerifiable(dir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := readManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(verifyEnv, os.Getenv(storetest.Env))
	before := verifyDatabases(t)
	for _, c := range []struct {
		name   string
		change func(*backupManifest)
		want   string
	}{
		{"one character more", func(m *backupManifest) { m.Counts.Characters++ }, "3 characters, but the manifest says 4"},
		{"less zeny", func(m *backupManifest) { m.Counts.Zeny -= 100 }, "600 zeny, but the manifest says 500"},
		{"another migration", func(m *backupManifest) { m.Migration-- }, "but the manifest says"},
		{"another hash", func(m *backupManifest) { m.SHA256 = strings.Repeat("ab", 32) }, "SHA-256"},
	} {
		changed := *m
		c.change(&changed)
		b, _ := json.Marshal(&changed)
		if err := os.WriteFile(manifest, b, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := worldVerifyBackup([]string{"--in", file}); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v", c.name, err)
		}
		if n := verifyDatabases(t); n != before {
			t.Errorf("%s: %d temporary databases left", c.name, n-before)
		}
	}
}
