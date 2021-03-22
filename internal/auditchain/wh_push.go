package auditchain

import (
	"context"
	"errors"
	"account-assets.kucoin.net/funding/internal/auditchain/ingest"
)

// Push 实现摄取 Sink。迟到的水位记告警并返回游标错误，条目仍留在仓里。

  func (w *Warehouse) Push(ctx context.Context, f ingest.Fact) error {
	   if w == nil || w.st == nil {
		   return ErrUnavailable
	   }
	   w.mu.Lock()
	  defer w.mu.Unlock()
	   row, err := w.appendLocked(ctx, Envelope{
		  Source: f.Source, IdemKey: f.IdemKey, Kind: f.Kind, Domain: f.Domain, Tenant: f.Tenant,
		   UserID: f.UserID, Asset: f.Asset, Amount: f.Amount, RefID: f.RefID, Actor: f.Actor,
		  Body: f.Body, OccurredAt: f.OccurredAt,
	  })
	  if err != nil {
		   return err
	  }
	  if row.Duplicate {
		   return nil
	  }
	  _, err = w.advanceCursorLocked(ctx, f.Source, Water(f.OccurredAt, f.IdemKey), row.ID, f.OccurredAt)
	   if errors.Is(err, ErrCursor) {
		   _ = w.raise(ctx, AlertLate, "", f.Source)
		  return err
	   }
	   return err
  }

// NoteLate 水位倒退时留一条迟到告警。
  func (w *Warehouse) NoteLate(ctx context.Context, source string) error {
	  if w == nil {
		   return ErrUnavailable
	   }
	   w.mu.Lock()
	   defer w.mu.Unlock()
	  return w.raise(ctx, AlertLate, "", source)
   }
