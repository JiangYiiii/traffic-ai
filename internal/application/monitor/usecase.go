// Package monitor 监控应用层：编排 MySQL 聚合查询与 Redis 实时计数，组装监控响应。
package monitor

import (
	"context"
	"time"

	domainModel "github.com/trailyai/traffic-ai/internal/domain/model"
	domainMonitor "github.com/trailyai/traffic-ai/internal/domain/monitor"
	"github.com/trailyai/traffic-ai/internal/infrastructure/persistence/mysql"
	redisinfra "github.com/trailyai/traffic-ai/internal/infrastructure/persistence/redis"
)

// UseCase 监控数据聚合编排。
type UseCase struct {
	monitorRepo    *mysql.MonitorRepo
	modelRepo      domainModel.ModelRepository
	accountRepo    domainModel.ModelAccountRepository
	monitorCounter *redisinfra.MonitorCounter
}

func NewUseCase(
	monitorRepo *mysql.MonitorRepo,
	modelRepo domainModel.ModelRepository,
	accountRepo domainModel.ModelAccountRepository,
	monitorCounter *redisinfra.MonitorCounter,
) *UseCase {
	return &UseCase{
		monitorRepo:    monitorRepo,
		modelRepo:      modelRepo,
		accountRepo:    accountRepo,
		monitorCounter: monitorCounter,
	}
}

// GetOverview 返回所有模型的聚合指标总览。
func (uc *UseCase) GetOverview(ctx context.Context, hours int) (*domainMonitor.OverviewResult, error) {
	if hours <= 0 {
		hours = 1
	}

	stats, err := uc.monitorRepo.ListModelStats(ctx, hours)
	if err != nil {
		return nil, err
	}

	modelNames := make([]string, 0, len(stats))
	for _, s := range stats {
		modelNames = append(modelNames, s.Model)
	}
	realtimeMap, _ := uc.monitorCounter.GetModelStats(ctx, modelNames)

	allModels, _ := uc.modelRepo.List(ctx, domainModel.ListFilter{})
	modelNameToID := make(map[string]int64, len(allModels))
	for _, m := range allModels {
		modelNameToID[m.ModelName] = m.ID
	}

	overviews := make([]*domainMonitor.ModelOverview, 0, len(stats))
	for _, s := range stats {
		errRate := float64(0)
		if s.TotalRequests > 0 {
			errRate = float64(s.ErrorCount) / float64(s.TotalRequests)
		}

		p95, _ := uc.monitorRepo.GetModelP95Latency(ctx, s.Model, hours)

		ov := &domainMonitor.ModelOverview{
			ModelID:       modelNameToID[s.Model],
			ModelName:     s.Model,
			TotalRequests: s.TotalRequests,
			ErrorCount:    s.ErrorCount,
			ErrorRate:     errRate,
			AvgLatencyMs:  s.AvgLatencyMs,
			P95LatencyMs:  p95,
			TotalTokens:   s.TotalTokens,
			TotalCostUSD:  float64(s.TotalCostMicro) / 1_000_000,
		}

		if rt, ok := realtimeMap[s.Model]; ok {
			ov.TodayRequests = rt.TotalRequests
			ov.TodayTokens = rt.TotalTokens
		}

		// active_accounts：该模型下启用的账号数
		if mid := modelNameToID[s.Model]; mid > 0 {
			if accts, err := uc.accountRepo.ListActiveByModelIDs(ctx, []int64{mid}); err == nil {
				ov.ActiveAccounts = len(accts)
			}
		}

		overviews = append(overviews, ov)
	}

	return &domainMonitor.OverviewResult{
		Models:      overviews,
		PeriodHours: hours,
		GeneratedAt: time.Now(),
	}, nil
}

// GetModelDetail 返回单模型详情 + 按模型账号拆分 + 时间趋势。
func (uc *UseCase) GetModelDetail(ctx context.Context, modelID int64, hours int, granularity string) (*domainMonitor.ModelDetailResult, error) {
	if hours <= 0 {
		hours = 1
	}
	if granularity == "" {
		granularity = "hour"
	}

	m, err := uc.modelRepo.FindByID(ctx, modelID)
	if err != nil || m == nil {
		return nil, err
	}

	stats, err := uc.monitorRepo.ListModelStats(ctx, hours)
	if err != nil {
		return nil, err
	}
	var modelStat *mysql.ModelStat
	for _, s := range stats {
		if s.Model == m.ModelName {
			modelStat = s
			break
		}
	}
	if modelStat == nil {
		modelStat = &mysql.ModelStat{Model: m.ModelName}
	}

	p95, _ := uc.monitorRepo.GetModelP95Latency(ctx, m.ModelName, hours)
	errRate := float64(0)
	if modelStat.TotalRequests > 0 {
		errRate = float64(modelStat.ErrorCount) / float64(modelStat.TotalRequests)
	}

	rtMap, _ := uc.monitorCounter.GetModelStats(ctx, []string{m.ModelName})
	ov := &domainMonitor.ModelOverview{
		ModelID:       m.ID,
		ModelName:     m.ModelName,
		TotalRequests: modelStat.TotalRequests,
		ErrorCount:    modelStat.ErrorCount,
		ErrorRate:     errRate,
		AvgLatencyMs:  modelStat.AvgLatencyMs,
		P95LatencyMs:  p95,
		TotalTokens:   modelStat.TotalTokens,
		TotalCostUSD:  float64(modelStat.TotalCostMicro) / 1_000_000,
	}
	if rt, ok := rtMap[m.ModelName]; ok {
		ov.TodayRequests = rt.TotalRequests
		ov.TodayTokens = rt.TotalTokens
	}

	// 模型账号维度：usage_logs 按 model_account_id 聚合
	accountStats, err := uc.monitorRepo.ListModelAccountStatsByModel(ctx, m.ModelName, hours)
	if err != nil {
		return nil, err
	}

	// 即使时间窗口内没有调用，也展示该模型下所有绑定账号（零值占位）。
	boundAccounts, _ := uc.accountRepo.ListByModelID(ctx, modelID)
	boundSet := make(map[int64]*domainModel.ModelAccount, len(boundAccounts))
	boundIDs := make([]int64, 0, len(boundAccounts))
	for _, a := range boundAccounts {
		if a == nil {
			continue
		}
		boundSet[a.ID] = a
		boundIDs = append(boundIDs, a.ID)
	}
	ov.ActiveAccounts = 0
	for _, a := range boundAccounts {
		if a != nil && a.IsActive {
			ov.ActiveAccounts++
		}
	}

	// 合并 accountStats 与绑定账号 ID，组装指标
	seen := make(map[int64]struct{}, len(accountStats)+len(boundIDs))
	allIDs := make([]int64, 0, len(accountStats)+len(boundIDs))
	for _, s := range accountStats {
		if s.ModelAccountID <= 0 {
			// 旧历史数据：model_account_id=0 的行直接忽略，不混进账号维度列表。
			continue
		}
		if _, ok := seen[s.ModelAccountID]; ok {
			continue
		}
		seen[s.ModelAccountID] = struct{}{}
		allIDs = append(allIDs, s.ModelAccountID)
	}
	for _, id := range boundIDs {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		allIDs = append(allIDs, id)
	}

	// 对于已从 boundSet 里拿到 info 的账号直接复用；其余仍需 ListByIDs。
	var extra []int64
	for _, id := range allIDs {
		if _, ok := boundSet[id]; !ok {
			extra = append(extra, id)
		}
	}
	if len(extra) > 0 {
		extras, _ := uc.accountRepo.ListByIDs(ctx, extra)
		for _, a := range extras {
			boundSet[a.ID] = a
		}
	}
	rtAccountMap, _ := uc.monitorCounter.GetAccountStats(ctx, allIDs)

	// 对 accountStats 按 id 建索引，便于 O(1) 查聚合
	statMap := make(map[int64]*mysql.ModelAccountStat, len(accountStats))
	for _, s := range accountStats {
		if s.ModelAccountID > 0 {
			statMap[s.ModelAccountID] = s
		}
	}

	accounts := make([]*domainMonitor.AccountMetrics, 0, len(allIDs))
	for _, id := range allIDs {
		stat := statMap[id]
		if stat == nil {
			stat = &mysql.ModelAccountStat{ModelAccountID: id}
		}
		am := modelAccountStatToMetrics(stat, boundSet[id], rtAccountMap[id])
		accounts = append(accounts, am)
	}

	tsRows, err := uc.monitorRepo.ListModelTimeSeries(ctx, m.ModelName, hours, granularity)
	if err != nil {
		return nil, err
	}
	ts := toTimeSeriesPoints(tsRows)

	return &domainMonitor.ModelDetailResult{
		Model:       ov,
		Accounts:    accounts,
		TimeSeries:  ts,
		PeriodHours: hours,
		Granularity: granularity,
	}, nil
}

// GetAccountDetail 返回单账号详情 + 时间趋势。
func (uc *UseCase) GetAccountDetail(ctx context.Context, modelAccountID int64, hours int, granularity string) (*domainMonitor.AccountDetailResult, error) {
	if hours <= 0 {
		hours = 24
	}
	if granularity == "" {
		granularity = "hour"
	}

	acct, err := uc.accountRepo.FindByID(ctx, modelAccountID)
	if err != nil || acct == nil {
		return nil, err
	}

	stat, err := uc.monitorRepo.GetModelAccountStat(ctx, modelAccountID, hours)
	if err != nil {
		return nil, err
	}

	rtMap, _ := uc.monitorCounter.GetAccountStats(ctx, []int64{modelAccountID})
	am := modelAccountStatToMetrics(stat, acct, rtMap[modelAccountID])

	tsRows, err := uc.monitorRepo.ListModelAccountTimeSeries(ctx, modelAccountID, hours, granularity)
	if err != nil {
		return nil, err
	}

	return &domainMonitor.AccountDetailResult{
		Account:     am,
		TimeSeries:  toTimeSeriesPoints(tsRows),
		PeriodHours: hours,
		Granularity: granularity,
	}, nil
}

// GetRealtime 返回全模型 + 全账号的 Redis 今日实时快照。
func (uc *UseCase) GetRealtime(ctx context.Context) (*domainMonitor.RealtimeResult, error) {
	modelNames, err := uc.monitorRepo.ListAllModelNames(ctx, 24)
	if err != nil {
		return nil, err
	}
	accountIDs, err := uc.monitorRepo.ListAllModelAccountIDs(ctx, 24)
	if err != nil {
		return nil, err
	}

	modelStats, _ := uc.monitorCounter.GetModelStats(ctx, modelNames)
	accountStats, _ := uc.monitorCounter.GetAccountStats(ctx, accountIDs)

	accountInfos, _ := uc.accountRepo.ListByIDs(ctx, accountIDs)
	accountInfoMap := make(map[int64]*domainModel.ModelAccount, len(accountInfos))
	for _, a := range accountInfos {
		accountInfoMap[a.ID] = a
	}

	modelResults := make([]*domainMonitor.ModelRealtimeStats, 0, len(modelNames))
	for _, name := range modelNames {
		s := &domainMonitor.ModelRealtimeStats{ModelName: name}
		if ms, ok := modelStats[name]; ok {
			s.TodayRequests = ms.TotalRequests
			s.TodayErrors = ms.ErrorCount
			s.TodayTokens = ms.TotalTokens
			s.AvgLatencyMs = ms.AvgLatencyMs
		}
		modelResults = append(modelResults, s)
	}

	accountResults := make([]*domainMonitor.AccountRealtimeStats, 0, len(accountIDs))
	for _, id := range accountIDs {
		s := &domainMonitor.AccountRealtimeStats{AccountID: id}
		if ai, ok := accountInfoMap[id]; ok {
			s.AccountName = ai.Name
		}
		if as, ok := accountStats[id]; ok {
			s.TodayRequests = as.TotalRequests
			s.TodayErrors = as.ErrorCount
			s.TodayTokens = as.TotalTokens
			s.AvgLatencyMs = as.AvgLatencyMs
		}
		accountResults = append(accountResults, s)
	}

	return &domainMonitor.RealtimeResult{
		Models:   modelResults,
		Accounts: accountResults,
		Date:     time.Now().Format("2006-01-02"),
	}, nil
}

// --- 辅助函数 ---

func modelAccountStatToMetrics(s *mysql.ModelAccountStat, acct *domainModel.ModelAccount, rt *redisinfra.AccountDayStats) *domainMonitor.AccountMetrics {
	am := &domainMonitor.AccountMetrics{
		AccountID:     s.ModelAccountID,
		TotalRequests: s.TotalRequests,
		ErrorCount:    s.ErrorCount,
		AvgLatencyMs:  s.AvgLatencyMs,
		TotalTokens:   s.TotalTokens,
		TotalCostUSD:  float64(s.TotalCostMicro) / 1_000_000,
	}
	if s.TotalRequests > 0 {
		am.ErrorRate = float64(s.ErrorCount) / float64(s.TotalRequests)
	}
	if acct != nil {
		am.AccountName = acct.Name
		am.Provider = acct.Provider
		am.Status = acct.Status()
	}
	if rt != nil {
		am.TodayRequests = rt.TotalRequests
		am.TodayTokens = rt.TotalTokens
		am.TodayAvgLatency = rt.AvgLatencyMs
	}
	return am
}

func toTimeSeriesPoints(rows []*mysql.TimeSeriesRow) []*domainMonitor.TimeSeriesPoint {
	out := make([]*domainMonitor.TimeSeriesPoint, 0, len(rows))
	for _, r := range rows {
		out = append(out, &domainMonitor.TimeSeriesPoint{
			Bucket:        r.Bucket,
			TotalRequests: r.TotalRequests,
			ErrorCount:    r.ErrorCount,
			AvgLatencyMs:  r.AvgLatencyMs,
			TotalTokens:   r.TotalTokens,
		})
	}
	return out
}
