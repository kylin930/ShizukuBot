package core

import (
	"fmt"
	"sync"
	"time"

	"shizukubot/plugin"
)

type BotManager struct {
	bots     map[string]*Bot
	mu       sync.RWMutex
	logMgr   *LogManager
}

func NewBotManager() *BotManager {
	return &BotManager{
		bots:    make(map[string]*Bot),
		logMgr:  NewLogManager(1000, LogLevelDebug),
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

	bot.SetLogManager(bm.logMgr)
	bm.bots[bot.ID] = bot

	bm.logMgr.Info(bot.ID, fmt.Sprintf("bot %s added", bot.ID))
	return nil
}

func (bm *BotManager) RemoveBot(id string) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	bot, exists := bm.bots[id]
	if !exists {
		return fmt.Errorf("bot %s not found", id)
	}

	bot.Stop()
	delete(bm.bots, id)

	bm.logMgr.Info(id, fmt.Sprintf("bot %s removed", id))
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

func (bm *BotManager) SetEngine(engine *plugin.ExecutionEngine) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	for _, bot := range bm.bots {
		bot.Engine = engine
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
	bot.PluginIDs = pluginIDs
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
	return bot.PluginIDs, nil
}

// SetBotConfig 设置bot的独立配置
func (bm *BotManager) SetBotConfig(botID string, pluginIDs []string, commandPrefix string, adminUsers []string) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	bot, exists := bm.bots[botID]
	if !exists {
		return fmt.Errorf("bot %s not found", botID)
	}
	bot.PluginIDs = pluginIDs
	if commandPrefix != "" {
		bot.CommandPrefix = commandPrefix
	}
	bot.AdminUsers = adminUsers
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
	return bot.PluginIDs, bot.CommandPrefix, bot.AdminUsers, nil
}

func (bm *BotManager) GetTotalStats() *Stats {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	total := NewStats()
	for _, bot := range bm.bots {
		total.UserCount += bot.Stats.UserCount
		total.GroupCount += bot.Stats.GroupCount
		total.TodayDAU += bot.Stats.TodayDAU
		total.SentMessages += bot.Stats.SentMessages
		total.ReceivedMessages += bot.Stats.ReceivedMessages

		for hour, stat := range bot.Stats.HourlyStats {
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
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	total := NewStats()
	for _, bot := range bm.bots {
		for hour, stat := range bot.Stats.HourlyStats {
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