package core

import (
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

	qpsWindow *SlidingWindow
	mu        sync.RWMutex
}

type SlidingWindow struct {
	buckets     []int64
	windowSize  int
	bucketSize  time.Duration
	currentIndex int
	total       int64
	mu          sync.RWMutex
}

func NewSlidingWindow(windowSize int, bucketSize time.Duration) *SlidingWindow {
	return &SlidingWindow{
		buckets:    make([]int64, windowSize),
		windowSize: windowSize,
		bucketSize: bucketSize,
	}
}

func (w *SlidingWindow) Add(count int64) {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now()
	expectedIndex := int(now.Unix() / int64(w.bucketSize.Seconds())) % w.windowSize

	if expectedIndex != w.currentIndex {
		steps := (expectedIndex - w.currentIndex + w.windowSize) % w.windowSize
		for i := 0; i < steps; i++ {
			nextIndex := (w.currentIndex + 1) % w.windowSize
			w.total -= w.buckets[nextIndex]
			w.buckets[nextIndex] = 0
			w.currentIndex = nextIndex
		}
	}

	w.buckets[expectedIndex] += count
	w.total += count
}

func (w *SlidingWindow) GetQPS() int64 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.total / int64(w.windowSize)
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