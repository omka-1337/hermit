// Package archive unpacks the archives mods come in: zip, 7z and rar, as
// Nexus Mods hands them out. It is pure Go, so it works wherever Hermit runs,
// with no 7z or unrar on the system.
package archive

import (
	"archive/zip"
	"bytes"
	"compress/bzip2"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/bodgit/sevenzip"
	"github.com/klauspost/compress/zstd"
	"github.com/nwaples/rardecode/v2"
	"github.com/ulikunitz/xz/lzma"
)

// Compression methods of zip archives beyond Store and Deflate, which some
// packing tools use.
const (
	zipBzip2 = 12
	zipLZMA  = 14
	zipZstd  = 93
)

var ErrUnsupported = errors.New("not a zip, 7z or rar archive")

// Supported reports whether a file's name says it is an archive Extract
// unpacks.
func Supported(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".zip", ".7z", ".rar":
		return true
	}
	return false
}

// List returns the names of the files in an archive, slash-separated,
// without unpacking it.
func List(file string) ([]string, error) {
	var names []string
	switch strings.ToLower(filepath.Ext(file)) {
	case ".zip":
		r, err := zip.OpenReader(file)
		if err != nil {
			return nil, err
		}
		defer r.Close()
		for _, f := range r.File {
			names = append(names, f.Name)
		}
	case ".7z":
		r, err := sevenzip.OpenReader(file)
		if err != nil {
			return nil, err
		}
		defer r.Close()
		for _, f := range r.File {
			names = append(names, f.Name)
		}
	case ".rar":
		files, err := rardecode.List(file)
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			names = append(names, f.Name)
		}
	default:
		return nil, fmt.Errorf("%s: %w", filepath.Base(file), ErrUnsupported)
	}
	for i, name := range names {
		names[i] = strings.ReplaceAll(name, `\`, "/")
	}
	return names, nil
}

// Extract unpacks the archive at file into dir, which it creates. progress,
// when not nil, is told how many bytes of how many are out so far.
func Extract(file, dir string, progress func(done, total int64)) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if progress == nil {
		progress = func(int64, int64) {}
	}
	u := &unpacker{dir: dir, names: map[string]string{}}
	switch strings.ToLower(filepath.Ext(file)) {
	case ".zip":
		return u.zip(file, progress)
	case ".7z":
		return u.sevenZip(file, progress)
	case ".rar":
		return u.rar(file, progress)
	}
	return fmt.Errorf("%s: %w", filepath.Base(file), ErrUnsupported)
}

// unpacker writes the entries of one archive into dir. Archives made on
// Windows can spell a folder differently from entry to entry, Textures and
// textures; there it is one folder, so here every spelling goes into the
// one seen first.
type unpacker struct {
	dir string
	// names maps lower-case paths to how they were first written.
	names map[string]string
}

func (u *unpacker) zip(file string, progress func(done, total int64)) error {
	r, err := zip.OpenReader(file)
	if err != nil {
		return err
	}
	defer r.Close()
	r.RegisterDecompressor(zipBzip2, func(in io.Reader) io.ReadCloser { return io.NopCloser(bzip2.NewReader(in)) })
	r.RegisterDecompressor(zipZstd, func(in io.Reader) io.ReadCloser {
		d, err := zstd.NewReader(in)
		if err != nil {
			return io.NopCloser(errReader{err})
		}
		return d.IOReadCloser()
	})
	var total, done int64
	for _, f := range r.File {
		total += int64(f.UncompressedSize64)
	}
	for _, f := range r.File {
		isDir := f.FileInfo().IsDir() || strings.HasSuffix(f.Name, "/") || strings.HasSuffix(f.Name, `\`)
		open := func() (io.ReadCloser, error) { return f.Open() }
		if f.Method == zipLZMA {
			open = func() (io.ReadCloser, error) { return openZipLZMA(f) }
		}
		err := u.write(f.Name, isDir, open)
		if err != nil {
			return err
		}
		done += int64(f.UncompressedSize64)
		progress(done, total)
	}
	return nil
}

func (u *unpacker) sevenZip(file string, progress func(done, total int64)) error {
	r, err := sevenzip.OpenReader(file)
	if err != nil {
		return err
	}
	defer r.Close()
	var total, done int64
	for _, f := range r.File {
		total += int64(f.UncompressedSize)
	}
	// In archive order: a solid 7z is unpacked in one pass that way.
	for _, f := range r.File {
		err := u.write(f.Name, f.FileInfo().IsDir(), func() (io.ReadCloser, error) { return f.Open() })
		if err != nil {
			return err
		}
		done += int64(f.UncompressedSize)
		progress(done, total)
	}
	return nil
}

func (u *unpacker) rar(file string, progress func(done, total int64)) error {
	r, err := rardecode.OpenReader(file)
	if err != nil {
		return err
	}
	defer r.Close()
	var total int64
	if fi, err := os.Stat(file); err == nil {
		// RAR does not say up front how much it unpacks to; the archive's
		// own size is the best guess there is.
		total = fi.Size()
	}
	var done int64
	for {
		h, err := r.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		err = u.write(h.Name, h.IsDir, func() (io.ReadCloser, error) { return io.NopCloser(r), nil })
		if err != nil {
			return err
		}
		done += h.PackedSize
		progress(min(done, total), total)
	}
}

// openZipLZMA reads an LZMA entry of a zip. The entry starts with the
// version of the LZMA SDK and the coder's properties; the size, which the
// classic LZMA header has after them, is the entry's own.
func openZipLZMA(f *zip.File) (io.ReadCloser, error) {
	raw, err := f.OpenRaw()
	if err != nil {
		return nil, err
	}
	var head [9]byte
	if _, err := io.ReadFull(raw, head[:]); err != nil {
		return nil, err
	}
	header := make([]byte, 13)
	copy(header, head[4:9])
	binary.LittleEndian.PutUint64(header[5:], f.UncompressedSize64)
	r, err := lzma.NewReader(io.MultiReader(bytes.NewReader(header), raw))
	if err != nil {
		return nil, err
	}
	return io.NopCloser(r), nil
}

type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }

// write puts one entry of an archive into the folder. Names written on
// Windows use backslashes; names that would leave the folder are refused.
func (u *unpacker) write(name string, isDir bool, open func() (io.ReadCloser, error)) error {
	rel, err := cleanName(name)
	if err != nil {
		return err
	}
	if rel == "" {
		return nil
	}
	target := filepath.Join(u.dir, filepath.FromSlash(u.fold(rel)))
	if isDir {
		return os.MkdirAll(target, 0o755)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	in, err := open()
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	defer in.Close()
	out, err := os.Create(target)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return fmt.Errorf("%s: %w", name, err)
	}
	return out.Close()
}

// fold spells each part of a path as it was spelled the first time.
func (u *unpacker) fold(rel string) string {
	parts := strings.Split(rel, "/")
	for i := range parts {
		key := strings.ToLower(strings.Join(parts[:i+1], "/"))
		if seen, ok := u.names[key]; ok {
			parts[i] = path.Base(seen)
		} else {
			u.names[key] = strings.Join(parts[:i+1], "/")
		}
	}
	return strings.Join(parts, "/")
}

func cleanName(name string) (string, error) {
	name = strings.ReplaceAll(name, `\`, "/")
	clean := path.Clean("/" + name)[1:]
	if clean == "" || clean == "." {
		return "", nil
	}
	if strings.HasPrefix(name, "/") || strings.Contains("/"+name+"/", "/../") || !fs.ValidPath(clean) {
		return "", fmt.Errorf("%q would be unpacked outside the mod's folder", name)
	}
	return clean, nil
}
