package stats

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"llmrelay/backend/internal/storage"
)

// ======================== Token 统计 ========================

type ModelStats struct {
	RequestCount     int64 `json:"request_count"`
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
}

// DailyStats 单日统计，每天0点自动重置
type DailyStats struct {
	Date          string                 `json:"date"`
	TotalRequests int64                  `json:"total_requests"`
	Models        map[string]*ModelStats `json:"models"`
}

type TokenStatsData struct {
	TotalRequests       int64                  `json:"total_requests"`
	Models              map[string]*ModelStats `json:"models"`
	Daily               *DailyStats            `json:"daily,omitempty"`
	Upstreams           map[string]*ModelStats `json:"upstreams,omitempty"`
	DailyUpstreams      map[string]*ModelStats `json:"daily_upstreams,omitempty"`
	ModelUpstreams      []BreakdownStats       `json:"model_upstreams,omitempty"`
	DailyModelUpstreams []BreakdownStats       `json:"daily_model_upstreams,omitempty"`
	Days                []BreakdownStats       `json:"days,omitempty"`
}

var (
	tokenStats     = &TokenStatsData{Models: map[string]*ModelStats{}, Daily: nil}
	tokenStatsMu   sync.Mutex
	tokenStatsPath = "llmrelay.db"
	statsDate      string // 当前统计日期 YYYY-MM-DD
	statsSaveMu    sync.Mutex
	statsSaveOnce  sync.Once
	statsSaveCh    = make(chan struct{}, 1)
)

// ======================== Token 统计 ========================

func GetToday() string {
	return time.Now().Format("2006-01-02")
}

func CheckAndResetDailyStats() {
	today := GetToday()
	tokenStatsMu.Lock()
	defer tokenStatsMu.Unlock()
	if statsDate == "" {
		statsDate = today
		if tokenStats.Daily == nil || tokenStats.Daily.Date != today {
			tokenStats.Daily = &DailyStats{Date: today, Models: map[string]*ModelStats{}}
		}
		return
	}
	if statsDate != today {
		log.Printf("[统计] 日期变更 %s -> %s，重置每日统计", statsDate, today)
		statsDate = today
		tokenStats.Daily = &DailyStats{Date: today, Models: map[string]*ModelStats{}}
	}
}

func LoadTokenStats() {
	if storage.IsSQLitePath(tokenStatsPath) {
		if err := loadSQLiteStats(tokenStatsPath); err != nil {
			log.Printf("加载 SQLite 统计失败: %v", err)
		}
		return
	}
	data, err := os.ReadFile(tokenStatsPath)
	if err != nil {
		CheckAndResetDailyStats()
		return
	}
	var st TokenStatsData
	if err := json.Unmarshal(data, &st); err != nil {
		CheckAndResetDailyStats()
		return
	}
	tokenStatsMu.Lock()
	if st.Models == nil {
		st.Models = map[string]*ModelStats{}
	}
	today := GetToday()
	if st.Daily != nil && st.Daily.Date != today {
		log.Printf("[统计] 每日统计日期 %s 已过期，重置", st.Daily.Date)
		st.Daily = &DailyStats{Date: today, Models: map[string]*ModelStats{}}
	} else if st.Daily == nil {
		st.Daily = &DailyStats{Date: today, Models: map[string]*ModelStats{}}
	}
	statsDate = today
	tokenStats = &st
	tokenStatsMu.Unlock()
}

func SaveTokenStats() {
	if usingSQLite() {
		return
	}
	statsSaveMu.Lock()
	defer statsSaveMu.Unlock()
	tokenStatsMu.Lock()
	data, err := json.MarshalIndent(tokenStats, "", "  ")
	tokenStatsMu.Unlock()
	if err != nil {
		log.Printf("保存统计失败: %v", err)
		return
	}
	dir := filepath.Dir(tokenStatsPath)
	temp, err := os.CreateTemp(dir, ".stats-*.tmp")
	if err != nil {
		log.Printf("创建统计临时文件失败: %v", err)
		return
	}
	tempPath := temp.Name()
	cleanup := func() {
		temp.Close()
		os.Remove(tempPath)
	}
	if err := temp.Chmod(0644); err != nil {
		cleanup()
		log.Printf("设置统计文件权限失败: %v", err)
		return
	}
	if _, err := temp.Write(data); err != nil {
		cleanup()
		log.Printf("写入统计临时文件失败: %v", err)
		return
	}
	if err := temp.Sync(); err != nil {
		cleanup()
		log.Printf("同步统计临时文件失败: %v", err)
		return
	}
	if err := temp.Close(); err != nil {
		os.Remove(tempPath)
		log.Printf("关闭统计临时文件失败: %v", err)
		return
	}
	if err := os.Rename(tempPath, tokenStatsPath); err != nil {
		// Windows 可能无法用 Rename 替换已有目标文件。
		// 写入器已串行化，因此可安全使用此平台降级方案。
		if removeErr := os.Remove(tokenStatsPath); removeErr == nil || os.IsNotExist(removeErr) {
			if retryErr := os.Rename(tempPath, tokenStatsPath); retryErr == nil {
				return
			}
		}
		os.Remove(tempPath)
		log.Printf("替换统计文件失败: %v", err)
	}
}

func ScheduleTokenStatsSave() {
	if usingSQLite() {
		return
	}
	statsSaveOnce.Do(func() {
		go func() {
			for range statsSaveCh {
				// 合并突发写入，同时将写入器数量严格限制为一个。
				timer := time.NewTimer(200 * time.Millisecond)
				for {
					select {
					case <-statsSaveCh:
						if !timer.Stop() {
							select {
							case <-timer.C:
							default:
							}
						}
						timer.Reset(200 * time.Millisecond)
					case <-timer.C:
						SaveTokenStats()
						goto nextSave
					}
				}
			nextSave:
			}
		}()
	})
	select {
	case statsSaveCh <- struct{}{}:
	default:
	}
}

func RecordTokenUsage(model string, promptTokens, completionTokens, totalTokens int64) {
	RecordUsage(model, "", "", promptTokens, completionTokens, totalTokens)
}

// RecordUsage persists one completed gateway call together with its route.
func RecordUsage(model, upstreamName, upstreamModel string, promptTokens, completionTokens, totalTokens int64) {
	RecordUsageWithTiming(model, upstreamName, upstreamModel, promptTokens, completionTokens, totalTokens, time.Now(), 0, 0)
}

// RecordUsageWithTiming persists a completed call and its user-visible latency.
func RecordUsageWithTiming(model, upstreamName, upstreamModel string, promptTokens, completionTokens, totalTokens int64, calledAt time.Time, firstByteMS, durationMS int64) {
	recordUsageWithAPIKey(model, upstreamName, upstreamModel, promptTokens, completionTokens, totalTokens, calledAt, firstByteMS, durationMS, "", "")
}

func recordUsageWithAPIKey(model, upstreamName, upstreamModel string, promptTokens, completionTokens, totalTokens int64, calledAt time.Time, firstByteMS, durationMS int64, apiKeyID, apiKeyName string) {
	if recordSQLiteUsage(model, upstreamName, upstreamModel, apiKeyID, apiKeyName, promptTokens, completionTokens, totalTokens, calledAt, firstByteMS, durationMS) {
		return
	}
	CheckAndResetDailyStats()
	tokenStatsMu.Lock()
	tokenStats.TotalRequests++
	ms, ok := tokenStats.Models[model]
	if !ok {
		ms = &ModelStats{}
		tokenStats.Models[model] = ms
	}
	ms.RequestCount++
	ms.PromptTokens += promptTokens
	ms.CompletionTokens += completionTokens
	ms.TotalTokens += totalTokens
	if tokenStats.Daily == nil {
		tokenStats.Daily = &DailyStats{Date: GetToday(), Models: map[string]*ModelStats{}}
	}
	tokenStats.Daily.TotalRequests++
	dms, ok := tokenStats.Daily.Models[model]
	if !ok {
		dms = &ModelStats{}
		tokenStats.Daily.Models[model] = dms
	}
	dms.RequestCount++
	dms.PromptTokens += promptTokens
	dms.CompletionTokens += completionTokens
	dms.TotalTokens += totalTokens
	tokenStatsMu.Unlock()
	ScheduleTokenStatsSave()
}

// RequestUsageAccumulator 确保每个成功的上游请求只增加一次请求计数，
// 即使兼容供应商未返回 usage。流式供应商可能多次输出部分或累计用量，
// 因此最终提交每个计数器观测到的最大值。
type RequestUsageAccumulator struct {
	model            string
	upstreamName     string
	upstreamModel    string
	promptTokens     int64
	completionTokens int64
	totalTokens      int64
	timing           *RequestTiming
}

func NewRequestUsageAccumulator(model string, route ...string) *RequestUsageAccumulator {
	model, upstreamName, upstreamModel, timing := decodeUsageIdentity(model)
	if len(route) > 0 {
		upstreamName = route[0]
	}
	if len(route) > 1 {
		upstreamModel = route[1]
	}
	return &RequestUsageAccumulator{model: model, upstreamName: upstreamName, upstreamModel: upstreamModel, timing: timing}
}

func NewRequestUsageAccumulatorForContext(ctx context.Context, model string, route ...string) *RequestUsageAccumulator {
	accumulator := NewRequestUsageAccumulator(model, route...)
	if timing := requestTimingFromContext(ctx); timing != nil {
		accumulator.timing = timing
	}
	return accumulator
}

func (a *RequestUsageAccumulator) ObserveMap(usage map[string]any) {
	if a == nil || usage == nil {
		return
	}
	promptTokens, _ := getFloat(usage, "prompt_tokens", "input_tokens")
	completionTokens, _ := getFloat(usage, "completion_tokens", "output_tokens")
	totalTokens, _ := getFloat(usage, "total_tokens")
	if promptTokens > float64(a.promptTokens) {
		a.promptTokens = int64(promptTokens)
	}
	if completionTokens > float64(a.completionTokens) {
		a.completionTokens = int64(completionTokens)
	}
	if totalTokens > float64(a.totalTokens) {
		a.totalTokens = int64(totalTokens)
	}
}

func (a *RequestUsageAccumulator) Commit() {
	if a == nil {
		return
	}
	if combined := a.promptTokens + a.completionTokens; combined > a.totalTokens {
		a.totalTokens = combined
	}
	sample := usageSample{
		model:            a.model,
		upstreamName:     a.upstreamName,
		upstreamModel:    a.upstreamModel,
		promptTokens:     a.promptTokens,
		completionTokens: a.completionTokens,
		totalTokens:      a.totalTokens,
	}
	if a.timing != nil {
		a.timing.addUsage(sample)
		return
	}
	recordUsageSample(sample, time.Now(), 0, 0)
}
