// Package storage handles the filesystem layout of a GoPick run: it creates
// the root project directory, resolves the target sub-directory for each
// image extension and persists downloaded payloads on disk.
package storage

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
)

const (
	// UnknownDir receives every image whose extension cannot be derived from
	// its URL.
	UnknownDir = "unknown"
	// fallbackName is used when the URL path does not expose a usable file
	// name (e.g. "https://example.com/").
	fallbackName = "image"
)

// Storage is safe for concurrent use by multiple goroutines.
type Storage struct {
	root string

	mu   sync.Mutex
	used map[string]struct{}
}

// New creates the root directory (if needed) and returns a Storage rooted at
// it. The root is created lazily with MkdirAll so nested relative paths work.
func New(root string) (*Storage, error) {
	if root == "" {
		return nil, fmt.Errorf("storage: пустой корневой каталог")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("storage: не удалось создать корневой каталог %q: %w", root, err)
	}
	return &Storage{root: root, used: make(map[string]struct{})}, nil
}

// Root returns the absolute or relative root directory of the storage.
func (s *Storage) Root() string { return s.root }

// Save streams the body of r into the sub-directory matching the image
// extension taken from rawURL. It returns the path of the written file.
func (s *Storage) Save(rawURL string, r io.Reader) (string, error) {
	ext := Extension(rawURL)
	dir := filepath.Join(s.root, ext)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("storage: не удалось создать каталог %q: %w", dir, err)
	}

	name := s.reserve(ext, fileName(rawURL, ext), rawURL)
	dst := filepath.Join(dir, name)

	f, err := os.Create(dst)
	if err != nil {
		return "", fmt.Errorf("storage: не удалось создать файл %q: %w", dst, err)
	}

	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		os.Remove(dst)
		return "", fmt.Errorf("storage: не удалось записать %q: %w", dst, err)
	}
	if err := f.Close(); err != nil {
		os.Remove(dst)
		return "", fmt.Errorf("storage: не удалось закрыть %q: %w", dst, err)
	}
	return dst, nil
}

// reserve возвращает имя файла, уникальное внутри dir. При совпадении базовых
// имён (типичный случай — чужие CDN вида /image.png) имя получает префикс хоста,
// а если и это не помогло — короткий хеш URL, чтобы параллельные загрузки не
// затирали друг друга.
func (s *Storage) reserve(dir, name, rawURL string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.claim(dir, name) {
		return name
	}

	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)

	if host := hostPrefix(rawURL); host != "" {
		for i := 1; i <= 100; i++ {
			candidate := host + "_" + stem + ext
			if i > 1 {
				candidate = fmt.Sprintf("%s_%s-%d%s", host, stem, i, ext)
			}
			if s.claim(dir, candidate) {
				return candidate
			}
		}
	}

	sum := sha1.Sum([]byte(rawURL))
	digest := hex.EncodeToString(sum[:])[:8]
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s-%s%s", stem, digest, ext)
		if i > 1 {
			candidate = fmt.Sprintf("%s-%s-%d%s", stem, digest, i, ext)
		}
		if s.claim(dir, candidate) {
			return candidate
		}
	}
}

// claim помечает имя как занятое и сообщает, было ли оно свободно.
func (s *Storage) claim(dir, name string) bool {
	key := filepath.Join(dir, name)
	if _, exists := s.used[key]; exists {
		return false
	}
	s.used[key] = struct{}{}
	return true
}

// hostPrefix превращает хост URL в пригодный для имени файла префикс.
func hostPrefix(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	host := u.Hostname()
	if host == "" {
		return ""
	}
	return sanitize(host)
}

// Extension extracts a lower-cased image extension from rawURL, or UnknownDir
// when the URL does not carry a usable one.
func Extension(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return UnknownDir
	}
	base := path.Base(u.Path)
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(base)), ".")
	if ext == "" || ext == "." || len(ext) > 10 {
		return UnknownDir
	}
	for _, r := range ext {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return UnknownDir
		}
	}
	return ext
}

// fileName builds a filesystem-safe name for rawURL inside the ext directory.
func fileName(rawURL, ext string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fallbackName
	}

	base := path.Base(u.Path)
	if base == "" || base == "." || base == "/" {
		base = fallbackName
	}
	base = sanitize(base)

	if filepath.Ext(base) == "" {
		if ext != UnknownDir {
			base += "." + ext
		}
	}
	if base == "" || base == "." {
		return fallbackName
	}
	return base
}

// sanitize strips characters that are not safe in a file name on Windows and
// POSIX alike, and collapses the reserved names "." and "..".
func sanitize(name string) string {
	name = strings.Map(func(r rune) rune {
		switch r {
		case '<', '>', ':', '"', '/', '\\', '|', '?', '*':
			return '_'
		}
		if r < 0x20 {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(strings.Trim(name, "."))
	return name
}
