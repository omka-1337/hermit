package thunderstore

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"hermit/internal/throttle"
)

// Thunderstore restricts names to these characters, which also makes them safe as file names.
var (
	validName    = regexp.MustCompile(`^[A-Za-z0-9_]+$`)
	validNS      = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	validVersion = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
)

// PackageRef identifies a specific package version.
type PackageRef struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Version   string `json:"version"`
}

// ParseDependency parses "<namespace>-<name>-<version>". Names and versions
// never contain '-', so the string is split from the right.
func ParseDependency(s string) (PackageRef, error) {
	i := strings.LastIndexByte(s, '-')
	if i <= 0 {
		return PackageRef{}, fmt.Errorf("invalid dependency %q", s)
	}
	j := strings.LastIndexByte(s[:i], '-')
	if j <= 0 {
		return PackageRef{}, fmt.Errorf("invalid dependency %q", s)
	}
	ref := PackageRef{Namespace: s[:j], Name: s[j+1 : i], Version: s[i+1:]}
	return ref, ref.Validate()
}

func (r PackageRef) Validate() error {
	if !validNS.MatchString(r.Namespace) || !validName.MatchString(r.Name) || !validVersion.MatchString(r.Version) {
		return fmt.Errorf("invalid package reference %s-%s-%s", r.Namespace, r.Name, r.Version)
	}
	return nil
}

// ID is the version-less full name, "<namespace>-<name>".
func (r PackageRef) ID() string { return r.Namespace + "-" + r.Name }

func (r PackageRef) String() string { return r.ID() + "-" + r.Version }

func (c *Client) DownloadURL(ref PackageRef) string {
	return c.baseURL + "/package/download/" + url.PathEscape(ref.Namespace) + "/" + url.PathEscape(ref.Name) + "/" + url.PathEscape(ref.Version) + "/"
}

type Archive struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	URL    string `json:"url"`
}

// DownloadPackage returns the package archive from the local cache, downloading
// it first if needed. progress, if set, receives downloaded and total bytes
// (total is -1 when unknown).
func (c *Client) DownloadPackage(ctx context.Context, ref PackageRef, progress func(done, total int64)) (Archive, error) {
	if err := ref.Validate(); err != nil {
		return Archive{}, err
	}
	dir := filepath.Join(c.cacheDir, "packages")
	path := filepath.Join(dir, ref.String()+".zip")
	archive := Archive{Path: path, URL: c.DownloadURL(ref)}

	if sum, err := verifyZip(path); err == nil {
		archive.SHA256 = sum
		return archive, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Archive{}, err
	}

	// Thunderstore's download redirects fail now and then with 5xx errors,
	// which should not break installing a modpack of dozens of mods.
	var lastErr error
	for attempt := range downloadAttempts {
		if attempt > 0 {
			select {
			case <-time.After(c.retryDelay << (attempt - 1)):
			case <-ctx.Done():
				return Archive{}, ctx.Err()
			}
		}
		sum, err := c.downloadOnce(ctx, ref, archive.URL, dir, path, progress)
		if err == nil {
			archive.SHA256 = sum
			return archive, nil
		}
		lastErr = err
		if !isTransient(err) || ctx.Err() != nil {
			break
		}
	}
	return Archive{}, lastErr
}

const downloadAttempts = 4

// transientError marks failures worth retrying.
type transientError struct{ error }

func (e transientError) Unwrap() error { return e.error }

func isTransient(err error) bool {
	var t transientError
	return errors.As(err, &t)
}

func (c *Client) downloadOnce(ctx context.Context, ref PackageRef, url, dir, path string, progress func(done, total int64)) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.download.Do(req)
	if err != nil {
		return "", transientError{fmt.Errorf("download %s: %w", ref, err)}
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return "", fmt.Errorf("%s: %w", ref, ErrNotFound)
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return "", transientError{fmt.Errorf("download %s: %s", ref, resp.Status)}
	case resp.StatusCode != http.StatusOK:
		return "", fmt.Errorf("download %s: %s", ref, resp.Status)
	}

	tmp, err := os.CreateTemp(dir, ref.String()+".*.part")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())

	hash := sha256.New()
	body := throttle.Downloads.Reader(ctx, resp.Body)
	if progress != nil {
		body = &progressReader{r: body, total: resp.ContentLength, fn: progress}
	}
	if _, err := io.Copy(io.MultiWriter(tmp, hash), body); err != nil {
		tmp.Close()
		return "", transientError{fmt.Errorf("download %s: %w", ref, err)}
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if _, err := verifyZip(tmp.Name()); err != nil {
		return "", transientError{fmt.Errorf("download %s: invalid archive: %w", ref, err)}
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// verifyZip checks that path is a readable zip and returns its SHA-256.
func verifyZip(path string) (string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	zr.Close()
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

type progressReader struct {
	r        io.Reader
	done     int64
	total    int64
	fn       func(done, total int64)
	lastEmit time.Time
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.done += int64(n)
	if err == io.EOF || time.Since(p.lastEmit) > 100*time.Millisecond {
		p.lastEmit = time.Now()
		p.fn(p.done, p.total)
	}
	return n, err
}
