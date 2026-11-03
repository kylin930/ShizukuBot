package core

import (
	"fmt"
	"sync"
	"time"

	"shizukubot/events"
	persist "shizukubot/storage/stats"
)

type BotManager struct {
	statsStore *persist.Store
	bots       map[string]*Bot
	mu         sync.RWMutex
	logMgr     *LogManager
	dispatcher *events.Dispatcher
	commands   func([]string, string) []string
}

func NewBotManager() *BotManager {
	return &BotManager{
		bots:       make(map[string]*Bot),
		logMgr:     NewLogManager(1000, LogLevelDebug),
		dispatcher: events.NewDispatcher(events.NewBus()),
	}
}

func (bm *BotManager) GetLogManager() *LogManager {
	return bm.logMgr
}

func (bm *BotManager) AddBot(bot *Bot) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if bot == nil {
		return fmt.Errorf("bot is nil")
	}

	if _, exists := bm.bots[bot.ID]; exists {
		return fmt.Errorf("bot %s already exists", bot.ID)
	}

	if bm.statsStore != nil {
		bot.Stats.AttachStore(bm.statsStore, bot.ID)
	}
	bot.SetLogManager(bm.logMgr)
	bot.Dispatcher = bm.dispatcher
	bot.Commands = bm.commands
	bm.bots[bot.ID] = bot

	bm.logMgr.Info(bot.ID, fmt.Sprintf("bot %s added", bot.ID))
	return nil
}

func (bm *BotManager) RemoveBot(id string) error {
	// Remove the routing entry before joining workers; never hold the manager lock
	// while a worker may call GetBot from a bus subscription.
	bm.mu.Lock()
	bot, exists := bm.bots[id]
	if exists {
		delete(bm.bots, id)
	}
	bm.mu.Unlock()
	if !exists {
		return fmt.Errorf("bot %s not found", id)
	}
	bot.Stop()
	bm.logMgr.Info(id, "bot removed")
	return nil
}

func (bm *BotManager) GetBot(id string) (*Bot, bool) {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	bot, exists := bm.bots[id]
	return bot, exists
}

func (bm *BotManager) GetAllBots() []*Bot {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	bots := make([]*Bot, 0, len(bm.bots))
	for _, bot := range bm.bots {
		bots = append(bots, bot)
	}
	return bots
}

func (bm *BotManager) StartBot(id string) error {
	bm.mu.RLock()
	bot, exists := bm.bots[id]
	bm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("bot %s not found", id)
	}

	return bot.Start()
}

func (bm *BotManager) StopBot(id string) error {
	bm.mu.RLock()
	bot, exists := bm.bots[id]
	bm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("bot %s not found", id)
	}

	bot.Stop()
	return nil
}

func (bm *BotManager) StartAll() error {
	bm.mu.RLock()
	bots := make([]*Bot, 0, len(bm.bots))
	for _, bot := range bm.bots {
		bots = append(bots, bot)
	}
	bm.mu.RUnlock()

	var wg sync.WaitGroup
	errChan := make(chan error, len(bots))

	for _, bot := range bots {
		if !bot.IsEnabled() {
			continue
		}
		wg.Add(1)
		go func(b *Bot) {
			defer wg.Done()
			if err := b.Start(); err != nil {
				errChan <- err
			}
		}(bot)
	}

	wg.Wait()
	close(errChan)

	var firstErr error
	for err := range errChan {
		if firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

func (bm *BotManager) StopAll() {
	bm.mu.RLock()
	bots := make([]*Bot, 0, len(bm.bots))
	for _, bot := range bm.bots {
		bots = append(bots, bot)
	}
	bm.mu.RUnlock()

	var wg sync.WaitGroup
	for _, bot := range bots {
		wg.Add(1)
		go func(b *Bot) {
			defer wg.Done()
			b.Stop()
		}(bot)
	}
	wg.Wait()

	bm.logMgr.Info("", "all bots stopped")
}

// Events returns the shared bot-scoped event bus.
func (bm *BotManager) Events() *events.Bus { return bm.dispatcher.Bus }

// SetCommandProvider connects optional plugin command discovery without graph dependencies.
func (bm *BotManager) SetCommandProvider(provider func([]string, string) []string) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	bm.commands = provider
	for _, bot := range bm.bots {
		bot.mu.Lock()
		bot.Commands = provider
		bot.mu.Unlock()
	}
}

// SetBotPlugins 设置bot装载的插件ID列表
func (bm *BotManager) SetBotPlugins(botID string, pluginIDs []string) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	bot, exists := bm.bots[botID]
	if !exists {
		return fmt.Errorf("bot %s not found", botID)
	}
	bot.mu.Lock()
	defer bot.mu.Unlock()
	bot.PluginIDs = append([]string(nil), pluginIDs...)
	return nil
}

// GetBotPlugins 获取bot装载的插件ID列表
func (bm *BotManager) GetBotPlugins(botID string) ([]string, error) {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	bot, exists := bm.bots[botID]
	if !exists {
		return nil, fmt.Errorf("bot %s not found", botID)
	}
	bot.mu.Lock()
	defer bot.mu.Unlock()
	return append([]string(nil), bot.PluginIDs...), nil
}

// SetBotConfig 设置bot的独立配置
func (bm *BotManager) SetBotConfig(botID string, pluginIDs []string, commandPrefix string, adminUsers []string) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	bot, exists := bm.bots[botID]
	if !exists {
		return fmt.Errorf("bot %s not found", botID)
	}
	bot.mu.Lock()
	defer bot.mu.Unlock()
	bot.PluginIDs = append([]string(nil), pluginIDs...)
	if commandPrefix != "" {
		bot.CommandPrefix = commandPrefix
	}
	bot.AdminUsers = append([]string(nil), adminUsers...)
	return nil
}

// GetBotConfig 获取bot的独立配置
func (bm *BotManager) GetBotConfig(botID string) (pluginIDs []string, commandPrefix string, adminUsers []string, err error) {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	bot, exists := bm.bots[botID]
	if !exists {
		return nil, "", nil, fmt.Errorf("bot %s not found", botID)
	}
	bot.mu.Lock()
	defer bot.mu.Unlock()
	return append([]string(nil), bot.PluginIDs...), bot.CommandPrefix, append([]string(nil), bot.AdminUsers...), nil
}

func (bm *BotManager) GetTotalStats() *Stats {
	bots := bm.GetAllBots()
	total := NewStats()
	for _, bot := range bots {
		stats := bot.Stats.Snapshot()
		total.UserCount += stats.UserCount
		total.GroupCount += stats.GroupCount
		total.TodayDAU += stats.TodayDAU
		total.QPS += stats.QPS
		total.SentMessages += stats.SentMessages
		total.ReceivedMessages += stats.ReceivedMessages

		for hour, stat := range stats.HourlyStats {
			if _, ok := total.HourlyStats[hour]; !ok {
				total.HourlyStats[hour] = &HourlyStat{Hour: hour}
			}
			total.HourlyStats[hour].Sent += stat.Sent
			total.HourlyStats[hour].Received += stat.Received
		}
	}

	return total
}

func (bm *BotManager) GetTodayHourlyStats() []*HourlyStat {
	bots := bm.GetAllBots()
	total := NewStats()
	for _, bot := range bots {
		stats := bot.Stats.Snapshot()
		for hour, stat := range stats.HourlyStats {
			if _, ok := total.HourlyStats[hour]; !ok {
				total.HourlyStats[hour] = &HourlyStat{Hour: hour}
			}
			total.HourlyStats[hour].Sent += stat.Sent
			total.HourlyStats[hour].Received += stat.Received
		}
	}

	today := time.Now().Format("2006-01-02")
	result := make([]*HourlyStat, 24)
	for i := 0; i < 24; i++ {
		hourKey := fmt.Sprintf("%s %02d:00", today, i)
		if stat, ok := total.HourlyStats[hourKey]; ok {
			result[i] = stat
		} else {
			result[i] = &HourlyStat{Hour: hourKey, Sent: 0, Received: 0}
		}
	}

	return result
}

// ReplaceBot stops the old instance before installing new credentials and options.
func (bm *BotManager) ReplaceBot(bot *Bot) error {
	old, ok := bm.GetBot(bot.ID)
	if !ok {
		return fmt.Errorf("bot not found")
	}
	old.Stop()
	bm.mu.Lock()
	defer bm.mu.Unlock()
	bot.SetLogManager(bm.logMgr)
	bot.Dispatcher = bm.dispatcher
	bot.Commands = bm.commands
	bot.Stats = old.Stats
	bm.bots[bot.ID] = bot
	return nil
}

func (bm *BotManager) SetStatsStore(store *persist.Store) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	bm.statsStore = store
	for _, bot := range bm.bots {
		bot.Stats.AttachStore(store, bot.ID)
	}
}
func (bm *BotManager) StatsStore() *persist.Store {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	return bm.statsStore
}
func (bm *BotManager) BotIDs() []string {
	bots := bm.GetAllBots()
	ids := make([]string, 0, len(bots))
	for _, bot := range bots {
		ids = append(ids, bot.ID)
	}
	return ids
}
func (bm *BotManager) QPS() int64 {
	var total int64
	for _, b := range bm.GetAllBots() {
		total += b.Stats.qpsWindow.GetQPS()
	}
	return total
}
