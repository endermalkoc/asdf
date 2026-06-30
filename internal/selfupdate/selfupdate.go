// Package selfupdate implements `cusp upgrade`: resolve a GitHub release, download
// the archive for the running OS/arch, verify its SHA-256 against checksums.txt, and
// atomically replace the running binary in place.
//
// It mirrors the conventions baked into install.sh and .goreleaser.yaml exactly — the
// archive name (`<binary>_<version>_<os>_<arch>.tar.gz`, `.zip` on Windows), the
// `checksums.txt` layout (`<hex>  <asset>`), and the release download base URL — so the
// binary can update itself from the same artifacts the install script consumes.
package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// DefaultRepo is the GitHub "owner/name" releases are published under (see install.sh).
const DefaultRepo = "endermalkoc/cusp"

// DefaultBinary is the binary's base name inside a release archive.
const DefaultBinary = "cusp"

const userAgent = "cusp-selfupdate"

// Options configures an upgrade run. Zero values fall back to sensible defaults
// (the running platform, the canonical repo, os.Executable()).
type Options struct {
	Repo     string       // GitHub owner/name; default DefaultRepo
	Binary   string       // archive binary base name; default DefaultBinary
	Current  string       // current version, e.g. "v0.1.0" or "dev"
	Target   string       // release tag to install; empty resolves the latest release
	GOOS     string       // default runtime.GOOS
	GOARCH   string       // default runtime.GOARCH
	DestPath string       // executable to replace; empty uses os.Executable()
	Client   *http.Client // default: a client with a 60s timeout
}

// Result describes what an upgrade did (or would do).
type Result struct {
	Previous string // version before the upgrade
	Latest   string // resolved target tag
	UpToDate bool   // already on Latest — nothing downloaded or replaced
	Asset    string // archive asset name for this platform
	Path     string // executable path that was replaced
}

func (o *Options) applyDefaults() error {
	if o.Repo == "" {
		o.Repo = DefaultRepo
	}
	if o.Binary == "" {
		o.Binary = DefaultBinary
	}
	if o.GOOS == "" {
		o.GOOS = runtime.GOOS
	}
	if o.GOARCH == "" {
		o.GOARCH = runtime.GOARCH
	}
	if o.Client == nil {
		o.Client = defaultClient()
	}
	if o.DestPath == "" {
		p, err := executablePath()
		if err != nil {
			return fmt.Errorf("locate the running binary: %w", err)
		}
		o.DestPath = p
	}
	return nil
}

func defaultClient() *http.Client { return &http.Client{Timeout: 60 * time.Second} }

// executablePath resolves the path of the running binary, following symlinks so we
// replace the real file rather than a symlink to it.
func executablePath() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		p = resolved
	}
	return p, nil
}

// LatestVersion resolves the most recent release tag for repo via the GitHub API.
func LatestVersion(ctx context.Context, client *http.Client, repo string) (string, error) {
	if client == nil {
		client = defaultClient()
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("query latest release: %w", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return "", fmt.Errorf("no published releases for %s yet", repo)
	default:
		return "", fmt.Errorf("GitHub API: %s resolving latest release", resp.Status)
	}
	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", fmt.Errorf("decode release response: %w", err)
	}
	if rel.TagName == "" {
		return "", fmt.Errorf("latest release has no tag_name")
	}
	return rel.TagName, nil
}

// Run performs (or, when the current version already matches the target, skips) an
// upgrade. progress, if non-nil, receives short human-readable step messages.
func Run(ctx context.Context, o Options, progress func(string)) (*Result, error) {
	if progress == nil {
		progress = func(string) {}
	}
	if err := o.applyDefaults(); err != nil {
		return nil, err
	}

	target := o.Target
	if target == "" {
		progress("Resolving the latest release…")
		v, err := LatestVersion(ctx, o.Client, o.Repo)
		if err != nil {
			return nil, err
		}
		target = v
	}

	res := &Result{
		Previous: displayVersion(o.Current),
		Latest:   target,
		Asset:    AssetName(o.Binary, target, o.GOOS, o.GOARCH),
		Path:     o.DestPath,
	}
	if SameVersion(o.Current, target) {
		res.UpToDate = true
		return res, nil
	}

	// Fail fast (before any download) if we can't write where the binary lives.
	destDir := filepath.Dir(o.DestPath)
	if err := ensureWritable(destDir); err != nil {
		return nil, err
	}

	work, err := os.MkdirTemp("", "cusp-upgrade-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(work)

	base := downloadBase(o.Repo, target)
	archivePath := filepath.Join(work, res.Asset)
	progress(fmt.Sprintf("Downloading %s (%s)…", res.Asset, target))
	if err := download(ctx, o.Client, base+"/"+res.Asset, archivePath); err != nil {
		return nil, fmt.Errorf("download %s: %w", res.Asset, err)
	}

	progress("Verifying checksum…")
	sumsPath := filepath.Join(work, "checksums.txt")
	if err := download(ctx, o.Client, base+"/checksums.txt", sumsPath); err != nil {
		return nil, fmt.Errorf("download checksums.txt: %w", err)
	}
	sums, err := os.ReadFile(sumsPath)
	if err != nil {
		return nil, err
	}
	want, err := ParseChecksum(sums, res.Asset)
	if err != nil {
		return nil, err
	}
	got, err := sha256File(archivePath)
	if err != nil {
		return nil, err
	}
	if got != want {
		return nil, fmt.Errorf("checksum mismatch for %s:\n  want %s\n  got  %s", res.Asset, want, got)
	}

	progress("Installing…")
	// Extract next to the destination so the final rename is atomic (same filesystem).
	newBin, err := extractBinary(archivePath, res.Asset, o.Binary, destDir)
	if err != nil {
		return nil, err
	}
	if err := replaceExecutable(newBin, o.DestPath); err != nil {
		_ = os.Remove(newBin)
		return nil, fmt.Errorf("replace %s: %w", o.DestPath, err)
	}
	return res, nil
}

// AssetName returns the archive file name GoReleaser produces for a target platform.
// It matches install.sh: `<binary>_<version-without-v>_<os>_<arch>.<ext>`.
func AssetName(binary, version, goos, goarch string) string {
	ext := "tar.gz"
	if goos == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf("%s_%s_%s_%s.%s", binary, strings.TrimPrefix(version, "v"), goos, goarch, ext)
}

func downloadBase(repo, version string) string {
	return fmt.Sprintf("https://github.com/%s/releases/download/%s", repo, version)
}

// ParseChecksum returns the lower-case hex SHA-256 recorded for asset in a GoReleaser
// checksums.txt body (lines of the form "<hex>  <asset>").
func ParseChecksum(data []byte, asset string) (string, error) {
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == asset {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("no checksum entry for %s in checksums.txt", asset)
}

// SameVersion reports whether current and target name the same release. A "dev" /
// unset / "(devel)" current version is never considered a match, so a source build
// can always upgrade to a real release.
func SameVersion(current, target string) bool {
	c := strings.TrimPrefix(strings.TrimSpace(current), "v")
	t := strings.TrimPrefix(strings.TrimSpace(target), "v")
	switch c {
	case "", "dev", "(devel)":
		return false
	}
	return c == t
}

func displayVersion(v string) string {
	if strings.TrimSpace(v) == "" {
		return "dev"
	}
	return v
}

// ensureWritable verifies dir accepts new files, so a permission problem surfaces
// before anything is downloaded rather than after.
func ensureWritable(dir string) error {
	f, err := os.CreateTemp(dir, ".cusp-upgrade-perm-*")
	if err != nil {
		return fmt.Errorf("cannot write to %s — need write access to replace the binary "+
			"(try sudo, or re-run the install script): %w", dir, err)
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return nil
}

func download(ctx context.Context, client *http.Client, url, dst string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s for %s", resp.Status, url)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func sha256File(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// extractBinary pulls the binary out of a verified .tar.gz or .zip archive and writes
// it to a fresh temp file in destDir, returning that path. Operating in destDir keeps
// the subsequent rename atomic. The caller owns (and on error this function removes)
// the temp file.
func extractBinary(archivePath, asset, binary, destDir string) (string, error) {
	out, err := os.CreateTemp(destDir, ".cusp-upgrade-*")
	if err != nil {
		return "", fmt.Errorf("create staging file in %s: %w", destDir, err)
	}
	tmp := out.Name()
	fail := func(e error) (string, error) {
		_ = out.Close()
		_ = os.Remove(tmp)
		return "", e
	}

	matches := func(name string) bool {
		b := path.Base(name) // archive entries use forward slashes
		return b == binary || b == binary+".exe"
	}

	if strings.HasSuffix(asset, ".zip") {
		zr, err := zip.OpenReader(archivePath)
		if err != nil {
			return fail(err)
		}
		defer zr.Close()
		for _, f := range zr.File {
			if !matches(f.Name) {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return fail(err)
			}
			_, err = io.Copy(out, rc)
			_ = rc.Close()
			if err != nil {
				return fail(err)
			}
			if err := out.Close(); err != nil {
				_ = os.Remove(tmp)
				return "", err
			}
			return tmp, nil
		}
		return fail(fmt.Errorf("%s not found inside %s", binary, asset))
	}

	f, err := os.Open(archivePath)
	if err != nil {
		return fail(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fail(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fail(err)
		}
		if hdr.Typeflag != tar.TypeReg || !matches(hdr.Name) {
			continue
		}
		if _, err := io.Copy(out, tr); err != nil {
			return fail(err)
		}
		if err := out.Close(); err != nil {
			_ = os.Remove(tmp)
			return "", err
		}
		return tmp, nil
	}
	return fail(fmt.Errorf("%s not found inside %s", binary, asset))
}

// replaceExecutable atomically swaps the freshly extracted newBin in for dst,
// preserving dst's permission bits. On Unix os.Rename replaces the file even while it
// is running (the running process keeps the old inode). On Windows a running image
// cannot be overwritten, so the current binary is moved aside first.
func replaceExecutable(newBin, dst string) error {
	mode := os.FileMode(0o755)
	if fi, err := os.Stat(dst); err == nil {
		mode = fi.Mode().Perm() | 0o111
	}
	if err := os.Chmod(newBin, mode); err != nil {
		return err
	}

	if runtime.GOOS == "windows" {
		old := dst + ".old"
		_ = os.Remove(old)
		if err := os.Rename(dst, old); err != nil {
			return err
		}
		if err := os.Rename(newBin, dst); err != nil {
			_ = os.Rename(old, dst) // best-effort rollback
			return err
		}
		_ = os.Remove(old) // may fail while the old image is still mapped; harmless
		return nil
	}
	return os.Rename(newBin, dst)
}
