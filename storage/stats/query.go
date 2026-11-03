package stats

import (
	"context"
	"encoding/json"
	"time"
)

func (s *Store) date() time.Time { s.mu.RLock(); loc := s.loc; s.mu.RUnlock(); return s.now().In(loc) }
func (s *Store) cutoff() string {
	o := s.Settings()
	if o.RetentionDays == 0 {
		return "0001-01-01"
	}
	return s.date().AddDate(0, 0, 1-o.RetentionDays).Format("2006-01-02")
}

// Calendar dates, rather than 24h subtraction, preserve boundaries across DST.
func Due(now time.Time, at, lastDay string) bool {
	return now.Format("2006-01-02") != lastDay && now.Format("15:04") >= at
}
func (s *Store) schedule() {
	now := s.date()
	var last string
	_ = s.db.QueryRow("SELECT value FROM metadata WHERE key='schedule_day'").Scan(&last)
	if !Due(now, s.Settings().CleanupTime, last) {
		return
	}
	// Mark attempts too: a failed scheduled cleanup is logged, not retried every second.
	_, err := s.db.Exec("INSERT INTO metadata(key,value) VALUES('schedule_day',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", now.Format("2006-01-02"))
	if err == nil {
		err = s.cleanup()
	}
	if err != nil {
		s.report(err)
	}
}
func (s *Store) cleanup() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, table := range []string{"hourly", "identities"} {
		if _, err = tx.Exec("DELETE FROM "+table+" WHERE day<?", s.cutoff()); err != nil {
			return err
		}
	}
	at := s.date().Format(time.RFC3339)
	if _, err = tx.Exec("INSERT INTO metadata(key,value) VALUES('last_cleanup',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", at); err != nil {
		return err
	}
	if err = tx.Commit(); err == nil {
		s.mu.Lock()
		s.lastCleanup = at
		s.mu.Unlock()
	}
	return err
}
func (s *Store) Cleanup(ctx context.Context) error { return s.call(ctx, s.cleanup) }
func (s *Store) Snapshot(ctx context.Context, bots []string) (Snapshot, error) {
	var result Snapshot
	err := s.call(ctx, func() error {
		ids, _ := json.Marshal(bots)
		filter := string(ids)
		today := s.date().Format("2006-01-02")
		cutoff := s.cutoff()
		if err := s.db.QueryRow("SELECT COALESCE(SUM(sent),0),COALESCE(SUM(received),0) FROM hourly WHERE day>=? AND bot IN (SELECT value FROM json_each(?))", cutoff, filter).Scan(&result.SentMessages, &result.ReceivedMessages); err != nil {
			return err
		}
		for _, field := range []struct {
			kind string
			dest *int64
		}{{"user", &result.UserCount}, {"group", &result.GroupCount}} {
			if err := s.db.QueryRow("SELECT COUNT(*) FROM (SELECT DISTINCT bot,digest FROM identities WHERE kind=? AND day>=? AND bot IN (SELECT value FROM json_each(?)))", field.kind, cutoff, filter).Scan(field.dest); err != nil {
				return err
			}
		}
		if err := s.db.QueryRow("SELECT COUNT(*) FROM identities WHERE kind='user' AND day=? AND bot IN (SELECT value FROM json_each(?))", today, filter).Scan(&result.TodayDAU); err != nil {
			return err
		}
		result.Hourly = make([]Hour, 24)
		for i := range result.Hourly {
			result.Hourly[i].Hour = s.date().Format("2006-01-02") + time.Date(2000, 1, 1, i, 0, 0, 0, time.UTC).Format(" 15:00")
		}
		rows, err := s.db.Query("SELECT hour,SUM(sent),SUM(received) FROM hourly WHERE day=? AND bot IN (SELECT value FROM json_each(?)) GROUP BY hour", today, filter)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var h int
			var sent, received int64
			if err = rows.Scan(&h, &sent, &received); err != nil {
				return err
			}
			result.Hourly[h].Sent = sent
			result.Hourly[h].Received = received
		}
		return rows.Err()
	})
	if err != nil {
		return Snapshot{}, err
	}
	return result, nil
}
func (s *Store) History(ctx context.Context, bots []string, days int) ([]Day, error) {
	if days != 7 && days != 30 && days != 90 {
		return nil, contextRangeError{}
	}
	o := s.Settings()
	if o.RetentionDays > 0 && days > o.RetentionDays {
		days = o.RetentionDays
	}
	result := make([]Day, days)
	err := s.call(ctx, func() error {
		today := s.date()
		start := today.AddDate(0, 0, 1-days)
		indices := map[string]int{}
		for i := range result {
			day := start.AddDate(0, 0, i).Format("2006-01-02")
			result[i].Date = day
			indices[day] = i
		}
		ids, _ := json.Marshal(bots)
		rows, err := s.db.Query("SELECT day,SUM(sent),SUM(received) FROM hourly WHERE day>=? AND day<=? AND bot IN (SELECT value FROM json_each(?)) GROUP BY day ORDER BY day", result[0].Date, today.Format("2006-01-02"), string(ids))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var d Day
			if err = rows.Scan(&d.Date, &d.Sent, &d.Received); err != nil {
				return err
			}
			if i, ok := indices[d.Date]; ok {
				result[i] = d
			}
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

type contextRangeError struct{}

func (contextRangeError) Error() string { return "history days must be 7, 30 or 90" }
