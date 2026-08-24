package matrix

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// maxArchiveSize caps a decompressed archive at 100 MiB so one deployment
// cannot exhaust the server's disk (and, combined with the per-file path
// checks, the classic zip-bomb).
const maxArchiveSize = 100 << 20

// ExtractArchiveForTest exposes extractArchive to the external test package.
func ExtractArchiveForTest(dir string, data []byte) (string, error) {
	return extractArchive(dir, data)
}

// extractArchive unpacks archive bytes (.tar.gz or .zip, sniffed by magic
// number) into a fresh directory under dir and returns that directory. The
// extraction is traversal-safe: entries escaping the target directory are
// rejected, symlinks and hardlinks are skipped, and the total unpacked size
// is capped. If every entry shares one top-level directory it is stripped,
// so archives of "dist/" deploy the same as archives of "./".
func extractArchive(dir string, data []byte) (string, error) {
	dst, err := os.MkdirTemp(dir, "remote-")
	if err != nil {
		return "", err
	}
	switch {
	case len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b:
		err = extractTarGz(dst, data)
	case len(data) >= 4 && bytes.Equal(data[:4], []byte("PK\x03\x04")):
		err = extractZip(dst, data)
	default:
		os.RemoveAll(dst)
		return "", fmt.Errorf("unsupported archive format (want .tar.gz or .zip)")
	}
	if err != nil {
		os.RemoveAll(dst)
		return "", err
	}
	top, err := stripSingleTopDir(dst)
	if err != nil {
		os.RemoveAll(dst)
		return "", err
	}
	return top, nil
}

// extractTarGz unpacks a gzip-compressed tar stream into dst.
func extractTarGz(dst string, data []byte) error {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("gzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(io.LimitReader(gz, maxArchiveSize+1))
	var total int64
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeDir && hdr.Typeflag != tar.TypeReg {
			continue // skip symlinks, hardlinks, devices, ...
		}
		target, err := safeJoin(dst, hdr.Name)
		if err != nil {
			return err
		}
		if hdr.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		total += hdr.Size
		if total > maxArchiveSize {
			return fmt.Errorf("archive expands beyond %d bytes", maxArchiveSize)
		}
		if err := copyEntry(tr, target); err != nil {
			return err
		}
	}
}

// extractZip unpacks a zip archive into dst.
func extractZip(dst string, data []byte) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("zip: %w", err)
	}
	var total int64
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			target, err := safeJoin(dst, f.Name)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if !f.Mode().IsRegular() {
			continue // skip symlinks and special files
		}
		total += int64(f.UncompressedSize64)
		if total > maxArchiveSize {
			return fmt.Errorf("archive expands beyond %d bytes", maxArchiveSize)
		}
		target, err := safeJoin(dst, f.Name)
		if err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("zip %s: %w", f.Name, err)
		}
		err = copyEntry(rc, target)
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// safeJoin resolves name relative to dst, rejecting any entry that would
// escape it (zip-slip / tar traversal).
func safeJoin(dst, name string) (string, error) {
	name = filepath.FromSlash(name)
	target := filepath.Join(dst, name)
	rel, err := filepath.Rel(dst, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("archive entry %q escapes the extraction directory", name)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	return target, nil
}

// copyEntry writes r into a new file at target (mode 0644).
func copyEntry(r io.Reader, target string) error {
	f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, r)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if n > maxArchiveSize {
		return fmt.Errorf("archive expands beyond %d bytes", maxArchiveSize)
	}
	return nil
}

// stripSingleTopDir removes a sole top-level directory from dst, so an
// archive whose root is "dist/..." deploys like one with files at the root.
// Mixed layouts (files at root plus subdirectories) are left as-is.
func stripSingleTopDir(dst string) (string, error) {
	entries, err := os.ReadDir(dst)
	if err != nil {
		return "", err
	}
	if len(entries) != 1 || !entries[0].IsDir() {
		return dst, nil
	}
	inner := filepath.Join(dst, entries[0].Name())
	// Only strip when the top-level dir actually contains everything:
	// recursively move its contents up.
	moved, err := os.ReadDir(inner)
	if err != nil {
		return "", err
	}
	for _, e := range moved {
		if err := os.Rename(filepath.Join(inner, e.Name()), filepath.Join(dst, e.Name())); err != nil {
			return dst, nil // mixed layout, keep as-is
		}
	}
	os.Remove(inner)
	return dst, nil
}
