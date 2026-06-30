package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestAssetName(t *testing.T) {
	cases := []struct {
		goos, goarch, version, want string
	}{
		{"linux", "amd64", "v0.1.0", "cusp_0.1.0_linux_amd64.tar.gz"},
		{"darwin", "arm64", "0.2.3", "cusp_0.2.3_darwin_arm64.tar.gz"},
		{"windows", "amd64", "v1.0.0", "cusp_1.0.0_windows_amd64.zip"},
	}
	for _, c := range cases {
		if got := AssetName("cusp", c.version, c.goos, c.goarch); got != c.want {
			t.Errorf("AssetName(%q,%q,%q) = %q, want %q", c.version, c.goos, c.goarch, got, c.want)
		}
	}
}

func TestParseChecksum(t *testing.T) {
	body := []byte("" +
		"aaaa1111  cusp_0.1.0_darwin_arm64.tar.gz\n" +
		"bbbb2222  cusp_0.1.0_linux_amd64.tar.gz\n" +
		"CCCC3333  cusp_0.1.0_windows_amd64.zip\n")

	if got, err := ParseChecksum(body, "cusp_0.1.0_linux_amd64.tar.gz"); err != nil || got != "bbbb2222" {
		t.Errorf("linux: got %q, err %v", got, err)
	}
	// hex is normalized to lower-case.
	if got, err := ParseChecksum(body, "cusp_0.1.0_windows_amd64.zip"); err != nil || got != "cccc3333" {
		t.Errorf("windows: got %q, err %v", got, err)
	}
	if _, err := ParseChecksum(body, "cusp_9.9.9_linux_arm64.tar.gz"); err == nil {
		t.Error("expected error for a missing asset")
	}
}

func TestSameVersion(t *testing.T) {
	cases := []struct {
		current, target string
		want            bool
	}{
		{"v0.1.0", "v0.1.0", true},
		{"0.1.0", "v0.1.0", true}, // leading v is optional on either side
		{"v0.1.0", "v0.2.0", false},
		{"dev", "v0.1.0", false},     // a dev build always upgrades
		{"", "v0.1.0", false},        // unset never matches
		{"(devel)", "v0.1.0", false}, // module build-info sentinel
	}
	for _, c := range cases {
		if got := SameVersion(c.current, c.target); got != c.want {
			t.Errorf("SameVersion(%q,%q) = %v, want %v", c.current, c.target, got, c.want)
		}
	}
}

func TestExtractBinaryTarGz(t *testing.T) {
	dir := t.TempDir()
	want := []byte("#!/bin/sh\necho fake cusp binary\n")

	archive := filepath.Join(dir, "cusp_0.1.0_linux_amd64.tar.gz")
	writeTarGz(t, archive, map[string][]byte{
		"README.md": []byte("docs"),
		"cusp":      want, // the binary, possibly alongside other files
	})

	got := extractAndRead(t, archive, "cusp_0.1.0_linux_amd64.tar.gz", "cusp", dir)
	if !bytes.Equal(got, want) {
		t.Errorf("extracted contents mismatch:\n got %q\nwant %q", got, want)
	}
}

func TestExtractBinaryZipWithExe(t *testing.T) {
	dir := t.TempDir()
	want := []byte("MZ fake windows binary")

	archive := filepath.Join(dir, "cusp_0.1.0_windows_amd64.zip")
	writeZip(t, archive, map[string][]byte{
		"cusp.exe": want, // windows archives carry the .exe name
	})

	got := extractAndRead(t, archive, "cusp_0.1.0_windows_amd64.zip", "cusp", dir)
	if !bytes.Equal(got, want) {
		t.Errorf("extracted contents mismatch:\n got %q\nwant %q", got, want)
	}
}

func TestExtractBinaryMissing(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "cusp_0.1.0_linux_amd64.tar.gz")
	writeTarGz(t, archive, map[string][]byte{"README.md": []byte("no binary here")})

	if _, err := extractBinary(archive, "cusp_0.1.0_linux_amd64.tar.gz", "cusp", dir); err == nil {
		t.Error("expected an error when the binary is absent from the archive")
	}
}

func TestReplaceExecutable(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "cusp")
	if err := os.WriteFile(dst, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	newBin := filepath.Join(dir, ".staged")
	if err := os.WriteFile(newBin, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := replaceExecutable(newBin, dst); err != nil {
		t.Fatalf("replaceExecutable: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Errorf("dst contents = %q, want %q", got, "new")
	}
	fi, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm()&0o111 == 0 {
		t.Errorf("replaced binary is not executable: mode %v", fi.Mode())
	}
}

// extractAndRead runs extractBinary and returns the staged file's contents.
func extractAndRead(t *testing.T, archive, asset, binary, dir string) []byte {
	t.Helper()
	tmp, err := extractBinary(archive, asset, binary, dir)
	if err != nil {
		t.Fatalf("extractBinary: %v", err)
	}
	got, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func writeTarGz(t *testing.T, path string, files map[string][]byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for name, data := range files {
		if err := tw.WriteHeader(&tar.Header{
			Name:     name,
			Mode:     0o755,
			Size:     int64(len(data)),
			Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeZip(t *testing.T, path string, files map[string][]byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for name, data := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}
