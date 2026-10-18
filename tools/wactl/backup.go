package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"time"

	"filippo.io/age"
	"filippo.io/age/armor"

	"github.com/lxsmnsyc/wathena/internal/buildinfo"
	"github.com/lxsmnsyc/wathena/internal/secretenv"
	"github.com/lxsmnsyc/wathena/server/store"
)

// Backups (decision 0048):
//   - A backup is pg_dump's custom format, encrypted with age to one or
//     more recipients as it streams, so no plain copy touches disk.
//   - In a folder, a backup is world-<UTC time>.dump.age, or .dump with
//     --plain, with a plain manifest world-<UTC time>.json beside it.
//   - Files are written to a hidden temporary name and renamed when they
//     are complete, so a crash never leaves a file that looks finished.

const (
	// backupStamp is the UTC time in backup names, such as
	// 20261018T153000Z. Names sort by time.
	backupStamp = "20060102T150405Z"
	ageExt      = ".dump.age"
	plainExt    = ".dump"
	manifestExt = ".json"
	// manifestFormat is the version of the manifest's fields and of the
	// counts' definitions in store.WorldCounts.
	manifestFormat = 1
	// verifyEnv holds the URL of the role that verify-backup restores
	// with. It needs CREATEDB and no rights on the world's database.
	verifyEnv = "WATHENA_VERIFY_DATABASE_URL"
	// retryAfter is the wait after a failed scheduled run, when it is
	// shorter than --every.
	retryAfter = time.Hour
	// noRecipientWarnEvery is how often --wait-for-recipient logs that
	// backups are off.
	noRecipientWarnEvery = time.Hour
	// staleTempAge is the age after which a scheduled run removes a
	// hidden temporary file that a crash left.
	staleTempAge = 24 * time.Hour
)

// backupName matches the files that wactl makes in a backup folder.
// Pruning only touches files that match it.
var backupName = regexp.MustCompile(`^world-(\d{8}T\d{6}Z)\.(dump\.age|dump|json)$`)

// tempName matches the hidden temporary files of writeAtomic for those
// names, such as .world-20261018T030000Z.dump.age.123456.tmp.
var tempName = regexp.MustCompile(`^\.world-\d{8}T\d{6}Z\.(dump\.age|dump|json)\.\d+\.tmp$`)

// errNoRecipient is the refusal of a backup without a recipient.
var errNoRecipient = errors.New("a backup needs --recipient or --recipients-file, the public key of `wactl world backup-key new`; --plain makes an unencrypted backup")

// ageHeader starts every binary age file, and ageArmor every armored one.
var (
	ageHeader = []byte("age-encryption.org/")
	ageArmor  = []byte(armor.Header)
)

// backupManifest is the plain file beside a backup. It holds no secrets.
type backupManifest struct {
	Format    int               `json:"format"`
	File      string            `json:"file"`
	Encrypted bool              `json:"encrypted"`
	Release   string            `json:"release"`
	Migration int64             `json:"migration"`
	Time      time.Time         `json:"time"`
	Size      int64             `json:"size"`
	SHA256    string            `json:"sha256"`
	Counts    store.WorldCounts `json:"counts"`
}

// listFlag collects a repeated flag.
type listFlag []string

func (l *listFlag) String() string     { return strings.Join(*l, ",") }
func (l *listFlag) Set(v string) error { *l = append(*l, v); return nil }

// backupKeyNew writes a new X25519 identity in the format of age-keygen,
// readable by its owner only, and prints its recipient.
func backupKeyNew(args []string) error {
	fs := newFlags("world backup-key new")
	out := fs.String("out", "", "the identity file to make; it must not exist")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return errors.New("--out is required: the identity file to make")
	}
	id, err := age.GenerateX25519Identity()
	if err != nil {
		return err
	}
	text := identityText(id, time.Now())
	f, err := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) // #nosec G304 -- the file the operator named.
	if err != nil {
		return fmt.Errorf("--out: %w", err)
	}
	_, err = f.WriteString(text)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(*out)
		return err
	}
	fmt.Println(id.Recipient().String())
	fmt.Fprintf(os.Stderr, "wrote the identity to %s. Keep it offline: it decrypts every backup made for the recipient above.\n", *out)
	return nil
}

// identityText is an identity file as age-keygen writes it.
func identityText(id *age.X25519Identity, now time.Time) string {
	return "# created: " + now.UTC().Format(time.RFC3339) + "\n" +
		"# public key: " + id.Recipient().String() + "\n" +
		id.String() + "\n"
}

// loadRecipients reads the recipients of --recipient and
// --recipients-file. A backup without one is refused unless plain.
func loadRecipients(values, files []string, plain bool) ([]age.Recipient, error) {
	if plain {
		if len(values) > 0 || len(files) > 0 {
			return nil, errors.New("--plain cannot go with --recipient or --recipients-file")
		}
		return nil, nil
	}
	var out []age.Recipient
	for _, v := range values {
		r, err := age.ParseRecipients(strings.NewReader(v))
		if err != nil {
			return nil, errors.New("--recipient: not an age recipient; it starts with age1")
		}
		out = append(out, r...)
	}
	for _, path := range files {
		b, err := os.ReadFile(path) // #nosec G304 -- the file the operator named.
		if err != nil {
			return nil, fmt.Errorf("--recipients-file: %w", err)
		}
		// A file of comments and blank lines has no recipient yet, as
		// secrets.sh makes it. age's parser refuses such a file.
		if !hasRecipientLine(b) {
			continue
		}
		r, err := age.ParseRecipients(bytes.NewReader(b))
		if err != nil {
			return nil, fmt.Errorf("--recipients-file %s: %w", path, err)
		}
		out = append(out, r...)
	}
	if len(out) == 0 {
		return nil, errNoRecipient
	}
	return out, nil
}

// hasRecipientLine reports whether a recipients file has a line that
// is not blank and not a # comment.
func hasRecipientLine(b []byte) bool {
	for line := range strings.Lines(string(b)) {
		if l := strings.TrimSpace(line); l != "" && !strings.HasPrefix(l, "#") {
			return true
		}
	}
	return false
}

// loadIdentities reads an identity file, as backup-key new or age-keygen
// writes it.
func loadIdentities(path string) ([]age.Identity, error) {
	f, err := os.Open(path) // #nosec G304 -- the file the operator named.
	if err != nil {
		return nil, fmt.Errorf("--identity: %w", err)
	}
	defer func() { _ = f.Close() }()
	ids, err := age.ParseIdentities(f)
	if err != nil {
		return nil, fmt.Errorf("--identity %s: %w", path, err)
	}
	return ids, nil
}

// backupPaths names a backup and its manifest in a folder.
func backupPaths(dir string, at time.Time, encrypted bool) (file, manifest string) {
	base := "world-" + at.UTC().Format(backupStamp)
	ext := ageExt
	if !encrypted {
		ext = plainExt
	}
	return filepath.Join(dir, base+ext), filepath.Join(dir, base+manifestExt)
}

// manifestPath is the manifest beside a backup file: the name without
// .dump.age or .dump, with .json.
func manifestPath(file string) string {
	for _, ext := range []string{ageExt, plainExt} {
		if s, ok := strings.CutSuffix(file, ext); ok {
			return s + manifestExt
		}
	}
	return file + manifestExt
}

// encryptTo returns the writer that a backup's plain stream goes into:
// age's encrypting writer, or w itself for a plain backup. Close it to
// finish the age stream.
func encryptTo(w io.Writer, recipients []age.Recipient) (io.WriteCloser, error) {
	if len(recipients) == 0 {
		return nopCloser{w}, nil
	}
	return age.Encrypt(w, recipients...)
}

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }

// writeAtomic makes final with what fill writes:
//   - It refuses a final that exists.
//   - fill writes into a hidden temporary file in final's folder, mode
//     0600, which is synced, then linked or renamed to final.
//   - On any error the temporary file is removed and final is not made.
//
// It returns the file's size and SHA-256.
func writeAtomic(final string, fill func(io.Writer) error) (size int64, sum string, err error) {
	if _, err := os.Lstat(final); err == nil {
		return 0, "", fmt.Errorf("%s exists", final)
	} else if !errors.Is(err, os.ErrNotExist) {
		return 0, "", err
	}
	dir, base := filepath.Split(final)
	if dir == "" {
		dir = "."
	}
	tmp, err := os.CreateTemp(dir, "."+base+".*.tmp")
	if err != nil {
		return 0, "", err
	}
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()
	h := sha256.New()
	counter := &countWriter{}
	bw := bufio.NewWriterSize(io.MultiWriter(tmp, h, counter), 1<<20)
	if err = fill(bw); err != nil {
		return 0, "", err
	}
	if err = bw.Flush(); err != nil {
		return 0, "", err
	}
	if err = tmp.Sync(); err != nil {
		return 0, "", err
	}
	if err = tmp.Close(); err != nil {
		return 0, "", err
	}
	if err = publish(tmp.Name(), final); err != nil {
		return 0, "", err
	}
	syncDir(dir)
	return counter.n, hex.EncodeToString(h.Sum(nil)), nil
}

// publish gives a complete temporary file its final name. A hard link
// never replaces a file. Where links do not work, a rename follows a
// check that final does not exist.
func publish(tmp, final string) error {
	if err := os.Link(tmp, final); err == nil {
		return os.Remove(tmp)
	} else if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("%s exists", final)
	}
	if _, err := os.Lstat(final); err == nil {
		return fmt.Errorf("%s exists", final)
	}
	return os.Rename(tmp, final)
}

// syncDir makes a new name in dir last across a crash, where the system
// supports it.
func syncDir(dir string) {
	d, err := os.Open(dir) // #nosec G304 -- the folder of a file wactl made.
	if err != nil {
		return
	}
	_ = d.Sync()
	_ = d.Close()
}

type countWriter struct{ n int64 }

func (c *countWriter) Write(p []byte) (int, error) { c.n += int64(len(p)); return len(p), nil }

// writeManifest writes a manifest with writeAtomic.
func writeManifest(path string, m *backupManifest) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	_, _, err = writeAtomic(path, func(w io.Writer) error {
		_, err := w.Write(append(b, '\n'))
		return err
	})
	return err
}

// readManifest reads a manifest and checks its format.
func readManifest(path string) (*backupManifest, error) {
	b, err := os.ReadFile(path) // #nosec G304 -- the manifest beside the backup the operator named.
	if err != nil {
		return nil, fmt.Errorf("manifest: %w", err)
	}
	var m backupManifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("manifest %s: %w", path, err)
	}
	if m.Format != manifestFormat {
		return nil, fmt.Errorf("manifest %s: format %d, but this wactl reads format %d", path, m.Format, manifestFormat)
	}
	return &m, nil
}

// checkFile compares a backup file's size and SHA-256 with its manifest.
func checkFile(path string, m *backupManifest) error {
	f, err := os.Open(path) // #nosec G304 -- the backup the operator named.
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return err
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if sum != m.SHA256 {
		return fmt.Errorf("%s: SHA-256 %s, but the manifest says %s", path, sum, m.SHA256)
	}
	if n != m.Size {
		return fmt.Errorf("%s: %d bytes, but the manifest says %d", path, n, m.Size)
	}
	return nil
}

// openBackup returns a backup's plain pg_dump stream. An age file, binary
// or armored, needs identities. Other input is taken as a plain backup.
func openBackup(r io.Reader, ids []age.Identity) (io.Reader, bool, error) {
	br := bufio.NewReaderSize(r, 1<<16)
	head, err := br.Peek(len(ageArmor))
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, bufio.ErrBufferFull) {
		return nil, false, err
	}
	var src io.Reader
	switch {
	case bytes.HasPrefix(head, ageHeader):
		src = br
	case bytes.HasPrefix(head, ageArmor):
		src = armor.NewReader(br)
	default:
		return br, false, nil
	}
	if len(ids) == 0 {
		return nil, true, errors.New("the backup is encrypted with age; give --identity")
	}
	plain, err := age.Decrypt(src, ids...)
	if err != nil {
		var none *age.NoIdentityMatchError
		if errors.As(err, &none) {
			return nil, true, errors.New("--identity does not open this backup: it was made for another recipient")
		}
		return nil, true, fmt.Errorf("decrypting the backup: %w", err)
	}
	return plain, true, nil
}

// backupJob makes backups of one database.
type backupJob struct {
	dsn        string
	recipients []age.Recipient
	now        func() time.Time
	// fake replaces dumpDir in tests of the schedule.
	fake func(ctx context.Context, dir string) (*backupManifest, error)
}

// dump writes the database into a backup file and its manifest. The
// counts and pg_dump share one snapshot, so they agree while the game
// writes. A manifest path of "" writes none.
func (j *backupJob) dump(ctx context.Context, file, manifestFile string, at time.Time) (*backupManifest, error) {
	if manifestFile != "" {
		if _, err := os.Lstat(manifestFile); err == nil {
			return nil, fmt.Errorf("%s exists", manifestFile)
		}
	}
	pool, err := connect(ctx, j.dsn)
	if err != nil {
		return nil, err
	}
	defer pool.Close()
	m, err := store.NewMigrator(pool)
	if err != nil {
		return nil, err
	}
	migration, _, err := m.Versions(ctx)
	_ = m.Close()
	if err != nil {
		return nil, err
	}
	snap, err := store.ExportSnapshot(ctx, pool)
	if err != nil {
		return nil, err
	}
	defer func() { _ = snap.Close(context.WithoutCancel(ctx)) }()
	fill := func(w io.Writer) error {
		enc, err := encryptTo(w, j.recipients)
		if err != nil {
			return err
		}
		cmd, err := pgCommand(ctx, "pg_dump", "--dbname=", j.dsn, "--format=custom", "--no-password", "--snapshot="+snap.ID)
		if err != nil {
			return err
		}
		cmd.Stdout = enc
		if err := runPG(cmd); err != nil {
			return err
		}
		return enc.Close()
	}
	man := &backupManifest{
		Format: manifestFormat, File: filepath.Base(file), Encrypted: len(j.recipients) > 0,
		Release: buildinfo.Version(buildVersion), Migration: migration, Time: at, Counts: snap.Counts,
	}
	if file == "-" {
		man.File = ""
		w := bufio.NewWriterSize(os.Stdout, 1<<20)
		if err := fill(w); err != nil {
			return nil, err
		}
		return man, w.Flush()
	}
	if man.Size, man.SHA256, err = writeAtomic(file, fill); err != nil {
		return nil, err
	}
	if manifestFile != "" {
		if err := writeManifest(manifestFile, man); err != nil {
			return nil, err
		}
	}
	return man, nil
}

// dumpDir makes a backup in a folder, named by the time.
func (j *backupJob) dumpDir(ctx context.Context, dir string) (*backupManifest, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	at := j.now().UTC().Truncate(time.Second)
	file, manifest := backupPaths(dir, at, len(j.recipients) > 0)
	return j.dump(ctx, file, manifest, at)
}

// worldBackup writes the whole database with pg_dump, in its custom
// format, encrypted with age unless --plain. The backup holds password
// hashes, sealed TOTP secrets and the world seed.
func worldBackup(args []string) error {
	fs := newFlags("world backup")
	database := fs.String("database", "", "PostgreSQL URL of the migrator role; prefer the "+migrateEnv+" environment variable")
	out := fs.String("out", "", "the backup file to make, or - for standard output; a file gets a manifest beside it")
	dir := fs.String("dir", "", "a folder for backups named world-<UTC time>.dump.age, each with a manifest world-<UTC time>.json")
	var recipients, recipientFiles listFlag
	fs.Var(&recipients, "recipient", "an age recipient (age1...) to encrypt to; repeat for more")
	fs.Var(&recipientFiles, "recipients-file", "a file of age recipients, one per line, # for comments; repeat for more")
	plain := fs.Bool("plain", false, "make an unencrypted backup")
	every := fs.Duration("every", 0, "with --dir, run as a long-lived process and make a backup this often, such as 24h")
	keepDaily := fs.Int("keep-daily", 7, "with --dir, keep the newest backup of each of this many UTC days")
	keepWeekly := fs.Int("keep-weekly", 4, "with --dir, keep the newest backup of each of this many ISO weeks (Monday to Sunday, UTC)")
	waitRecipient := fs.Bool("wait-for-recipient", false, "with --every, wait and log a warning every hour while the recipients files have no recipient, instead of failing; restart to read them again")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	explicit := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { explicit[f.Name] = true })
	if (*out == "") == (*dir == "") {
		return errors.New("give one of --out (a file, or - for standard output) and --dir (a backup folder)")
	}
	if *dir == "" && (*every != 0 || explicit["keep-daily"] || explicit["keep-weekly"]) {
		return errors.New("--every, --keep-daily and --keep-weekly need --dir")
	}
	if *every != 0 && *every < time.Minute {
		return errors.New("--every must be at least 1m")
	}
	if *waitRecipient && (*every == 0 || *plain) {
		return errors.New("--wait-for-recipient needs --every, and cannot go with --plain")
	}
	keep := keepRule{Daily: *keepDaily, Weekly: *keepWeekly}
	if err := keep.check(); err != nil {
		return err
	}
	recs, err := loadRecipients(recipients, recipientFiles, *plain)
	if errors.Is(err, errNoRecipient) && *waitRecipient {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		waitForRecipient(ctx, slog.New(slog.NewTextHandler(os.Stderr, nil)), noRecipientWarnEvery)
		return nil
	}
	if err != nil {
		return err
	}
	dsn, err := migratorURL(*database)
	if err != nil {
		return err
	}
	job := &backupJob{dsn: dsn, recipients: recs, now: time.Now}
	if *every != 0 {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		log := slog.New(slog.NewTextHandler(os.Stderr, nil))
		return backupLoop(ctx, log, job, *dir, *every, keep)
	}
	ctx, cancel := commandContext(24 * time.Hour)
	defer cancel()
	if *dir != "" {
		m, err := job.dumpDir(ctx, *dir)
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, backupLine(m))
		if explicit["keep-daily"] || explicit["keep-weekly"] {
			removed, err := pruneDir(*dir, keep)
			for _, name := range removed {
				fmt.Fprintln(os.Stderr, "removed", name)
			}
			return err
		}
		return nil
	}
	manifest := ""
	if *out != "-" {
		manifest = manifestPath(*out)
	}
	m, err := job.dump(ctx, *out, manifest, time.Now().UTC().Truncate(time.Second))
	if err != nil {
		return err
	}
	if *out != "-" {
		fmt.Fprintln(os.Stderr, backupLine(m))
	}
	return nil
}

// backupLine reports a backup.
func backupLine(m *backupManifest) string {
	return fmt.Sprintf("wrote %s, %.1f MB, migration %d, %d accounts, %d characters, %d item serials, %d zeny",
		m.File, float64(m.Size)/(1<<20), m.Migration, m.Counts.Accounts, m.Counts.Characters, m.Counts.ItemSerials, m.Counts.Zeny)
}

// waitForRecipient is the backup schedule without a recipient, with
// --wait-for-recipient. It makes no backup. It logs a warning at once
// and then every interval until ctx ends. The recipients are read only
// at start, so the owner restarts the service after adding one.
func waitForRecipient(ctx context.Context, log *slog.Logger, every time.Duration) {
	for {
		log.Warn("backups are off: the recipients files have no recipient. Add the public key of `wactl world backup-key new`, then restart this service")
		timer := time.NewTimer(every)
		select {
		case <-ctx.Done():
			timer.Stop()
			log.Info("stopped")
			return
		case <-timer.C:
		}
	}
}

// removeStaleTemps removes the hidden temporary files in dir that match
// tempName and were last changed more than maxAge before now. A crash
// or SIGKILL during a backup leaves them. Other files stay. It returns
// the names it removed.
func removeStaleTemps(dir string, now time.Time, maxAge time.Duration) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var removed []string
	var errs []error
	for _, e := range entries {
		if !e.Type().IsRegular() || !tempName.MatchString(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				errs = append(errs, err)
			}
			continue
		}
		if now.Sub(info.ModTime()) <= maxAge {
			continue
		}
		if err := os.Remove(filepath.Join(dir, e.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
			continue
		}
		removed = append(removed, e.Name())
	}
	return removed, errors.Join(errs...)
}

// backupLoop makes a backup in dir every interval until ctx ends:
//   - The first run is due one interval after the newest backup in dir,
//     or now, so a restart does not make an extra backup.
//   - Before each backup it removes hidden temporary files older than
//     a day, which a crash left.
//   - After each backup it prunes dir by keep.
//   - A failed run is logged and tried again after an hour, or after the
//     interval when that is shorter.
//   - SIGTERM stops it. A backup in progress is stopped and its
//     temporary file removed, and the loop returns nil.
func backupLoop(ctx context.Context, log *slog.Logger, job *backupJob, dir string, every time.Duration, keep keepRule) error {
	next := job.now()
	if sets, err := scanDir(dir); err != nil {
		return err
	} else if latest, ok := newestBackup(sets, false); ok && latest.at.Add(every).After(next) {
		next = latest.at.Add(every)
	}
	log.Info("backup schedule", "dir", dir, "every", every.String(), "keep_daily", keep.Daily, "keep_weekly", keep.Weekly,
		"encrypted", len(job.recipients) > 0)
	for {
		log.Info("next backup", "at", next.UTC().Format(time.RFC3339))
		if !sleepUntil(ctx, next) {
			log.Info("stopped")
			return nil
		}
		start := job.now()
		stale, err := removeStaleTemps(dir, start, staleTempAge)
		for _, name := range stale {
			log.Info("removed a temporary file that a crash left", "file", name)
		}
		if err != nil {
			log.Error("removing old temporary files failed", "error", err.Error())
		}
		dump := job.dumpDir
		if job.fake != nil {
			dump = job.fake
		}
		m, err := dump(ctx, dir)
		if ctx.Err() != nil {
			log.Info("stopped during a backup; no file was made")
			return nil
		}
		if err != nil {
			log.Error("backup failed", "error", err.Error())
			next = start.Add(min(every, retryAfter))
			continue
		}
		log.Info("backup made", "file", m.File, "bytes", m.Size, "seconds", int(job.now().Sub(start).Seconds()),
			"migration", m.Migration, "accounts", m.Counts.Accounts, "characters", m.Counts.Characters,
			"item_serials", m.Counts.ItemSerials, "zeny", m.Counts.Zeny)
		removed, err := pruneDir(dir, keep)
		for _, name := range removed {
			log.Info("removed an old backup", "file", name)
		}
		if err != nil {
			log.Error("pruning failed", "error", err.Error())
		}
		next = m.Time.Add(every)
	}
}

// sleepUntil waits until t, and reports false when ctx ends first.
func sleepUntil(ctx context.Context, t time.Time) bool {
	d := time.Until(t)
	if d <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// keepRule is how many backups pruning keeps:
//   - Daily: for each of the newest Daily UTC days that have a backup,
//     the newest backup of that day.
//   - Weekly: for each of the newest Weekly ISO weeks (Monday to Sunday,
//     UTC) that have a backup, the newest backup of that week.
//
// A backup may count for both. Every other backup is removed. The
// newest backup is always kept, since Daily is at least 1.
type keepRule struct{ Daily, Weekly int }

func (k keepRule) check() error {
	if k.Daily < 1 || k.Daily > 10000 {
		return errors.New("--keep-daily must be 1 to 10000")
	}
	if k.Weekly < 0 || k.Weekly > 10000 {
		return errors.New("--keep-weekly must be 0 to 10000")
	}
	return nil
}

// keepTimes returns the backup times that k keeps.
func (k keepRule) keepTimes(times []time.Time) map[time.Time]bool {
	sorted := slices.Clone(times)
	slices.SortFunc(sorted, func(a, b time.Time) int { return b.Compare(a) })
	kept := map[time.Time]bool{}
	days, weeks := map[string]bool{}, map[string]bool{}
	for _, t := range sorted {
		t = t.UTC()
		if day := t.Format(time.DateOnly); !days[day] && len(days) < k.Daily {
			days[day] = true
			kept[t] = true
		}
		y, w := t.ISOWeek()
		if week := fmt.Sprintf("%d-W%02d", y, w); !weeks[week] && len(weeks) < k.Weekly {
			weeks[week] = true
			kept[t] = true
		}
	}
	return kept
}

// backupSet is the files of one backup time in a folder.
type backupSet struct {
	at       time.Time
	dump     string // the .dump.age or .dump file, or ""
	manifest string // the .json file, or ""
}

// scanDir lists the backups in dir, by the names of backupName only. A
// folder that does not exist has none.
func scanDir(dir string) ([]backupSet, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	byTime := map[time.Time]*backupSet{}
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		parts := backupName.FindStringSubmatch(e.Name())
		if parts == nil {
			continue
		}
		at, err := time.Parse(backupStamp, parts[1])
		if err != nil {
			continue
		}
		s := byTime[at]
		if s == nil {
			s = &backupSet{at: at}
			byTime[at] = s
		}
		if parts[2] == "json" {
			s.manifest = e.Name()
		} else {
			s.dump = e.Name()
		}
	}
	out := make([]backupSet, 0, len(byTime))
	for _, s := range byTime {
		out = append(out, *s)
	}
	slices.SortFunc(out, func(a, b backupSet) int { return a.at.Compare(b.at) })
	return out, nil
}

// newestBackup returns the newest set with a backup file, and with a
// manifest too when needManifest.
func newestBackup(sets []backupSet, needManifest bool) (backupSet, bool) {
	for i := len(sets) - 1; i >= 0; i-- {
		if sets[i].dump != "" && (!needManifest || sets[i].manifest != "") {
			return sets[i], true
		}
	}
	return backupSet{}, false
}

// pruneDir removes the backups of dir that keep does not keep, with
// their manifests, and returns the names it removed. Only backups count
// for keep. A manifest whose backup is gone is removed too. Files that
// do not match backupName stay.
func pruneDir(dir string, keep keepRule) ([]string, error) {
	sets, err := scanDir(dir)
	if err != nil {
		return nil, err
	}
	var times []time.Time
	for _, s := range sets {
		if s.dump != "" {
			times = append(times, s.at)
		}
	}
	kept := keep.keepTimes(times)
	var removed []string
	var errs []error
	for _, s := range sets {
		if kept[s.at] {
			continue
		}
		for _, name := range []string{s.dump, s.manifest} {
			if name == "" {
				continue
			}
			if err := os.Remove(filepath.Join(dir, name)); err != nil {
				errs = append(errs, err)
				continue
			}
			removed = append(removed, name)
		}
	}
	return removed, errors.Join(errs...)
}

// worldRestore loads a backup into an empty database that roles.sql
// made, in one transaction. An age backup needs --identity. A backup
// from an older release then needs `wactl migrate up`.
func worldRestore(args []string) error {
	fs := newFlags("world restore")
	database := fs.String("database", "", "PostgreSQL URL of the migrator role; prefer the "+migrateEnv+" environment variable")
	in := fs.String("in", "", "the backup file, or - for standard input")
	identity := fs.String("identity", "", "the age identity file, for an encrypted backup")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *in == "" {
		return errors.New("--in is required: a backup file, or - for standard input")
	}
	var ids []age.Identity
	if *identity != "" {
		var err error
		if ids, err = loadIdentities(*identity); err != nil {
			return err
		}
	}
	raw, err := migratorURL(*database)
	if err != nil {
		return err
	}
	ctx, cancel := commandContext(24 * time.Hour)
	defer cancel()
	var r io.Reader = os.Stdin
	if *in != "-" {
		f, err := os.Open(*in) // #nosec G304 -- the file the operator named.
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		r = f
	}
	src, _, err := openBackup(r, ids)
	if err != nil {
		return err
	}
	pool, err := connect(ctx, raw)
	if err != nil {
		return err
	}
	defer pool.Close()
	has, err := store.HasSchema(ctx, pool)
	if err != nil {
		return err
	}
	if has {
		return errors.New("the database is not empty; restore into a new database made with deploy/postgres/roles.sql")
	}
	if err := restoreInto(ctx, raw, src); err != nil {
		return err
	}
	if err := store.CheckMigrations(ctx, pool); errors.Is(err, store.ErrPending) {
		fmt.Fprintln(os.Stderr, "restored; the backup is from an older release, so run wactl migrate up")
		return nil
	} else if err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "restored")
	return nil
}

// restoreInto runs pg_restore in one transaction with src as its input.
// When reading src fails, such as on a tampered age file, pg_restore is
// killed before it can commit, so the database stays empty.
func restoreInto(ctx context.Context, dsn string, src io.Reader, extra ...string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	args := append([]string{"--exit-on-error", "--single-transaction", "--no-password"}, extra...)
	cmd, err := pgCommand(ctx, "pg_restore", "--dbname=", dsn, args...)
	if err != nil {
		return err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("pg_restore: %w", err)
	}
	rr := &readErr{r: src}
	_, copyErr := io.Copy(stdin, rr)
	if rr.err != nil {
		cancel()
	}
	_ = stdin.Close()
	waitErr := cmd.Wait()
	switch {
	case rr.err != nil:
		return fmt.Errorf("reading the backup: %w; nothing was restored", rr.err)
	case waitErr != nil:
		return fmt.Errorf("pg_restore: %w", waitErr)
	}
	return copyErr
}

// readErr keeps the first error of its reader that is not io.EOF.
type readErr struct {
	r   io.Reader
	err error
}

func (r *readErr) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	if err != nil && !errors.Is(err, io.EOF) && r.err == nil {
		r.err = err
	}
	return n, err
}

// worldVerifyBackup restores a backup into a temporary database and
// checks it against its manifest.
func worldVerifyBackup(args []string) error {
	fs := newFlags("world verify-backup")
	database := fs.String("database", "", "PostgreSQL URL of a role with CREATEDB and a database it may connect to, such as postgres; prefer the "+verifyEnv+" environment variable")
	in := fs.String("in", "", "the backup file to verify")
	manifest := fs.String("manifest", "", "its manifest; the default is the .json beside it")
	dir := fs.String("dir", "", "a backup folder: verify its newest backup that has a manifest")
	identity := fs.String("identity", "", "the age identity file, for an encrypted backup")
	every := fs.Duration("every", 0, "with --dir, run as a long-lived process and verify the newest backup this often, such as 24h")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if (*in == "") == (*dir == "") {
		return errors.New("give one of --in (a backup file) and --dir (a backup folder)")
	}
	if *manifest != "" && *in == "" {
		return errors.New("--manifest goes with --in")
	}
	if *every != 0 && (*dir == "" || *every < time.Minute) {
		return errors.New("--every needs --dir and must be at least 1m")
	}
	var ids []age.Identity
	if *identity != "" {
		var err error
		if ids, err = loadIdentities(*identity); err != nil {
			return err
		}
	}
	dsn, err := verifyURL(*database)
	if err != nil {
		return err
	}
	v := &verifier{dsn: dsn, ids: ids}
	if *every != 0 {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		log := slog.New(slog.NewTextHandler(os.Stderr, nil))
		return v.loop(ctx, log, *dir, *every)
	}
	ctx, cancel := commandContext(24 * time.Hour)
	defer cancel()
	file, man := *in, *manifest
	if *dir != "" {
		if file, man, err = latestVerifiable(*dir); err != nil {
			return err
		}
	} else if man == "" {
		man = manifestPath(file)
	}
	m, err := v.verify(ctx, file, man)
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, verifiedLine(file, m))
	return nil
}

// verifyURL returns --database, else verifyEnv or verifyEnv_FILE.
func verifyURL(flagValue string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	value, err := secretenv.Get(verifyEnv)
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", errors.New("give --database or set " + verifyEnv + " or " + verifyEnv + "_FILE: a role with CREATEDB, such as wathena_verify (deploy/README.md, \"Backups\")")
	}
	return value, nil
}

// latestVerifiable names the newest backup in dir that has a manifest.
func latestVerifiable(dir string) (file, manifest string, err error) {
	sets, err := scanDir(dir)
	if err != nil {
		return "", "", err
	}
	s, ok := newestBackup(sets, true)
	if !ok {
		return "", "", fmt.Errorf("%s has no backup with a manifest", dir)
	}
	return filepath.Join(dir, s.dump), filepath.Join(dir, s.manifest), nil
}

func verifiedLine(file string, m *backupManifest) string {
	return fmt.Sprintf("verified %s: the SHA-256, migration %d and counts match the manifest: %d accounts, %d characters, %d item serials, %d zeny",
		filepath.Base(file), m.Migration, m.Counts.Accounts, m.Counts.Characters, m.Counts.ItemSerials, m.Counts.Zeny)
}

// verifier restores backups into temporary databases with a role that
// has CREATEDB.
type verifier struct {
	dsn string
	ids []age.Identity
}

// verify checks a backup:
//  1. Its size and SHA-256 match the manifest.
//  2. It decrypts and restores into a new temporary database, without
//     owners and grants, so the roles of the world need not exist.
//  3. Its migration is the manifest's, and not newer than this wactl's.
//  4. Its counts are the manifest's.
//
// The temporary database is dropped in the end, also on failure.
func (v *verifier) verify(ctx context.Context, file, manifestFile string) (m *backupManifest, err error) {
	if m, err = readManifest(manifestFile); err != nil {
		return nil, err
	}
	if err := checkFile(file, m); err != nil {
		return nil, err
	}
	f, err := os.Open(file) // #nosec G304 -- the backup the operator named.
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	src, encrypted, err := openBackup(f, v.ids)
	if err != nil {
		return nil, err
	}
	if encrypted != m.Encrypted {
		return nil, fmt.Errorf("%s: encrypted is %v, but the manifest says %v", file, encrypted, m.Encrypted)
	}
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	name := "wathena_verify_" + hex.EncodeToString(b)
	target, err := databaseURL(v.dsn, name)
	if err != nil {
		return nil, err
	}
	create, err := pgCommand(ctx, "createdb", "--maintenance-db=", v.dsn, "--no-password", name)
	if err != nil {
		return nil, err
	}
	if err := runPG(create); err != nil {
		return nil, err
	}
	defer func() {
		// A stopped run still drops the database, with its own time limit.
		dctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
		defer cancel()
		drop, derr := pgCommand(dctx, "dropdb", "--maintenance-db=", v.dsn, "--no-password", "--if-exists", "--force", name)
		if derr == nil {
			derr = runPG(drop)
		}
		if derr != nil {
			err = errors.Join(err, fmt.Errorf("dropping the temporary database %s: %w", name, derr))
		}
	}()
	if err := restoreInto(ctx, target, src, "--no-owner", "--no-acl"); err != nil {
		return nil, err
	}
	current, latest, counts, err := readRestored(ctx, target)
	if err != nil {
		return nil, err
	}
	if p := restoredProblems(current, latest, counts, m); len(p) > 0 {
		return nil, errors.New("the restored backup does not match its manifest: " + strings.Join(p, "; "))
	}
	return m, nil
}

// readRestored reads a restored database's newest migration, this
// wactl's newest migration, and the counts.
func readRestored(ctx context.Context, dsn string) (current, latest int64, counts store.WorldCounts, err error) {
	pool, err := connect(ctx, dsn)
	if err != nil {
		return 0, 0, counts, err
	}
	defer pool.Close()
	mig, err := store.NewMigrator(pool)
	if err != nil {
		return 0, 0, counts, err
	}
	current, latest, err = mig.Versions(ctx)
	_ = mig.Close()
	if err != nil {
		return 0, 0, counts, err
	}
	counts, err = store.CountWorld(ctx, pool)
	return current, latest, counts, err
}

// restoredProblems compares a restored database with its manifest. The
// migration must be the manifest's and not newer than this wactl's, and
// the counts must be the manifest's.
func restoredProblems(current, latest int64, counts store.WorldCounts, m *backupManifest) []string {
	var problems []string
	if current > latest {
		problems = append(problems, fmt.Sprintf("migration %d is newer than this wactl's %d", current, latest))
	}
	if current != m.Migration {
		problems = append(problems, fmt.Sprintf("migration %d, but the manifest says %d", current, m.Migration))
	}
	return append(problems, countProblems(counts, m.Counts)...)
}

// countProblems lists the counts that differ.
func countProblems(got, want store.WorldCounts) []string {
	var out []string
	for _, c := range []struct {
		name      string
		got, want int64
	}{
		{"accounts", got.Accounts, want.Accounts},
		{"characters", got.Characters, want.Characters},
		{"item serials", got.ItemSerials, want.ItemSerials},
		{"zeny", got.Zeny, want.Zeny},
	} {
		if c.got != c.want {
			out = append(out, fmt.Sprintf("%d %s, but the manifest says %d", c.got, c.name, c.want))
		}
	}
	return out
}

// loop verifies the newest backup of dir every interval until ctx ends,
// and logs each result. The first check runs at once. When dir has no
// backup yet, it looks again after an hour, or after the interval when
// that is shorter.
func (v *verifier) loop(ctx context.Context, log *slog.Logger, dir string, every time.Duration) error {
	log.Info("verify schedule", "dir", dir, "every", every.String())
	next := time.Now()
	for {
		if !sleepUntil(ctx, next) {
			log.Info("stopped")
			return nil
		}
		start := time.Now()
		file, man, err := latestVerifiable(dir)
		var m *backupManifest
		if err == nil {
			m, err = v.verify(ctx, file, man)
		}
		if ctx.Err() != nil {
			log.Info("stopped during a check")
			return nil
		}
		next = start.Add(every)
		switch {
		case file == "":
			// No backup yet, such as on a new server: look again sooner.
			log.Error("no backup to check", "error", err.Error())
			next = start.Add(min(every, retryAfter))
		case err != nil:
			log.Error("backup check failed", "file", filepath.Base(file), "error", err.Error())
		default:
			log.Info("backup verified", "file", filepath.Base(file), "seconds", int(time.Since(start).Seconds()),
				"migration", m.Migration, "accounts", m.Counts.Accounts, "characters", m.Counts.Characters,
				"item_serials", m.Counts.ItemSerials, "zeny", m.Counts.Zeny)
		}
		log.Info("next check", "at", next.UTC().Format(time.RFC3339))
	}
}

// databaseURL is a postgres:// URL with another database name.
func databaseURL(raw, name string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.User == nil {
		return "", errors.New("the database URL must be a postgres:// URL with a user name")
	}
	u.Path = "/" + name
	u.RawPath = ""
	return u.String(), nil
}
