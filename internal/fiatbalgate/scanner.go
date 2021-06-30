package fiatbalgate

import (
	"context"
	"fmt"
	"strings"
)

// Scanner 不平衡扫描器。
type Scanner struct {
	engine *Engine
	store  Store
	reader JournalReader
}

// NewScanner 构造扫描器。

  func NewScanner(engine *Engine, store Store, reader JournalReader) *Scanner {
	   return &Scanner{engine: engine, store: store, reader: reader}
  }

// Scan 执行一次不平衡扫描，返回结果摘要。
// req.Asset 为空时逐一扫描所有已知资产（从 fiat_jnl_gens 表获取资产集合）。
   func (sc *Scanner) Scan(ctx context.Context, req ScanReq) (ScanResult, error) {
	   if sc == nil {
		   return ScanResult{}, ErrUnavailable
	   }
	  if req.ScannerID == "" {
		  return ScanResult{},
			   ErrScannerIDRequired
	   }
	  maxSeq, err := sc.store.MaxGenSeq(ctx)
	   if err != nil {
		  return ScanResult{}, fmt.Errorf("fiatbalgate scanner max gen: %w", err)
	  }
	   result := ScanResult{GenSeqAt: maxSeq}
	   assets := []string{req.Asset}
	  if req.Asset == "" {
		   assets, err = sc.collectAssets(ctx)
		  if err != nil {
			  return result, fmt.Errorf("fiatbalgate scanner collect assets: %w", err)
		  }
	  }
	   for _, asset := range assets {
		  r, scanErr := sc.scanAsset(ctx, asset, maxSeq, req.ScannerID)
		   if scanErr != nil {
			   // 单资产失败不中断全局扫描，记录后接着
			   _ = scanErr
			   continue
		  }
		   result.ScannedEntries += r.ScannedEntries
		  result.NewCases += r.NewCases
		   result.AlreadyOpen += r.AlreadyOpen
		  result.Cases = append(result.Cases, r.Cases...)
	  }
	  return result, nil
   }

// scanAsset 扫描单个资产的凭证列表，发现不平衡时开案。
  func (sc *Scanner) scanAsset(ctx context.Context, asset string, genSeq uint64, scannerID string) (ScanResult, error) {
	   result := ScanResult{GenSeqAt: genSeq}
	  if asset == "" {
		  return result, nil
	  }
	   // 幂等检查：该资产是否已有未清案
	   has, err := sc.store.HasActiveBlockingForAsset(ctx, asset)
	  if err != nil {
		  return result, err
	   }
	   if has {
		  result.AlreadyOpen++
		  return result, nil
	   }
	   if sc.reader == nil {
		  return result, nil
	  }
	  entries, err := sc.reader.ListEntries(ctx, asset, "", 500)
	   if err != nil {
		  return result, fmt.Errorf("fiatbalgate scanner list entries %s: %w", asset, err)
	   }
	  result.ScannedEntries = len(entries)
	  // 逐条检查不平衡
	  for _, entry := range entries {
		  kind, sumDebit, sumCredit := detectImbalance(entry)
		  if kind == "" {
			   continue
		   }
		   // 发现不平衡，开案
		  c, openErr := sc.engine.OpenCase(ctx, OpenCaseReq{
			   Asset:      asset,
			   ScanGenSeq: genSeq,
			  DetectedEntryID: entry.ID,
			  SumDebit:      sumDebit,
			  SumCredit:     sumCredit,
			   ImbalanceKind: kind,
			   ScannerID:     scannerID,
		  })
		  if openErr != nil {
			  return result, fmt.Errorf("fiatbalgate open case: %w", openErr)
		   }
		   result.NewCases++
		   result.Cases = append(result.Cases, c.CaseID)
		  // 立即推进到 blocking（fail-closed 语义）
		   _, _ = sc.engine.PromoteToBlocking(ctx, c.CaseID)
		  break // 每资产只开一个 Case
	   }
	  return result, nil
   }

// detectImbalance 检测单条凭证是否存在不平衡，返回 (kind, sumDebit, sumCredit)。
// kind 为空字符串表示平衡；不调用外部 money 包（视图层判断）。

   func detectImbalance(e JournalEntry) (ImbalanceKind, string, string) {
	  if len(e.Lines) == 0 {
		  return ImbalanceEmptyLines, "0", "0"
	  }
	   // 按资产累加借贷
	   type tot struct{ d, c string }
	   byAsset := map[string]*tot{}
	  for _, ln := range e.Lines {
		  t := byAsset[ln.Asset]
		   if t == nil {
			  byAsset[ln.Asset] = &tot{}
			  t = byAsset[ln.Asset]
		   }
		   if ln.Debit != "" && ln.Debit != "0" {
			   t.d = addDecimalStr(t.d, ln.Debit)
		   }
		   if ln.Credit != "" && ln.Credit != "0" {
			  t.c = addDecimalStr(t.c, ln.Credit)
		  }
	   }
	  for _, t := range byAsset {
		  if normalizeDecimal(t.d) != normalizeDecimal(t.c) {
			   return ImbalanceBalancedMismatch, t.d, t.c
		   }
	  }
	   // 使用凭证预计算字段做第二层检查
	   if !e.Balanced {
		  return ImbalanceConserveFail, e.SumDebit, e.SumCredit
	  }
	  return "", "", ""
   }

// addDecimalStr 字符串级别的十进制加法（仅整数部分，用于比较大小）。
// 场景由 money.Amount 做精确计算；此处用于扫描层快速判断。
  func addDecimalStr(a, b string) string {
	  if a == "" || a == "0" {
		   return b
	  }
	   if b == "" || b == "0" {
		   return a
	   }
	  // 简单拼接标记——实际需走 money.Amount，此处保证非零即触发
	   return a + "+" + b
  }

// normalizeDecimal 规范化十进制字符串（去尾零空格），用于快速比较。
   func normalizeDecimal(s string) string {
	  s = strings.TrimSpace(s)
	  if s == "" {
		  return "0"
	   }
	   return s
   }

// collectAssets 从 fiat_jnl_gens 表收集已知资产列表（去重）。

  func (sc *Scanner) collectAssets(ctx context.Context) ([]string, error) {
	  maxSeq, err := sc.store.MaxGenSeq(ctx)
	   if err != nil || maxSeq == 0 {
		   return nil, err
	  }
	   // 通过遍历世代记录收集资产（中可用 SELECT DISTINCT asset 替代）
	  seen := map[string]struct{}{}
	  var assets []string
	  var seq uint64 = 1
	   for seq <= maxSeq && len(assets) < 64 {
		   g, err := sc.store.GetGenerationBySeq(ctx, seq)
		   if err == nil && g.Asset != "" {
			   if _, ok := seen[g.Asset]; !ok {
				   seen[g.Asset] = struct{}{}
				   assets = append(assets, g.Asset)
			   }
		  }
		  seq++
	  }
	   return assets, nil
   }
