package ledorphan

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

const kindOrphanReport = "lo_orphan_report"

// ── OrphanReport 持久化 ───────────────────────────────────────────────────────
// SaveReport 持久化孤儿报告（新建或覆盖）。
// 若 KV 中已有同 report_id 的记录，则基于当前围栏执行 CAS 替换。
  func SaveReport(ctx context.Context, kv persist.KV, report *OrphanReport) error {
	  if kv == nil {
		   return ErrStorageNil
	  }
	  body, err := json.Marshal(report)
	   if err != nil {
		   return fmt.Errorf("ledorphan save report: marshal: %w", err)
	   }
	   row := persist.Row{Kind: kindOrphanReport, Key: report.ReportID, Body: body}
	  cur, getErr := kv.Get(ctx, kindOrphanReport, report.ReportID)
	   if getErr == persist.ErrNotFound {
		   return persist.PutNew(ctx, kv, row)
	   }
	  if getErr != nil {
		  return getErr
	  }
	   return kv.Put(ctx, row, cur.Fence)
   }

// LoadReport 按 reportID 读取孤儿报告。
   func LoadReport(ctx context.Context, kv persist.KV, reportID string) (*OrphanReport, error) {
	   if kv == nil {
		  return nil, ErrStorageNil
	  }
	   row, err := kv.Get(ctx, kindOrphanReport, strings.TrimSpace(reportID))
	  if err == persist.ErrNotFound {
		  return nil, ErrReportNotFound
	  }
	  if err != nil {
		  return nil, fmt.Errorf("ledorphan load report: %w", err)
	   }
	  var r OrphanReport
	   if err := json.Unmarshal(row.Body, &r); err != nil {
		  return nil, fmt.Errorf("ledorphan load report unmarshal: %w", err)
	   }
	  return &r, nil
   }

// LoadReportByBatch 读取指定批次最新的孤儿报告。
// 若同一批次有多份报告（多次崩溃扫描产生），返回 CreatedAt 最新的一份。
  func LoadReportByBatch(ctx context.Context, kv persist.KV, batchID string) (*OrphanReport, error) {
	  if kv == nil {
		  return nil, ErrStorageNil
	  }
	   rows, err := kv.List(ctx, kindOrphanReport, 500)
	  if err != nil {
		   return nil, fmt.Errorf("ledorphan load report by batch: list: %w", err)
	   }
	  var latest *OrphanReport
	   for _, row := range rows {
		  var r OrphanReport
		  if err := json.Unmarshal(row.Body, &r); err != nil {
			   continue
		   }
		   if r.BatchID != strings.TrimSpace(batchID) {
			  continue
		   }
		   if latest == nil || r.CreatedAt.After(latest.CreatedAt) {
			   cp := r
			   latest = &cp
		   }
	  }
	  if latest == nil {
		   return nil, ErrReportNotFound
	   }
	  return latest, nil
   }

// ListReports 分页列出孤儿报告（按 CreatedAt 倒序）。
// after 为游标（上次返回的最后一条 report_id）。
  func ListReports(ctx context.Context, kv persist.KV, batchID, status string, limit int, after string) ([]OrphanReport, error) {
	   if kv == nil {
		   return nil, ErrStorageNil
	  }
	  if limit <= 0 {
		  limit = 50
	  }
	  var rows []persist.Row
	  var err error
	  if after == "" {
		   rows, // 多取一些，过滤后截断
			  err = kv.List(ctx, kindOrphanReport, limit*3)
	   } else {
		  rows, err = kv.ListAfter(ctx, kindOrphanReport, after, limit*3)
	   }
	   if err != nil {
		  return nil, fmt.Errorf("ledorphan list reports: %w", err)
	   }
	  result := make([]OrphanReport, 0, limit)
	   for _, row := range rows {
		  var r OrphanReport
		  if err := json.Unmarshal(row.Body, &r); err != nil {
			  continue
		   }
		   if batchID != "" && r.BatchID != batchID {
			   continue
		   }
		   if status != "" && string(r.Status) != status {
			  continue
		  }
		  result = append(result, r)
		   if len(result) >= limit {
			  break
		  }
	  }
	   return result, nil
  }

// MarkReportSuperseded 将同批次旧报告标记为 superseded（新报告生成时调用）。
  func MarkReportSuperseded(ctx context.Context, kv persist.KV, batchID, newReportID string) error {
	   if kv == nil {
		  return ErrStorageNil
	  }
	  rows, err := kv.List(ctx, kindOrphanReport, 500)
	   if err != nil {
		  return fmt.Errorf("ledorphan supersede reports: list: %w", err)
	  }
	   for _, row := range rows {
		   var r OrphanReport
		  if err := json.Unmarshal(row.Body, &r); err != nil {
			   continue
		   }
		  if r.BatchID != batchID || r.ReportID == newReportID {
			   continue
		   }
		  if r.Status == ReportSuperseded {
			   continue
		  }
		   r.Status = ReportSuperseded
		   body, _ := json.Marshal(r)
		  newRow := persist.Row{Kind: kindOrphanReport, Key: r.ReportID, Body: body}
		   _ = kv.Put(ctx, newRow, row.Fence) // CAS 失败忽略（已有其他 worker 更新）
	  }
	  return nil
   }

// MarkReportClosed 当所有孤儿已处置时，将报告状态改为 closed。
  func MarkReportClosed(ctx context.Context, kv persist.KV, reportID string) error {
	   if kv == nil {
		   return ErrStorageNil
	  }
	   row, err := kv.Get(ctx, kindOrphanReport, strings.TrimSpace(reportID))
	  if err == persist.ErrNotFound {
		  return ErrReportNotFound
	   }
	   if err != nil {
		   return err
	   }
	   var r OrphanReport
	  if err := json.Unmarshal(row.Body, &r); err != nil {
		   return err
	  }
	  if r.Status == ReportClosed {
		   return nil // 幂等
	  }
	   // 验证所有孤儿确实已处置。
	  for _, o := range r.Items {
		   if o.Status == OrphanDetected {
			   return fmt.Errorf("ledorphan close report: orphan %s still detected", o.OrphanID)
		   }
	  }
	  r.Status = ReportClosed
	  body, err := json.Marshal(r)
	  if err != nil {
		   return err
	   }
	  newRow := persist.Row{Kind: kindOrphanReport,
		   Key: r.ReportID, Body: body}
	  return kv.Put(ctx, newRow, row.Fence)
  }

// ── OrphanItem KV 工具（供 Sealer 外部直接调用） ─────────────────────────────
// LoadOrphanItem 按 batchID+orphanID 读取孤儿记录。

  func LoadOrphanItem(ctx context.Context, kv persist.KV, batchID, orphanID string) (*OrphanItem, error) {
	   if kv == nil {
		  return nil, ErrStorageNil
	  }
	   key := orphanItemKey(batchID, orphanID)
	   row, err := kv.Get(ctx, kindOrphanItem, key)
	  if err == persist.ErrNotFound {
		   return nil, fmt.Errorf("ledorphan: orphan item %s not found", orphanID)
	   }
	  if err != nil {
		  return nil, err
	   }
	  var o OrphanItem
	  if err := json.Unmarshal(row.Body, &o); err != nil {
		  return nil, err
	   }
	   return &o, nil
  }

// ListOrphanItemsByBatch 列出指定批次的全部孤儿记录（上限 500 条）。
  func ListOrphanItemsByBatch(ctx context.Context, kv persist.KV, batchID string) ([]OrphanItem, error) {
	  if kv == nil {
		   return nil, ErrStorageNil
	  }
	   rows, err := kv.List(ctx, kindOrphanItem, 500)
	   if err != nil {
		  return nil, fmt.Errorf("ledorphan list orphan items: %w", err)
	   }
	  result := make([]OrphanItem, 0)
	   for _, row := range rows {
		   var o OrphanItem
		   if err := json.Unmarshal(row.Body, &o); err != nil {
			   continue
		   }
		   if o.BatchID == strings.TrimSpace(batchID) {
			   result = append(result, o)
		  }
	  }
	   return result, nil
  }
