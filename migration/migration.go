package migration

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"shizukubot/plugins"
	"sort"
	"strings"
	"time"
)

type Options struct {
	Source, CustomNodes, Destination, Backups string
	Apply                                     bool
}
type Item struct {
	File          string   `json:"file"`
	ID            string   `json:"id"`
	NewID         string   `json:"new_id"`
	BotID         string   `json:"bot_id"`
	SHA256        string   `json:"sha256"`
	Status        string   `json:"status"`
	Reason        string   `json:"reason,omitempty"`
	Capabilities  []string `json:"capabilities"`
	GeneratedHash string   `json:"generated_hash,omitempty"`
}
type Report struct {
	Version     int    `json:"version"`
	Backup      string `json:"backup,omitempty"`
	Destination string `json:"destination"`
	Items       []Item `json:"items"`
}

func digest(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }
func snapshot(root string) (map[string][]byte, error) {
	out := map[string][]byte{}
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return out, nil
	}
	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	total := 0
	err = fs.WalkDir(r.FS(), ".", func(name string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() {
			return nil
		}
		if !e.Type().IsRegular() {
			return errors.New("backup refuses symlinks or special files")
		}
		b, err := r.ReadFile(name)
		if err != nil {
			return err
		}
		total += len(b)
		if total > 64<<20 {
			return errors.New("backup exceeds 64 MiB; use an external verified backup")
		}
		out[name] = b
		return nil
	})
	return out, err
}

// Run never changes original files. Apply backs up exact bytes before installing disabled code.
func Run(o Options) (Report, error) {
	dest, err := filepath.Abs(o.Destination)
	report := Report{Version: 1, Destination: dest, Items: []Item{}}
	if err != nil {
		return report, err
	}
	source, err := snapshot(o.Source)
	if err != nil {
		return report, err
	}
	custom, err := snapshot(o.CustomNodes)
	if err != nil {
		return report, err
	}
	names := []string{}
	for name := range source {
		if filepath.Ext(name) == ".json" && !strings.Contains(name, "/") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	type output struct {
		manifest plugins.Manifest
		code     string
	}
	outputs := map[string]output{}
	for _, name := range names {
		raw := source[name]
		item := Item{File: name, SHA256: digest(raw), Status: "manual"}
		var old legacy
		if json.Unmarshal(raw, &old) != nil {
			item.Reason = "invalid legacy JSON"
			report.Items = append(report.Items, item)
			continue
		}
		item.ID = old.ID
		item.BotID = old.BotID
		item.NewID = old.ID
		if !plugins.ValidID(item.NewID) || strings.HasPrefix(item.NewID, "builtin-") {
			item.NewID = "legacy-" + item.SHA256[:16]
		}
		mf, code, caps, err := convert(old, item.NewID)
		item.Capabilities = caps
		if err == nil {
			for _, raw := range custom {
				var definition struct {
					Type string `json:"type"`
				}
				if json.Unmarshal(raw, &definition) == nil {
					for _, capability := range caps {
						if definition.Type == capability {
							err = errors.New("global custom node overrides a used capability; manual review required")
						}
					}
				}
			}
		}
		if err != nil {
			item.Reason = err.Error()
		} else {
			item.Status = "convertible"
			outputs[name] = output{mf, code}
		}
		if _, err = os.Lstat(filepath.Join(dest, item.NewID)); err == nil {
			item.Status = "conflict"
			item.Reason = "destination exists; never overwrite"
			b, readErr := os.ReadFile(filepath.Join(dest, item.NewID, "migration.json"))
			var prior struct {
				SHA string `json:"source_sha256"`
			}
			if readErr == nil && json.Unmarshal(b, &prior) == nil && prior.SHA == item.SHA256 {
				item.Status = "already-migrated"
				item.Reason = ""
			}
		}
		report.Items = append(report.Items, item)
	}
	counts := map[string]int{}
	for _, item := range report.Items {
		if item.Status == "convertible" {
			counts[item.NewID]++
		}
	}
	for i, item := range report.Items {
		if counts[item.NewID] > 1 {
			report.Items[i].Status = "manual"
			report.Items[i].Reason = "duplicate legacy IDs; manual reconciliation required"
		}
	}
	if !o.Apply {
		return report, nil
	}
	if err = os.MkdirAll(o.Backups, 0700); err != nil {
		return report, err
	}
	lock, err := os.OpenFile(filepath.Join(o.Backups, ".migration.lock"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return report, errors.New("another migration may be running; inspect migration lock")
	}
	lock.Close()
	defer os.Remove(filepath.Join(o.Backups, ".migration.lock"))
	backup, err := os.MkdirTemp(o.Backups, time.Now().Format("20060102-150405")+"-")
	if err != nil {
		return report, err
	}
	backup, err = filepath.Abs(backup)
	if err != nil {
		return report, err
	}
	report.Backup = backup
	sums := map[string]string{}
	for prefix, files := range map[string]map[string][]byte{"plugins": source, "custom_nodes": custom} {
		for name, b := range files {
			file := filepath.Join(backup, prefix, filepath.FromSlash(name))
			if err = os.MkdirAll(filepath.Dir(file), 0700); err != nil {
				return report, err
			}
			if err = os.WriteFile(file, b, 0600); err != nil {
				return report, err
			}
			verify, err := os.ReadFile(file)
			if err != nil || digest(verify) != digest(b) {
				return report, errors.New("backup verification failed")
			}
			sums[prefix+"/"+name] = digest(b)
		}
	}
	if err = plugins.AtomicJSON(filepath.Join(backup, "checksums.json"), sums); err != nil {
		return report, err
	}
	if err = plugins.AtomicJSON(filepath.Join(backup, "report.json"), report); err != nil {
		return report, err
	}
	store, err := plugins.NewStore(dest)
	if err != nil {
		return report, err
	}
	for i, item := range report.Items {
		if item.Status != "convertible" {
			continue
		}
		out := outputs[item.File]
		manifest, _ := json.MarshalIndent(out.manifest, "", "  ")
		info, _ := json.Marshal(map[string]string{"source_sha256": item.SHA256, "source_bot_id": item.BotID, "backup_id": filepath.Base(backup)})
		var buf bytes.Buffer
		z := zip.NewWriter(&buf)
		for name, b := range map[string][]byte{"manifest.json": manifest, "dist/index.js": []byte(out.code), "src/index.js": []byte(out.code), "migration.json": info} {
			w, err := z.Create(name)
			if err != nil {
				return report, err
			}
			if _, err = w.Write(b); err != nil {
				return report, err
			}
		}
		if err = z.Close(); err != nil {
			return report, err
		}
		expected := map[string][]byte{"manifest.json": manifest, "dist/index.js": []byte(out.code), "src/index.js": []byte(out.code), "migration.json": info, "package.json": []byte(`{"type":"module"}`)}
		report.Items[i].Status = "conversion-pending"
		report.Items[i].GeneratedHash = treeHash(expected)
		if err = plugins.AtomicJSON(filepath.Join(backup, "report.json"), report); err != nil {
			return report, err
		}
		if _, err = store.Install(buf.Bytes()); err != nil {
			return report, err
		}
		tree, err := snapshot(filepath.Join(dest, item.NewID))
		if err != nil {
			return report, err
		}
		report.Items[i].Status = "converted-disabled"
		report.Items[i].GeneratedHash = treeHash(tree)
		if err = plugins.AtomicJSON(filepath.Join(backup, "report.json"), report); err != nil {
			return report, err
		}
	}
	return report, nil
}
func treeHash(files map[string][]byte) string {
	names := []string{}
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	hash := sha256.New()
	for _, name := range names {
		fmt.Fprintf(hash, "%s\x00%s\n", name, digest(files[name]))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// Rollback removes only unmodified generated projects; originals have never been altered.
// Stop the bot host first so no worker is running a package being removed.
func Rollback(backup, destination string) error {
	data, err := os.ReadFile(filepath.Join(backup, "report.json"))
	if err != nil {
		return err
	}
	var report Report
	if json.Unmarshal(data, &report) != nil || report.Version != 1 {
		return errors.New("invalid backup report")
	}
	expected, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	if report.Destination != expected {
		return errors.New("backup destination differs from explicit rollback destination")
	}
	if !filepath.IsAbs(report.Destination) {
		return errors.New("invalid destination")
	}
	for _, item := range report.Items {
		if item.Status != "converted-disabled" && item.Status != "conversion-pending" {
			continue
		}
		if !plugins.ValidID(item.NewID) {
			return errors.New("invalid generated id")
		}
		if filepath.Base(item.File) != item.File {
			return errors.New("invalid backup source path")
		}
		original, err := os.ReadFile(filepath.Join(backup, "plugins", item.File))
		if err != nil || digest(original) != item.SHA256 {
			return errors.New("original backup failed checksum verification")
		}
		dir := filepath.Join(report.Destination, item.NewID)
		if _, err = os.Stat(dir); os.IsNotExist(err) {
			continue
		}
		files, err := snapshot(dir)
		if err != nil {
			return err
		}
		if treeHash(files) != item.GeneratedHash {
			return errors.New("generated plugin edited; refusing to overwrite user work")
		}
	}
	for _, item := range report.Items {
		if item.Status == "converted-disabled" || item.Status == "conversion-pending" {
			if err = os.RemoveAll(filepath.Join(report.Destination, item.NewID)); err != nil {
				return err
			}
		}
	}
	return nil
}
