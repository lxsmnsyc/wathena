package modkit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/lxsmnsyc/wathena/server/mod"
)

// InstallOptions say what wactl mod install installs, and where.
type InstallOptions struct {
	// Source is a Git URL, a .zip file, or a folder.
	Source string
	// ModsDir is the server's mods folder (mods.dir).
	ModsDir string
	// Ref is a tag, branch or commit to check out of a Git source.
	Ref string
	// SHA256 is the .zip file's expected checksum. Without it, a
	// <file>.sha256 next to the file is used when there is one.
	SHA256 string
	// PublicKey is a minisign public key, or a file that holds one. With
	// it, <file>.minisig must exist and match.
	PublicKey string
	// Replace removes an installed mod of the same ID first.
	Replace bool
}

// IsGitURL reports whether a source names a Git repository.
func IsGitURL(s string) bool {
	for _, p := range []string{"https://", "http://", "ssh://", "git://", "git@", "file://"} {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return strings.HasSuffix(s, ".git")
}

// Install checks a mod and copies it into the mods folder
// (strategy section 6.10):
//   - A Git source is cloned, at Ref when given.
//   - A .zip file's checksum is checked when one is known, and its
//     signature when a public key is given.
//   - The mod is validated against core and the mods already installed.
//   - The permissions it requests are shown. None are approved: the
//     owner adds the config lines that Install prints.
//
// It writes what it does to out.
func (e *Env) Install(ctx context.Context, o InstallOptions, out io.Writer) error {
	if o.ModsDir == "" {
		return errors.New("name the server's mods folder with --mods-dir")
	}
	if fi, err := os.Stat(o.ModsDir); err != nil || !fi.IsDir() {
		return fmt.Errorf("%s is not a folder", o.ModsDir)
	}
	src := o.Source
	isZip := false
	switch {
	case IsGitURL(src):
		tmp, err := os.MkdirTemp("", "wactl-install-")
		if err != nil {
			return err
		}
		defer func() { _ = os.RemoveAll(tmp) }()
		commit, err := clone(ctx, src, o.Ref, tmp)
		if err != nil {
			return err
		}
		say(out, "cloned %s at %s\n", src, commit)
		if o.SHA256 != "" || o.PublicKey != "" {
			return errors.New("--sha256 and --public-key apply to .zip files; a Git source is pinned with --ref")
		}
		src = tmp
	case strings.EqualFold(filepath.Ext(src), ".zip"):
		isZip = true
		data, err := os.ReadFile(src) // #nosec G304 -- the file the owner named.
		if err != nil {
			return err
		}
		if err := checkSum(src, data, o.SHA256, out); err != nil {
			return err
		}
		if err := checkSignature(src, data, o.PublicKey, out); err != nil {
			return err
		}
	default:
		if o.Ref != "" || o.SHA256 != "" || o.PublicKey != "" {
			return errors.New("--ref, --sha256 and --public-key apply to Git and .zip sources")
		}
	}
	return e.installFrom(ctx, o, src, isZip, out)
}

// clone clones a repository into dir and checks out ref, and returns the
// commit.
func clone(ctx context.Context, url, ref, dir string) (string, error) {
	git := func(args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, "git", args...) // #nosec G204 -- git with the URL and ref the owner named.
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		b, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("git %s: %v: %s", args[0], err, strings.TrimSpace(string(b)))
		}
		return strings.TrimSpace(string(b)), nil
	}
	if _, err := git("clone", "--quiet", "--", url, dir); err != nil {
		return "", err
	}
	if ref != "" {
		if _, err := git("-C", dir, "checkout", "--quiet", "--detach", ref); err != nil {
			return "", err
		}
	}
	return git("-C", dir, "rev-parse", "HEAD")
}

// checkSum checks a .zip file against its known SHA-256.
func checkSum(file string, data []byte, want string, out io.Writer) error {
	from := "--sha256"
	if want == "" {
		b, err := os.ReadFile(file + ".sha256") // #nosec G304 -- next to the file the owner named.
		if errors.Is(err, fs.ErrNotExist) {
			say(out, "no checksum: %s.sha256 does not exist and --sha256 is not given\n", file)
			return nil
		}
		if err != nil {
			return err
		}
		fields := strings.Fields(string(b))
		if len(fields) == 0 {
			return fmt.Errorf("%s.sha256 is empty", file)
		}
		want, from = fields[0], file+".sha256"
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("checksum mismatch: %s has SHA-256 %s, and %s says %s", file, got, from, want)
	}
	say(out, "checksum ok: SHA-256 %s (from %s)\n", got, from)
	return nil
}

// checkSignature checks a .zip file's minisign signature when a public
// key is given.
func checkSignature(file string, data []byte, key string, out io.Writer) error {
	sig, err := os.ReadFile(file + ".minisig") // #nosec G304 -- next to the file the owner named.
	switch {
	case key == "" && err == nil:
		say(out, "not verified: %s.minisig exists, and no --public-key is given\n", file)
		return nil
	case key == "":
		return nil
	case err != nil:
		return fmt.Errorf("--public-key is given, and %s.minisig cannot be read: %w", file, err)
	}
	comment, err := VerifyMinisign(data, string(sig), key)
	if err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	say(out, "signature ok: %s\n", comment)
	return nil
}

// installFrom validates the mod at src and copies it into the mods
// folder.
func (e *Env) installFrom(ctx context.Context, o InstallOptions, src string, isZip bool, out io.Writer) error {
	rep := e.Validate(ctx, src, o.ModsDir)
	if rep.Manifest == nil || len(rep.Problems) > 0 {
		return fmt.Errorf("the mod does not pass wactl mod validate:\n  %s", strings.Join(rep.Problems, "\n  "))
	}
	m := rep.Manifest
	say(out, "%s %s: %s\n", m.ID, m.Version, m.Name)
	PrintPermissions(out, m.ID, rep.Permissions)

	installed, err := mod.Discover(o.ModsDir)
	if err != nil {
		return err
	}
	for _, other := range installed {
		if other.ID() != m.ID {
			continue
		}
		if !o.Replace {
			return fmt.Errorf("%s is installed already at %s; use --replace to replace it", m.ID, other.Path)
		}
		if err := os.RemoveAll(other.Path); err != nil {
			return err
		}
		say(out, "removed %s\n", other.Path)
	}
	var dest string
	if isZip {
		dest = filepath.Join(o.ModsDir, m.ID+"-"+m.Version+".zip")
		data, err := os.ReadFile(src) // #nosec G304 -- the file the owner named.
		if err != nil {
			return err
		}
		if err := os.WriteFile(dest, data, 0o600); err != nil {
			return err
		}
	} else {
		dest = filepath.Join(o.ModsDir, m.ID)
		files, err := packFiles(src, "")
		if err != nil {
			return err
		}
		for _, rel := range files {
			data, err := os.ReadFile(filepath.Join(src, filepath.FromSlash(rel))) // #nosec G304 -- a file of the mod.
			if err != nil {
				return err
			}
			if err := writeFile(filepath.Join(dest, filepath.FromSlash(rel)), data); err != nil {
				return err
			}
		}
	}
	say(out, "installed %s\n", dest)
	return nil
}

// PrintPermissions shows a mod's permissions, which of them need the
// owner's approval, and the config lines that approve them.
func PrintPermissions(out io.Writer, id string, perms []mod.Permission) {
	if len(perms) == 0 {
		say(out, "%s\n", "permissions: none requested")
		return
	}
	say(out, "%s\n", "permissions:")
	var approve []string
	for _, p := range perms {
		note := "granted to every mod"
		if p.Approval {
			note = "needs the owner's approval"
			approve = append(approve, p.Name)
		}
		if p.Flagged {
			note += ", flagged: logged as a warning at each start"
		}
		say(out, "  %-20s %s: %s\n", p.Name, note, p.Allows)
	}
	if len(approve) == 0 {
		return
	}
	say(out, "nothing is approved. To approve, review the mod, then add to the server config:\n")
	say(out, "  mods:\n    approve:\n      %s: [%s]\n", id, strings.Join(approve, ", "))
}

// say writes to a writer of messages, where a failed write has no one
// to tell.
func say(w io.Writer, format string, args ...any) { _, _ = fmt.Fprintf(w, format, args...) }
