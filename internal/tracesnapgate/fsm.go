package tracesnapgate

import "time"

// ─── 转换检查 ───────────────────────────────────────────────────────────────
// CanSeal 检查快照是否可密封。
   func CanSeal(s *TraceSnapshot) error {
	   if s.Status == StatusSealed {
		   return ErrSealedAlready
	   }
	   if s.Status != StatusOpen {
		   return ErrNotOpen
	  }
	  return nil
   }

// CanReplay 检查快照是否可重放复验。

   func CanReplay(s *TraceSnapshot) error {
	   if s.Status != StatusSealed && s.Status != StatusExported {
		  if s.Status == StatusSuperseded {
			  return ErrSuperseded
		   }
		  return ErrNotSealed
	  }
	  return nil
   }

// CanExport 检查快照是否可法医导出。

   func CanExport(s *TraceSnapshot) error {
	  if s.Status == StatusSuperseded {
		   return ErrSuperseded
	   }
	   if s.Status != StatusSealed && s.Status != StatusExported {
		   return ErrNotSealed
	   }
	   return nil
  }

// CanSupersede 检查快照是否可被取代（open 或 sealed 均可）。
  func CanSupersede(s *TraceSnapshot) error {
	   if s.IsTerminal() {
		  return ErrSuperseded
	  }
	  return nil
  }

// ─── 状态转换应用 ────────────────────────────────────────────────────────────
// ApplySeal 将快照转换为 sealed 状态，固化摘要根和密封时刻。
// 前置条件：CanSeal(s) == nil。
// 副作用：修改 s.Status、s.AsOf、s.SealedAt、s.TransfersRoot、s.BooksRoot。
   func ApplySeal(s *TraceSnapshot, transfersRoot, booksRoot string, now time.Time) {
	  s.Status = StatusSealed
	   s.AsOf = now
	   s.SealedAt = &now
	   s.TransfersRoot = transfersRoot
	   s.BooksRoot = booksRoot
   }

// ApplyExport 将快照转换为 exported 状态，记录导出时刻。
// 前置条件：CanExport(s) == nil。

   func ApplyExport(s *TraceSnapshot, now time.Time) {
	   s.Status = StatusExported
	   s.ExportedAt = &now
  }

// ApplySupersede 将快照转换为 superseded 状态，记录取代者和时刻。
// 前置条件：CanSupersede(s) == nil。

  func ApplySupersede(s *TraceSnapshot, newSnapshotID string, now time.Time) {
	  s.Status = StatusSuperseded
	  s.SupersededAt = &now
	  s.SupersededBy = newSnapshotID
   }
