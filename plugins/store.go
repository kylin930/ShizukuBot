package plugins

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Package is installed code only; operational settings and state are kept separately.
type Package struct {
	Manifest  Manifest `json:"manifest"`
	Directory string   `json:"-"`
}
type Store struct {
	Root string
	mu   sync.Mutex
}

func NewStore(root string) (*Store, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(abs, 0700); err != nil {
		return nil, err
	}
	return &Store{Root: abs}, nil
}
func portablePath(name string) bool {
	if !fs.ValidPath(name) || strings.ContainsAny(name, "\\:") || strings.HasPrefix(name, ".") {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		stem := strings.ToUpper(strings.Split(part, ".")[0])
		if strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") || stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" || strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT") {
			return false
		}
	}
	return true
}
func readManifest(dir string) (Manifest, error) {
	var m Manifest
	root, err := os.OpenRoot(dir)
	if err != nil {
		return m, err
	}
	defer root.Close()
	b, err := root.ReadFile("manifest.json")
	if err != nil {
		return m, err
	}
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&m); err != nil {
		return m, err
	}
	if err = m.Validate(); err != nil {
		return m, err
	}
	if m.Runtime == "javascript" {
		if !portablePath(m.Entry) {
			return m, errors.New("invalid entry path")
		}
		f, err := root.Open(m.Entry)
		if err != nil {
			return m, err
		}
		defer f.Close()
		stat, err := f.Stat()
		if err != nil || !stat.Mode().IsRegular() {
			return m, errors.New("entry must be regular file")
		}
	}
	return m, nil
}
func (s *Store) Get(id string) (Package, error) {
	if !ValidID(id) {
		return Package{}, errors.New("invalid plugin id")
	}
	dir := filepath.Join(s.Root, id)
	m, err := readManifest(dir)
	if err != nil {
		return Package{}, err
	}
	if m.ID != id {
		return Package{}, errors.New("manifest id differs from directory")
	}
	return Package{m, dir}, nil
}
func (s *Store) List() ([]Package, error) {
	entries, err := os.ReadDir(s.Root)
	if err != nil {
		return nil, err
	}
	out := []Package{}
	for _, e := range entries {
		if !e.IsDir() || !ValidID(e.Name()) {
			continue
		}
		p, err := s.Get(e.Name())
		if err != nil {
			return nil, fmt.Errorf("plugin %s: %w", e.Name(), err)
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Manifest.ID < out[j].Manifest.ID })
	return out, nil
}

// Install validates an untrusted ZIP in a private staging directory; existing IDs are never overwritten.
func (s *Store) Install(data []byte) (Package, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(data) > 4<<20 {
		return Package{}, errors.New("package too large")
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Package{}, err
	}
	if len(archive.File) > 128 {
		return Package{}, errors.New("too many package files")
	}
	staging, err := os.MkdirTemp(s.Root, ".install-")
	if err != nil {
		return Package{}, err
	}
	defer os.RemoveAll(staging)
	root, err := os.OpenRoot(staging)
	if err != nil {
		return Package{}, err
	}
	defer root.Close()
	total := int64(0)
	seen := map[string]bool{}
	for _, f := range archive.File {
		name := strings.TrimSuffix(f.Name, "/")
		key := strings.ToLower(name)
		if !portablePath(name) || seen[key] || strings.Split(name, "/")[0] == "node_modules" || !f.Mode().IsRegular() && !f.FileInfo().IsDir() {
			return Package{}, errors.New("unsafe or duplicate package path")
		}
		seen[key] = true
		if f.FileInfo().IsDir() {
			if err = root.MkdirAll(name, 0700); err != nil {
				return Package{}, err
			}
			continue
		}
		if f.UncompressedSize64 > 1<<20 {
			return Package{}, errors.New("file too large")
		}
		if err = root.MkdirAll(path.Dir(name), 0700); err != nil {
			return Package{}, err
		}
		reader, err := f.Open()
		if err != nil {
			return Package{}, err
		}
		b, err := io.ReadAll(io.LimitReader(reader, (1<<20)+1))
		reader.Close()
		if err != nil {
			return Package{}, err
		}
		total += int64(len(b))
		if len(b) > 1<<20 || total > 4<<20 {
			return Package{}, errors.New("expanded package too large")
		}
		if err = root.WriteFile(name, b, 0600); err != nil {
			return Package{}, err
		}
	}
	m, err := readManifest(staging)
	if err != nil {
		return Package{}, err
	}
	if m.Runtime != "javascript" {
		return Package{}, errors.New("Go extensions must be statically registered")
	}
	if _, err = root.Stat("package.json"); os.IsNotExist(err) {
		err = root.WriteFile("package.json", []byte(`{"type":"module"}`), 0600)
		if err != nil {
			return Package{}, err
		}
	} else if err != nil {
		return Package{}, err
	} else {
		b, err := root.ReadFile("package.json")
		if err != nil {
			return Package{}, err
		}
		var pkg struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(b, &pkg) != nil || pkg.Type != "module" {
			return Package{}, errors.New("package.json must use type module")
		}
	}
	destination := filepath.Join(s.Root, m.ID)
	if _, err = os.Lstat(destination); !os.IsNotExist(err) {
		return Package{}, errors.New("plugin id already installed")
	}
	if err = os.Rename(staging, destination); err != nil {
		return Package{}, err
	}
	return s.Get(m.ID)
}
func (s *Store) Export(id string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(p.Directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	err = fs.WalkDir(root.FS(), ".", func(name string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() {
			return nil
		}
		if !portablePath(name) || e.Type()&os.ModeSymlink != 0 {
			return errors.New("unsafe package file")
		}
		b, err := root.ReadFile(name)
		if err != nil {
			return err
		}
		writer, err := z.Create(name)
		if err != nil {
			return err
		}
		_, err = writer.Write(b)
		return err
	})
	if err != nil {
		return nil, err
	}
	if err = z.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Remove is deliberately package-only; settings/state backups are not silently deleted.
func (s *Store) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !ValidID(id) {
		return errors.New("invalid id")
	}
	return os.RemoveAll(filepath.Join(s.Root, id))
}
