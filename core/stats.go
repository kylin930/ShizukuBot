package core

import (
	"context"
	persist "shizukubot/storage/stats"
	"sync"
	"time"
)

type HourlyStat struct {
	Hour     string
	Sent     int64
	Received int64
}

type Stats struct {
	UserCount        int64
	GroupCount       int64
	TodayDAU         int64
	QPS              int64
	SentMessages     int64
	ReceivedMessages int64

	HourlyStats map[string]*HourlyStat

	// 去重计数集合（累计，只增不减）
	seenUsers  map[string]struct{}
	seenGroups map[string]struct{}
	// 今日活跃用户集合（跨天清零）
	todayDate  string
	todayUsers map[string]struct{}

	store     *persist.Store
	botID     string
	qpsWindow *SlidingWindow
	mu        sync.RWMutex
}

type SlidingWindow struct {
	buckets      []int64
	windowSize   int
	bucketSize   time.Duration
	currentIndex int
	total        int64
	lastTick     int64
	now          func() time.Time
	mu           sync.RWMutex
}

func NewSlidingWindow(windowSize int, bucketSize time.Duration) *SlidingWindow {
	return &SlidingWindow{
		buckets:    make([]int64, windowSize),
		windowSize: windowSize,
		bucketSize: bucketSize,
		now:        time.Now,
	}
}

func (w *SlidingWindow) Add(count int64) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.advance()
	expectedIndex := w.currentIndex

	w.buckets[expectedIndex] += count
	w.total += count
}

func (w *SlidingWindow) advance() {
	tick := w.now().UnixNano() / int64(w.bucketSize)
	delta := tick - w.lastTick
	if w.lastTick == 0 || delta >= int64(w.windowSize) || delta < 0 {
		clear(w.buckets)
		w.total = 0
	} else {
		for i := int64(1); i <= delta; i++ {
			index := int((w.lastTick + i) % int64(w.windowSize))
			w.total -= w.buckets[index]
			w.buckets[index] = 0
		}
	}
	w.currentIndex = int(tick % int64(w.windowSize))
	w.lastTick = tick
}
func (w *SlidingWindow) GetQPS() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.advance()
	return w.total / int64(w.windowSize)
}
func (s *Stats) AttachStore(store *persist.Store, bot string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.store = store
	s.botID = bot
}
func (s *Stats) persistent(kind, id string) bool {
	s.mu.RLock()
	store, bot := s.store, s.botID
	s.mu.RUnlock()
	if store == nil {
		return false
	}
	store.Record(persist.Event{Bot: bot, Kind: kind, Identity: id})
	return true
}

func NewStats() *Stats {
	return &Stats{
		HourlyStats: make(map[string]*HourlyStat),
		seenUsers:   make(map[string]struct{}),
		seenGroups:  make(map[string]struct{}),
		todayUsers:  make(map[string]struct{}),
		todayDate:   time.Now().Format("2006-01-02"),
		qpsWindow:   NewSlidingWindow(60, time.Second),
	}
}

// RecordUser 记录一个接触过的用户，返回是否为新增（用于累计 UserCount）
func (s *Stats) RecordUser(userID string) {
	if s.persistent("user", userID) {
		return
	}
	if userID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	// 跨天清零今日DAU
	today := time.Now().Format("2006-01-02")
	if s.todayDate != today {
		s.todayDate = today
		s.todayUsers = make(map[string]struct{})
		s.TodayDAU = 0
	}

	// 累计独立用户
	if _, ok := s.seenUsers[userID]; !ok {
		s.seenUsers[userID] = struct{}{}
		s.UserCount = int64(len(s.seenUsers))
	}
	// 今日活跃用户
	if _, ok := s.todayUsers[userID]; !ok {
		s.todayUsers[userID] = struct{}{}
		s.TodayDAU = int64(len(s.todayUsers))
	}
}

// RecordGroup 记录一个接触过的群，返回是否为新增（用于累计 GroupCount）
func (s *Stats) RecordGroup(groupID string) {
	if s.persistent("group", groupID) {
		return
	}
	if groupID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.seenGroups[groupID]; !ok {
		s.seenGroups[groupID] = struct{}{}
		s.GroupCount = int64(len(s.seenGroups))
	}
}

func (s *Stats) IncReceived() {
	if s.persistent("received", "") {
		s.qpsWindow.Add(1)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ReceivedMessages++
	s.qpsWindow.Add(1)
	s.QPS = s.qpsWindow.GetQPS()

	hour := time.Now().Format("2006-01-02 15:00")
	if _, ok := s.HourlyStats[hour]; !ok {
		s.HourlyStats[hour] = &HourlyStat{Hour: hour}
	}
	s.HourlyStats[hour].Received++
}

func (s *Stats) IncSent() {
	if s.persistent("sent", "") {
		s.qpsWindow.Add(1)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	s.SentMessages++
	s.qpsWindow.Add(1)
	s.QPS = s.qpsWindow.GetQPS()

	hour := time.Now().Format("2006-01-02 15:00")
	if _, ok := s.HourlyStats[hour]; !ok {
		s.HourlyStats[hour] = &HourlyStat{Hour: hour}
	}
	s.HourlyStats[hour].Sent++
}

func (s *Stats) GetTodayStats() *HourlyStat {
	s.mu.RLock()
	defer s.mu.RUnlock()

	today := time.Now().Format("2006-01-02")
	total := &HourlyStat{Hour: today}

	for hour, stat := range s.HourlyStats {
		if len(hour) >= len(today) && hour[:len(today)] == today {
			total.Sent += stat.Sent
			total.Received += stat.Received
		}
	}

	return total
}

func (s *Stats) ResetDAU() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TodayDAU = 0
}

// Snapshot returns independent counters and hourly values under the statistics lock.
func (s *Stats) Snapshot() *Stats {
	s.mu.RLock()
	store, bot := s.store, s.botID
	s.mu.RUnlock()
	if store != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		snapshot, err := store.Snapshot(ctx, []string{bot})
		v := NewStats()
		v.QPS = s.qpsWindow.GetQPS()
		if err == nil {
			v.UserCount = snapshot.UserCount
			v.GroupCount = snapshot.GroupCount
			v.TodayDAU = snapshot.TodayDAU
			v.SentMessages = snapshot.SentMessages
			v.ReceivedMessages = snapshot.ReceivedMessages
			for _, h := range snapshot.Hourly {
				v.HourlyStats[h.Hour] = &HourlyStat{Hour: h.Hour, Sent: h.Sent, Received: h.Received}
			}
		}
		return v
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	v := NewStats()
	v.UserCount = s.UserCount
	v.GroupCount = s.GroupCount
	if s.todayDate == time.Now().Format("2006-01-02") {
		v.TodayDAU = s.TodayDAU
	}
	v.QPS = s.qpsWindow.GetQPS()
	v.SentMessages = s.SentMessages
	v.ReceivedMessages = s.ReceivedMessages
	for k, h := range s.HourlyStats {
		copy := *h
		v.HourlyStats[k] = &copy
	}
	return v
}

func (s *Stats) RealtimeQPS() int64 { return s.qpsWindow.GetQPS() }
