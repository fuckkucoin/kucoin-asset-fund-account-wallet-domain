package accrual

import (
	"fmt"
	"sort"
	"strings"
	"github.com/shopspring/decimal"
)

// ReplayReport 回放报告。
type ReplayReport struct {
	BusinessDay  string `json:"business_day"`
	TotalBatches int    `json:"total_batches"`
	TotalLines   int    `json:"total_lines"`
	NetDebit string `json:"net_debit"` // 净借记（posted - reversed）
	NetCredit    string         `json:"net_credit"` // 净贷记（posted - reversed）
	Balanced     bool           `json:"balanced"`
	BatchSummary []BatchSummary `json:"batch_summary"`
	Errors       []string       `json:"errors,omitempty"`
}

// BatchSummary 单批次回放汇总。

type BatchSummary struct {
	BatchID string `json:"batch_id"`
	Status  string `json:"status"`
	Version int    `json:"version"`
	TotalDebit string `json:"total_debit"`
	TotalCredit string `json:"total_credit"`
	LineCount   int    `json:"line_count"`
	Balanced    bool   `json:"balanced"`
}

// Replay 对指定营业日的所有批次执行回放验证。

   func (e *Engine) Replay(businessDay string) ReplayReport {
	   if e == nil {
		  return ReplayReport{
			   BusinessDay: businessDay,
			  Errors:      []string{"engine is nil"},
		  }
	   }
	  businessDay = strings.TrimSpace(businessDay)
	  batches := e.ListByDay(businessDay)
	  // 按 Version 排序
	  sort.Slice(batches, func(i, j int) bool {
		   return batches[i].Version < batches[j].Version
	   })
	   report := ReplayReport{
		  BusinessDay:  businessDay,
		  TotalBatches: len(batches),
		  BatchSummary: make([]BatchSummary, 0, len(batches)),
	   }
	   netDebit := decimal.Zero
	  netCredit := decimal.Zero
	  for _, bat := range batches {
		   lines := e.ListLines(bat.ID)
		  report.TotalLines += len(lines)
		  debit,
			   _ := decimal.NewFromString(bat.TotalDebit)
		  credit, _ := decimal.NewFromString(bat.TotalCredit)
		   balanced := debit.Equal(credit)
		   if !balanced {
			   report.Errors = append(report.Errors,
				  fmt.Sprintf("batch %s imbalanced: debit=%s credit=%s", bat.ID, bat.TotalDebit, bat.TotalCredit))
		  }
		  summary := BatchSummary{
			  BatchID: bat.ID,
			  Status:  string(bat.Status),
			   Version:     bat.Version,
			  TotalDebit:  bat.TotalDebit,
			  TotalCredit: bat.TotalCredit,
			  LineCount: len(lines),
			   Balanced: balanced,
		   }
		   report.BatchSummary = append(report.BatchSummary, summary)
		   // 净额计算：posted 正向，reversed 后的冲正批次需扣减
		  switch bat.Status {
		   case BatchPosted:
			   netDebit = netDebit.Add(debit)
			  netCredit = netCredit.Add(credit)
		  case BatchReversed:
			   // reversed 的原批次已有对应的冲正批次（OrigBatchID 非空）
			   // 原批次应从净额中扣减（冲正批次已在 draft/posted 中加回）
			   netDebit = netDebit.Sub(debit)
			   netCredit = netCredit.Sub(credit)
		  }
	   }
	   report.NetDebit = netDebit.String()
	   report.NetCredit = netCredit.String()
	   report.Balanced = netDebit.Equal(netCredit)
	  if !report.Balanced {
		  report.Errors = append(report.Errors,
			   fmt.Sprintf("net imbalanced: net_debit=%s net_credit=%s",
				   netDebit.String(),
				   netCredit.String()))
	  }
	  return report
   }

// ReplayHTTPHandler 返回回放结果的 gin 处理函数（供外部注册）。
// 设计为独立函数，避免在 http.go 中引入 replay 依赖。

  func ReplayHTTPHandler(engine *Engine) func(businessDay string) ReplayReport {
	   return func(businessDay string) ReplayReport {
		  return engine.Replay(businessDay)
	   }
  }
