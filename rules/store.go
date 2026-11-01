package rules

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"sync"
)

const maxFileBytes = 2 << 20

// Store persists one versioned file per Bot under an os.Root. Rule IDs never
// participate in paths. Use a single Engine writer for a running data directory.
type Store struct {
	root     *os.Root
	once     sync.Once
	closeErr error
}

// NewStore opens a scoped directory without changing existing rule files.
func NewStore(directory string) (*Store, error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	return &Store{root: root}, nil
}
func filename(bot string) string {
	sum := sha256.Sum256([]byte(bot))
	return hex.EncodeToString(sum[:]) + ".json"
}
func (s *Store) load(ctx context.Context, bot string) (Set, error) {
	out := Set{SchemaVersion: SchemaVersion, BotID: bot, Rules: []Rule{}}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	info, err := s.root.Lstat(filename(bot))
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	if !info.Mode().IsRegular() {
		return out, errors.New("rules file must be regular")
	}
	file, err := s.root.Open(filename(bot))
	if err != nil {
		return out, err
	}
	raw, readErr := io.ReadAll(io.LimitReader(file, maxFileBytes+1))
	closeErr := file.Close()
	if err = errors.Join(readErr, closeErr); err != nil {
		return out, err
	}
	if len(raw) > maxFileBytes {
		return out, errors.New("rules file exceeds quota")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&out); err != nil {
		return out, errors.New("invalid rules file")
	}
	if err = decoder.Decode(new(any)); err != io.EOF {
		return out, errors.New("invalid trailing rules data")
	}
	if out.SchemaVersion != SchemaVersion || out.BotID != bot || out.Rules == nil {
		return out, errors.New("incompatible rules version or Bot scope")
	}
	prepared, err := Prepare(out.Rules)
	if err != nil {
		return out, err
	}
	out.Rules = prepared
	return out, nil
}
func (s *Store) commit(ctx context.Context, set Set) (err error) {
	if err = ctx.Err(); err != nil {
		return err
	}
	if info, statErr := s.root.Lstat(filename(set.BotID)); statErr == nil && !info.Mode().IsRegular() {
		return errors.New("existing rules path preserved")
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return statErr
	}
	raw, err := json.MarshalIndent(set, "", "  ")
	if err != nil {
		return err
	}
	if len(raw) > maxFileBytes {
		return errors.New("rules file exceeds quota")
	}
	var token [16]byte
	if _, err = rand.Read(token[:]); err != nil {
		return err
	}
	name := ".rules-" + hex.EncodeToString(token[:])
	file, err := s.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer func() {
		if cleanupErr := s.root.Remove(name); cleanupErr != nil && !os.IsNotExist(cleanupErr) {
			err = errors.Join(err, cleanupErr)
		}
	}()
	if _, err = file.Write(raw); err != nil {
		return errors.Join(err, file.Close())
	}
	if err = file.Sync(); err != nil {
		return errors.Join(err, file.Close())
	}
	if err = file.Close(); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	// Rename is the commit point. Memory is published only after this succeeds.
	return s.root.Rename(name, filename(set.BotID))
}

// Close releases the scoped directory descriptor and is idempotent.
func (s *Store) Close() error { s.once.Do(func() { s.closeErr = s.root.Close() }); return s.closeErr }
