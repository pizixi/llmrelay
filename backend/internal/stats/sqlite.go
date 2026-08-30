package stats

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"llmrelay/backend/internal/storage"
)

// BreakdownStats is used by the admin panel for model/upstream/day views.
type BreakdownStats struct {
	Model            string `json:"model,omitempty"`
	Upstream         string `json:"upstream,omitempty"`
	Date             string `json:"date,omitempty"`
	RequestCount     int64  `json:"request_count"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CachedTokens     int64  `json:"cached_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
}

type UsageRecord struct {
	ID               int64  `json:"id"`
	RequestModel     string `json:"request_model"`
	UpstreamName     string `json:"upstream_name"`
	UpstreamModel    string `json:"upstream_model"`
	APIKeyID         string `json:"api_key_id,omitempty"`
	APIKeyName       string `json:"api_key_name,omitempty"`
	CalledAt         string `json:"called_at"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CachedTokens     int64  `json:"cached_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
	RequestCount     int64  `json:"request_count"`
	FirstByteMS      int64  `json:"first_byte_ms"`
	DurationMS       int64  `json:"duration_ms"`
}

type UsageSummary struct {
	RequestCount     int64 `json:"request_count"`
	PromptTokens     int64 `json:"prompt_tokens"`
	CachedTokens     int64 `json:"cached_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
}

type UsageQuery struct {
	Limit      int
	Offset     int
	Model      string
	Upstream   string
	APIKeyName string
	Date       string
}

type UsagePage struct {
	Items    []UsageRecord `json:"items"`
	Total    int64         `json:"total"`
	Limit    int           `json:"limit"`
	Offset   int           `json:"offset"`
	Summary  UsageSummary  `json:"summary"`
	KeyNames []string      `json:"key_names"`
}

var sqliteState struct {
	sync.Mutex
	db *sql.DB
}

const usageIdentityPrefix = "\x00llmrelay-usage:"

// UsageIdentity encodes route metadata into the string accepted by all
// existing stream adapters. The prefix is never sent to an upstream or client.
func UsageIdentity(model, upstreamName, upstreamModel string) string {
	payload, _ := json.Marshal([]string{model, upstreamName, upstreamModel})
	return usageIdentityPrefix + string(payload)
}

func UsageIdentityForContext(ctx context.Context, model, upstreamName, upstreamModel string) string {
	values := []string{model, upstreamName, upstreamModel}
	if timing := requestTimingFromContext(ctx); timing != nil {
		values = append(values, strconv.FormatUint(timing.id, 10))
	}
	payload, _ := json.Marshal(values)
	return usageIdentityPrefix + string(payload)
}

func DecodeUsageIdentity(value string) (model, upstreamName, upstreamModel string) {
	model, upstreamName, upstreamModel, _ = decodeUsageIdentity(value)
	return model, upstreamName, upstreamModel
}

func decodeUsageIdentity(value string) (model, upstreamName, upstreamModel string, timing *RequestTiming) {
	if !strings.HasPrefix(value, usageIdentityPrefix) {
		return value, "", value, nil
	}
	var values []string
	if json.Unmarshal([]byte(strings.TrimPrefix(value, usageIdentityPrefix)), &values) != nil {
		return value, "", value, nil
	}
	if len(values) > 0 {
		model = values[0]
	}
	if len(values) > 1 {
		upstreamName = values[1]
	}
	if len(values) > 2 {
		upstreamModel = values[2]
	}
	if len(values) > 3 {
		if timingID, err := strconv.ParseUint(values[3], 10, 64); err == nil {
			timing = requestTimingByID(timingID)
		}
	}
	return model, upstreamName, upstreamModel, timing
}

func usingSQLite() bool {
	return storage.IsSQLitePath(tokenStatsPath)
}

func sqliteDB() (*sql.DB, error) {
	sqliteState.Lock()
	defer sqliteState.Unlock()
	if sqliteState.db != nil {
		return sqliteState.db, nil
	}
	db, err := storage.Open(tokenStatsPath)
	if err != nil {
		return nil, err
	}
	sqliteState.db = db
	return db, nil
}

func closeSQLiteStats() {
	sqliteState.Lock()
	if sqliteState.db != nil {
		_ = sqliteState.db.Close()
		sqliteState.db = nil
	}
	sqliteState.Unlock()
}

func loadSQLiteStats(path string) error {
	closeSQLiteStats()
	db, err := storage.Open(path)
	if err != nil {
		return err
	}
	if err := importLegacyStats(db, path); err != nil {
		db.Close()
		return err
	}
	sqliteState.Lock()
	sqliteState.db = db
	sqliteState.Unlock()
	return nil
}

func recordSQLiteUsage(model, upstreamName, upstreamModel, apiKeyID, apiKeyName string, promptTokens, cachedTokens, completionTokens, totalTokens int64, calledAt time.Time, firstByteMS, durationMS int64) bool {
	if !usingSQLite() {
		return false
	}
	db, err := sqliteDB()
	if err != nil {
		log.Printf("打开 SQLite 统计失败: %v", err)
		return true
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = "未命名模型"
	}
	upstreamName = strings.TrimSpace(upstreamName)
	if upstreamName == "" {
		upstreamName = "未记录上游"
	}
	upstreamModel = strings.TrimSpace(upstreamModel)
	if upstreamModel == "" {
		upstreamModel = model
	}
	apiKeyID = strings.TrimSpace(apiKeyID)
	apiKeyName = strings.TrimSpace(apiKeyName)
	promptTokens = max(promptTokens, 0)
	cachedTokens = max(cachedTokens, 0)
	completionTokens = max(completionTokens, 0)
	if totalTokens < promptTokens+completionTokens {
		totalTokens = promptTokens + completionTokens
	}
	if calledAt.IsZero() {
		calledAt = time.Now()
	}
	firstByteMS = max(firstByteMS, 0)
	durationMS = max(durationMS, firstByteMS)
	_, err = db.Exec(`INSERT INTO usage_records
		(request_model, upstream_name, upstream_model, api_key_id, api_key_name, called_at, called_date, prompt_tokens, cached_tokens, completion_tokens, total_tokens, first_byte_ms, duration_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, model, upstreamName, upstreamModel, apiKeyID, apiKeyName, calledAt.UnixMilli(), calledAt.Format("2006-01-02"), promptTokens, cachedTokens, completionTokens, totalTokens, firstByteMS, durationMS)
	if err != nil {
		log.Printf("写入 SQLite 用量失败: %v", err)
	}
	return true
}

func resetSQLiteStats() bool {
	if !usingSQLite() {
		return false
	}
	db, err := sqliteDB()
	if err != nil {
		log.Printf("打开 SQLite 统计失败: %v", err)
		return true
	}
	if _, err := db.Exec("DELETE FROM usage_records"); err != nil {
		log.Printf("清空 SQLite 统计失败: %v", err)
	}
	return true
}

func aggregateQuery(db *sql.DB, where string, args ...any) map[string]*ModelStats {
	query := `SELECT request_model, COALESCE(SUM(request_count), 0), COALESCE(SUM(prompt_tokens), 0),
		COALESCE(SUM(cached_tokens), 0), COALESCE(SUM(completion_tokens), 0), COALESCE(SUM(total_tokens), 0)
        FROM usage_records`
	if where != "" {
		query += " WHERE " + where
	}
	query += " GROUP BY request_model"
	rows, err := db.Query(query, args...)
	if err != nil {
		log.Printf("查询统计失败: %v", err)
		return map[string]*ModelStats{}
	}
	defer rows.Close()
	result := map[string]*ModelStats{}
	for rows.Next() {
		var model string
		var value ModelStats
		if err := rows.Scan(&model, &value.RequestCount, &value.PromptTokens, &value.CachedTokens, &value.CompletionTokens, &value.TotalTokens); err == nil {
			result[model] = &value
		}
	}
	return result
}

func upstreamAggregateQuery(db *sql.DB, where string, args ...any) map[string]*ModelStats {
	query := `SELECT upstream_name, COALESCE(SUM(request_count), 0), COALESCE(SUM(prompt_tokens), 0),
		COALESCE(SUM(cached_tokens), 0), COALESCE(SUM(completion_tokens), 0), COALESCE(SUM(total_tokens), 0)
        FROM usage_records`
	if where != "" {
		query += " WHERE " + where
	}
	query += " GROUP BY upstream_name"
	rows, err := db.Query(query, args...)
	if err != nil {
		log.Printf("查询上游统计失败: %v", err)
		return map[string]*ModelStats{}
	}
	defer rows.Close()
	result := map[string]*ModelStats{}
	for rows.Next() {
		var name string
		var value ModelStats
		if err := rows.Scan(&name, &value.RequestCount, &value.PromptTokens, &value.CachedTokens, &value.CompletionTokens, &value.TotalTokens); err == nil {
			result[name] = &value
		}
	}
	return result
}

func breakdownQuery(db *sql.DB, query string) []BreakdownStats {
	rows, err := db.Query(query)
	if err != nil {
		log.Printf("查询统计维度失败: %v", err)
		return []BreakdownStats{}
	}
	defer rows.Close()
	result := []BreakdownStats{}
	for rows.Next() {
		var value BreakdownStats
		if err := rows.Scan(&value.Model, &value.Upstream, &value.Date, &value.RequestCount, &value.PromptTokens, &value.CachedTokens, &value.CompletionTokens, &value.TotalTokens); err == nil {
			result = append(result, value)
		}
	}
	return result
}

func sqliteSnapshot() TokenStatsData {
	db, err := sqliteDB()
	if err != nil {
		log.Printf("打开 SQLite 统计失败: %v", err)
		return TokenStatsData{Models: map[string]*ModelStats{}, Upstreams: map[string]*ModelStats{}}
	}
	today := time.Now().Format("2006-01-02")
	models := aggregateQuery(db, "called_date != ''")
	dailyModels := aggregateQuery(db, "called_date = ?", today)
	upstreams := upstreamAggregateQuery(db, "called_date != ''")
	daily := &DailyStats{Date: today, Models: dailyModels}
	var totalRequests int64
	for _, item := range models {
		totalRequests += item.RequestCount
	}
	daily.TotalRequests = 0
	for _, item := range dailyModels {
		daily.TotalRequests += item.RequestCount
	}
	return TokenStatsData{
		TotalRequests:  totalRequests,
		Models:         models,
		Daily:          daily,
		Upstreams:      upstreams,
		DailyUpstreams: upstreamAggregateQuery(db, "called_date = ?", today),
		ModelUpstreams: breakdownQuery(db, `SELECT request_model, upstream_name, '', COALESCE(SUM(request_count), 0),
			COALESCE(SUM(prompt_tokens), 0), COALESCE(SUM(cached_tokens), 0), COALESCE(SUM(completion_tokens), 0), COALESCE(SUM(total_tokens), 0)
			FROM usage_records GROUP BY request_model, upstream_name ORDER BY COALESCE(SUM(total_tokens), 0) DESC`),
		DailyModelUpstreams: breakdownQuery(db, `SELECT request_model, upstream_name, '', COALESCE(SUM(request_count), 0),
			COALESCE(SUM(prompt_tokens), 0), COALESCE(SUM(cached_tokens), 0), COALESCE(SUM(completion_tokens), 0), COALESCE(SUM(total_tokens), 0)
			FROM usage_records WHERE called_date = '`+today+`' GROUP BY request_model, upstream_name ORDER BY COALESCE(SUM(total_tokens), 0) DESC`),
		Days: breakdownQuery(db, `SELECT '', '', called_date, COALESCE(SUM(request_count), 0),
			COALESCE(SUM(prompt_tokens), 0), COALESCE(SUM(cached_tokens), 0), COALESCE(SUM(completion_tokens), 0), COALESCE(SUM(total_tokens), 0)
			FROM usage_records GROUP BY called_date ORDER BY called_date DESC`),
	}
}

// usageAPIKeySource uses the managed API key table when it is available. The
// name stored on usage_records remains a snapshot for records created before
// a managed key existed, or after that key was deleted; otherwise a rename of
// an API key is reflected immediately in the usage page and its filters.
func usageAPIKeySource(db *sql.DB) (from, nameExpression string) {
	var exists int
	if err := db.QueryRow(`SELECT EXISTS(
		SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'api_keys'
	)`).Scan(&exists); err != nil || exists == 0 {
		return "usage_records", "usage_records.api_key_name"
	}
	return "usage_records LEFT JOIN api_keys ON api_keys.id = usage_records.api_key_id",
		"COALESCE(NULLIF(TRIM(api_keys.name), ''), usage_records.api_key_name)"
}

func ListUsageRecords(query UsageQuery) (UsagePage, error) {
	page := UsagePage{Items: []UsageRecord{}, KeyNames: []string{}}
	if query.Limit <= 0 || query.Limit > 500 {
		query.Limit = 50
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	page.Limit, page.Offset = query.Limit, query.Offset
	db, err := sqliteDB()
	if err != nil {
		return page, err
	}
	from, apiKeyNameExpression := usageAPIKeySource(db)
	conditions := []string{"1 = 1"}
	args := []any{}
	if value := strings.TrimSpace(query.Model); value != "" {
		conditions = append(conditions, "usage_records.request_model = ?")
		args = append(args, value)
	}
	if value := strings.TrimSpace(query.Upstream); value != "" {
		conditions = append(conditions, "usage_records.upstream_name = ?")
		args = append(args, value)
	}
	if value := strings.TrimSpace(query.APIKeyName); value != "" {
		conditions = append(conditions, apiKeyNameExpression+" = ?")
		args = append(args, value)
	}
	if value := strings.TrimSpace(query.Date); value != "" {
		conditions = append(conditions, "usage_records.called_date = ?")
		args = append(args, value)
	}
	where := strings.Join(conditions, " AND ")
	if err := db.QueryRow("SELECT COUNT(*) FROM "+from+" WHERE "+where, args...).Scan(&page.Total); err != nil {
		return page, err
	}
	if err := db.QueryRow(`SELECT COALESCE(SUM(request_count), 0), COALESCE(SUM(prompt_tokens), 0),
		COALESCE(SUM(cached_tokens), 0), COALESCE(SUM(completion_tokens), 0), COALESCE(SUM(total_tokens), 0)
		FROM `+from+` WHERE `+where, args...).Scan(&page.Summary.RequestCount, &page.Summary.PromptTokens,
		&page.Summary.CachedTokens, &page.Summary.CompletionTokens, &page.Summary.TotalTokens); err != nil {
		return page, err
	}
	keyRows, err := db.Query("SELECT DISTINCT " + apiKeyNameExpression + " FROM " + from +
		" WHERE TRIM(" + apiKeyNameExpression + ") != '' ORDER BY " + apiKeyNameExpression)
	if err != nil {
		return page, err
	}
	for keyRows.Next() {
		var name string
		if err := keyRows.Scan(&name); err != nil {
			_ = keyRows.Close()
			return page, err
		}
		page.KeyNames = append(page.KeyNames, name)
	}
	if err := keyRows.Err(); err != nil {
		_ = keyRows.Close()
		return page, err
	}
	if err := keyRows.Close(); err != nil {
		return page, err
	}
	args = append(args, query.Limit, query.Offset)
	rows, err := db.Query(`SELECT usage_records.id, usage_records.request_model, usage_records.upstream_name, usage_records.upstream_model, usage_records.called_at,
		usage_records.api_key_id, `+apiKeyNameExpression+`, usage_records.prompt_tokens, usage_records.cached_tokens, usage_records.completion_tokens, usage_records.total_tokens, usage_records.request_count, usage_records.first_byte_ms, usage_records.duration_ms
		FROM `+from+` WHERE `+where+` ORDER BY usage_records.called_at DESC, usage_records.id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var value UsageRecord
		var calledAt int64
		if err := rows.Scan(&value.ID, &value.RequestModel, &value.UpstreamName, &value.UpstreamModel, &calledAt,
			&value.APIKeyID, &value.APIKeyName,
			&value.PromptTokens, &value.CachedTokens, &value.CompletionTokens, &value.TotalTokens, &value.RequestCount, &value.FirstByteMS, &value.DurationMS); err != nil {
			return page, err
		}
		value.CalledAt = time.UnixMilli(calledAt).Format(time.RFC3339)
		page.Items = append(page.Items, value)
	}
	return page, rows.Err()
}

// DeleteUsageRecord removes one persisted usage entry. Aggregate statistics
// are calculated from usage_records, so snapshots and summaries immediately
// reflect the deletion without a separate reconciliation step.
func DeleteUsageRecord(id int64) (bool, error) {
	if id <= 0 {
		return false, nil
	}
	db, err := sqliteDB()
	if err != nil {
		return false, err
	}
	result, err := db.Exec("DELETE FROM usage_records WHERE id = ?", id)
	if err != nil {
		return false, err
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return deleted > 0, nil
}

func importLegacyStats(db *sql.DB, databasePath string) error {
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM usage_records").Scan(&count); err != nil || count > 0 {
		return err
	}
	legacyPath := filepath.Join(filepath.Dir(databasePath), "stats.json")
	data, err := os.ReadFile(legacyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var legacy TokenStatsData
	if err := json.Unmarshal(data, &legacy); err != nil {
		return fmt.Errorf("parse legacy stats: %w", err)
	}
	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1)
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	insert := func(model string, value *ModelStats, when time.Time) error {
		if value == nil || value.RequestCount == 0 && value.TotalTokens == 0 {
			return nil
		}
		_, err := tx.Exec(`INSERT INTO usage_records
			(request_model, upstream_name, upstream_model, called_at, called_date, prompt_tokens, cached_tokens, completion_tokens, total_tokens, request_count, source)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'legacy_import')`, model, "历史数据", model, when.UnixMilli(), when.Format("2006-01-02"), value.PromptTokens, value.CachedTokens, value.CompletionTokens, value.TotalTokens, value.RequestCount)
		return err
	}
	for model, total := range legacy.Models {
		daily := (*ModelStats)(nil)
		if legacy.Daily != nil && legacy.Daily.Date == today {
			daily = legacy.Daily.Models[model]
		}
		if daily != nil {
			historical := &ModelStats{
				RequestCount:     total.RequestCount - daily.RequestCount,
				PromptTokens:     total.PromptTokens - daily.PromptTokens,
				CachedTokens:     total.CachedTokens - daily.CachedTokens,
				CompletionTokens: total.CompletionTokens - daily.CompletionTokens,
				TotalTokens:      total.TotalTokens - daily.TotalTokens,
			}
			if err := insert(model, historical, yesterday); err != nil {
				_ = tx.Rollback()
				return err
			}
			if err := insert(model, daily, time.Now()); err != nil {
				_ = tx.Rollback()
				return err
			}
		} else if err := insert(model, total, yesterday); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}
