package plugin

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "github.com/mattn/go-sqlite3"
)

var listDB *sql.DB
var listDBMu sync.Mutex
var dbOnce sync.Once

func initListDB() error {
	var err error
	dbOnce.Do(func() {
		dataDir := filepath.Join(".", "data")
		if err = os.MkdirAll(dataDir, 0755); err != nil {
			return
		}
		dbPath := filepath.Join(dataDir, "lists.db")
		listDB, err = sql.Open("sqlite3", dbPath+"?cache=shared")
		if err != nil {
			return
		}
		listDB.SetMaxOpenConns(1)
		_, err = listDB.Exec(`
			CREATE TABLE IF NOT EXISTS lists (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				plugin_id TEXT NOT NULL,
				created_at INTEGER NOT NULL
			)
		`)
		if err != nil {
			return
		}
		_, err = listDB.Exec(`
			CREATE TABLE IF NOT EXISTS list_items (
				list_id TEXT NOT NULL,
				index_num INTEGER NOT NULL,
				value TEXT NOT NULL,
				PRIMARY KEY (list_id, index_num),
				FOREIGN KEY (list_id) REFERENCES lists(id) ON DELETE CASCADE
			)
		`)
	})
	return err
}

type ListStore struct{}

func (ls *ListStore) CreateList(listID, name, pluginID string) error {
	if err := initListDB(); err != nil {
		return err
	}
	listDBMu.Lock()
	defer listDBMu.Unlock()
	_, err := listDB.Exec(
		"INSERT INTO lists (id, name, plugin_id, created_at) VALUES (?, ?, ?, strftime('%s', 'now'))",
		listID, name, pluginID,
	)
	return err
}

func (ls *ListStore) DeleteList(listID string) error {
	if err := initListDB(); err != nil {
		return err
	}
	listDBMu.Lock()
	defer listDBMu.Unlock()
	_, err := listDB.Exec("DELETE FROM lists WHERE id = ?", listID)
	return err
}

func (ls *ListStore) AddItem(listID string, value string) error {
	if err := initListDB(); err != nil {
		return err
	}
	listDBMu.Lock()
	defer listDBMu.Unlock()
	tx, err := listDB.Begin()
	if err != nil {
		return err
	}
	var maxIndex int
	err = tx.QueryRow("SELECT COALESCE(MAX(index_num), -1) FROM list_items WHERE list_id = ?", listID).Scan(&maxIndex)
	if err != nil {
		tx.Rollback()
		return err
	}
	_, err = tx.Exec("INSERT INTO list_items (list_id, index_num, value) VALUES (?, ?, ?)", listID, maxIndex+1, value)
	if err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (ls *ListStore) AddItems(listID string, values []string) error {
	if err := initListDB(); err != nil {
		return err
	}
	listDBMu.Lock()
	defer listDBMu.Unlock()
	tx, err := listDB.Begin()
	if err != nil {
		return err
	}
	var maxIndex int
	err = tx.QueryRow("SELECT COALESCE(MAX(index_num), -1) FROM list_items WHERE list_id = ?", listID).Scan(&maxIndex)
	if err != nil {
		tx.Rollback()
		return err
	}
	for i, v := range values {
		_, err = tx.Exec("INSERT INTO list_items (list_id, index_num, value) VALUES (?, ?, ?)", listID, maxIndex+1+i, v)
		if err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (ls *ListStore) AddItemRepeat(listID string, value string, count int) error {
	if err := initListDB(); err != nil {
		return err
	}
	listDBMu.Lock()
	defer listDBMu.Unlock()
	tx, err := listDB.Begin()
	if err != nil {
		return err
	}
	var maxIndex int
	err = tx.QueryRow("SELECT COALESCE(MAX(index_num), -1) FROM list_items WHERE list_id = ?", listID).Scan(&maxIndex)
	if err != nil {
		tx.Rollback()
		return err
	}
	for i := 0; i < count; i++ {
		_, err = tx.Exec("INSERT INTO list_items (list_id, index_num, value) VALUES (?, ?, ?)", listID, maxIndex+1+i, value)
		if err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (ls *ListStore) GetItems(listID string) ([]string, error) {
	if err := initListDB(); err != nil {
		return nil, err
	}
	listDBMu.Lock()
	defer listDBMu.Unlock()
	rows, err := listDB.Query("SELECT value FROM list_items WHERE list_id = ? ORDER BY index_num", listID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		items = append(items, v)
	}
	return items, nil
}

func (ls *ListStore) GetItem(listID string, index int) (string, error) {
	if err := initListDB(); err != nil {
		return "", err
	}
	listDBMu.Lock()
	defer listDBMu.Unlock()
	var v string
	err := listDB.QueryRow("SELECT value FROM list_items WHERE list_id = ? AND index_num = ?", listID, index).Scan(&v)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("索引越界")
	}
	return v, err
}

func (ls *ListStore) SetItem(listID string, index int, value string) error {
	if err := initListDB(); err != nil {
		return err
	}
	listDBMu.Lock()
	defer listDBMu.Unlock()
	result, err := listDB.Exec("UPDATE list_items SET value = ? WHERE list_id = ? AND index_num = ?", value, listID, index)
	if err != nil {
		return err
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("索引越界")
	}
	return nil
}

func (ls *ListStore) GetLength(listID string) (int, error) {
	if err := initListDB(); err != nil {
		return 0, err
	}
	listDBMu.Lock()
	defer listDBMu.Unlock()
	var count int
	err := listDB.QueryRow("SELECT COUNT(*) FROM list_items WHERE list_id = ?", listID).Scan(&count)
	return count, err
}

func (ls *ListStore) IsEmpty(listID string) (bool, error) {
	count, err := ls.GetLength(listID)
	if err != nil {
		return true, err
	}
	return count == 0, nil
}

func (ls *ListStore) FindIndex(listID string, value string) (int, error) {
	if err := initListDB(); err != nil {
		return -1, err
	}
	listDBMu.Lock()
	defer listDBMu.Unlock()
	var idx int
	err := listDB.QueryRow("SELECT index_num FROM list_items WHERE list_id = ? AND value = ? ORDER BY index_num LIMIT 1", listID, value).Scan(&idx)
	if err == sql.ErrNoRows {
		return -1, nil
	}
	return idx, err
}

func (ls *ListStore) GetSubList(listID string, start, end int) ([]string, error) {
	if err := initListDB(); err != nil {
		return nil, err
	}
	listDBMu.Lock()
	defer listDBMu.Unlock()
	rows, err := listDB.Query("SELECT value FROM list_items WHERE list_id = ? AND index_num >= ? AND index_num <= ? ORDER BY index_num", listID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		items = append(items, v)
	}
	return items, nil
}

func (ls *ListStore) Sort(listID string, byNumber bool, ascending bool) error {
	if err := initListDB(); err != nil {
		return err
	}
	listDBMu.Lock()
	defer listDBMu.Unlock()
	items, err := ls.GetItems(listID)
	if err != nil {
		return err
	}
	if byNumber {
		for i := 0; i < len(items)-1; i++ {
			for j := i + 1; j < len(items); j++ {
				var vi, vj float64
				fmt.Sscanf(items[i], "%f", &vi)
				fmt.Sscanf(items[j], "%f", &vj)
				if (ascending && vi > vj) || (!ascending && vi < vj) {
					items[i], items[j] = items[j], items[i]
				}
			}
		}
	} else {
		for i := 0; i < len(items)-1; i++ {
			for j := i + 1; j < len(items); j++ {
				if (ascending && items[i] > items[j]) || (!ascending && items[i] < items[j]) {
					items[i], items[j] = items[j], items[i]
				}
			}
		}
	}
	tx, err := listDB.Begin()
	if err != nil {
		return err
	}
	_, err = tx.Exec("DELETE FROM list_items WHERE list_id = ?", listID)
	if err != nil {
		tx.Rollback()
		return err
	}
	for i, v := range items {
		_, err = tx.Exec("INSERT INTO list_items (list_id, index_num, value) VALUES (?, ?, ?)", listID, i, v)
		if err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (ls *ListStore) ListExists(listID string) (bool, error) {
	if err := initListDB(); err != nil {
		return false, err
	}
	listDBMu.Lock()
	defer listDBMu.Unlock()
	var count int
	err := listDB.QueryRow("SELECT COUNT(*) FROM lists WHERE id = ?", listID).Scan(&count)
	return count > 0, err
}