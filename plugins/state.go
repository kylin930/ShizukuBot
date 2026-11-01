package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// AtomicJSON commits complete state/config files in the destination filesystem.
func AtomicJSON(file string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return atomicBytes(file, data)
}
func atomicBytes(file string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(file), ".write-")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(name, file)
}
func scope(bot string) string { h := sha256.Sum256([]byte(bot)); return hex.EncodeToString(h[:]) }

type diskState struct {
	file string
	mu   sync.Mutex
}

func (s *diskState) read() (map[string]json.RawMessage, error) {
	out := map[string]json.RawMessage{}
	b, err := os.ReadFile(s.file)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}
func (s *diskState) Get(ctx context.Context, key string) (json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	v, err := s.read()
	if err != nil {
		return nil, err
	}
	return v[key], nil
}
func (s *diskState) Set(ctx context.Context, key string, value json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if key == "" || len(key) > 128 || len(value) > 65536 || !json.Valid(value) {
		return errors.New("invalid state value")
	}
	v, err := s.read()
	if err != nil {
		return err
	}
	v[key] = append(json.RawMessage(nil), value...)
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(b) > 1<<20 {
		return errors.New("state quota exceeded")
	}
	return atomicBytes(s.file, b)
}
