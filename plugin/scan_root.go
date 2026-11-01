package plugin

import (
	"errors"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ScanDataSource confines administrative file scans to the data tree, including symlinks.
func ScanDataSource(path string, directory bool) (*ScanResult, error) {
	path = filepath.Clean(path)
	path = strings.TrimPrefix(path, "data"+string(filepath.Separator))
	if !filepath.IsLocal(path) {
		return nil, errors.New("source path outside data")
	}
	root, err := os.OpenRoot("data")
	if err != nil {
		return nil, err
	}
	defer root.Close()
	scanner := &codeScanner{fset: token.NewFileSet(), result: &ScanResult{Functions: make([]FunctionSpec, 0)}}
	if !directory {
		b, err := root.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if len(b) > 2<<20 {
			return nil, errors.New("source too large")
		}
		return scanner.scanBytes(path, b)
	}
	var all strings.Builder
	err = fs.WalkDir(root.FS(), filepath.ToSlash(path), func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		data, err := root.ReadFile(name)
		if err != nil {
			return err
		}
		if all.Len()+len(data) > 2<<20 {
			return errors.New("source too large")
		}
		src := string(data)
		if i := strings.Index(src, "package "); i >= 0 {
			if n := strings.Index(src[i:], "\n"); n > 0 {
				src = src[i+n+1:]
			}
		}
		all.WriteString(src)
		all.WriteByte('\n')
		_, err = scanner.scanBytes(name, data)
		return err
	})
	if err != nil {
		return nil, err
	}
	scanner.result.AllSource = all.String()
	return scanner.result, nil
}
