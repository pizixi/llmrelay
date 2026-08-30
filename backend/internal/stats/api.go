package stats

import (
	"encoding/json"
	"strconv"
	"strings"
)

func getFloat(values map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			switch number := value.(type) {
			case float64:
				return number, true
			case float32:
				return float64(number), true
			case int:
				return float64(number), true
			case int64:
				return float64(number), true
			case int32:
				return float64(number), true
			case json.Number:
				if parsed, err := number.Float64(); err == nil {
					return parsed, true
				}
			case string:
				if parsed, err := strconv.ParseFloat(strings.TrimSpace(number), 64); err == nil {
					return parsed, true
				}
			}
		}
	}
	return 0, false
}

// cachedTokensFromUsage recognizes the cache-hit counters used by OpenAI
// Chat, OpenAI Responses, Anthropic, and common compatible providers. Cache
// hits remain a separate metric because OpenAI includes them in input tokens,
// while Anthropic reports them alongside ordinary input tokens.
func cachedTokensFromUsage(usage map[string]any) float64 {
	if usage == nil {
		return 0
	}
	best, _ := getFloat(usage,
		"cache_read_input_tokens",
		"cached_tokens",
		"cached_content_token_count",
	)
	for _, detailsKey := range []string{"prompt_tokens_details", "input_tokens_details"} {
		details, _ := usage[detailsKey].(map[string]any)
		if value, ok := getFloat(details, "cached_tokens", "cache_read_input_tokens"); ok && value > best {
			best = value
		}
	}
	if best < 0 {
		return 0
	}
	return best
}

// Snapshot 返回统计数据的深拷贝。
func Snapshot() TokenStatsData {
	if usingSQLite() {
		return sqliteSnapshot()
	}
	tokenStatsMu.Lock()
	defer tokenStatsMu.Unlock()
	return cloneData(tokenStats)
}

// Reset 清空累计和当日统计。
func Reset() {
	if resetSQLiteStats() {
		return
	}
	today := GetToday()
	tokenStatsMu.Lock()
	tokenStats = &TokenStatsData{Models: map[string]*ModelStats{}, Daily: &DailyStats{Date: today, Models: map[string]*ModelStats{}}}
	statsDate = today
	tokenStatsMu.Unlock()
	SaveTokenStats()
}

// SetPath 配置统计持久化文件，主要由应用启动和测试环境调用。
func SetPath(path string) {
	closeSQLiteStats()
	tokenStatsPath = path
}

func Path() string { return tokenStatsPath }

// Restore 替换内存中的统计状态，供配置迁移和测试隔离使用。
func Restore(data TokenStatsData, date string) {
	copy := cloneData(&data)
	tokenStatsMu.Lock()
	tokenStats = &copy
	statsDate = date
	tokenStatsMu.Unlock()
}

func cloneData(source *TokenStatsData) TokenStatsData {
	result := TokenStatsData{Models: map[string]*ModelStats{}}
	if source == nil {
		return result
	}
	result.TotalRequests = source.TotalRequests
	for model, item := range source.Models {
		if item != nil {
			copy := *item
			result.Models[model] = &copy
		}
	}
	if source.Daily != nil {
		result.Daily = &DailyStats{Date: source.Daily.Date, TotalRequests: source.Daily.TotalRequests, Models: map[string]*ModelStats{}}
		for model, item := range source.Daily.Models {
			if item != nil {
				copy := *item
				result.Daily.Models[model] = &copy
			}
		}
	}
	return result
}
