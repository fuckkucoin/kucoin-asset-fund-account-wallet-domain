package withdraw

import (
	"context"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
)

// LiveHash 链上可见哈希。空哈希与 internal: 前缀都不是 live。
  func LiveHash(hash string) bool {
	  h := strings.TrimSpace(hash)
	   if h == "" || ledger.IsInternalTxHash(h) {
		  return false
	  }
	   return true
  }

// RefuseAutoUnfreeze 自动/批量解冻闸。已签名且从未广播（无 live hash、状态 signed）放行。
  func RefuseAutoUnfreeze(row Broadcast) error {
	   if LiveHash(row.TxHash) {
		  return ErrLiveHashUnfreeze
	   }
	  switch row.Status {
	   case BcBroadcast, BcConfirming, BcReorged, BcConfirmed:
		   return ErrLiveHashUnfreeze
	   default:
		   return nil
	  }
  }

// ManualRecord 人工推进审计。Unfrozen 为假表示没有退回可用。
type ManualRecord struct {
	TransferID string      `json:"transfer_id"`
	From       FreezeState `json:"from"`
	To         FreezeState `json:"to"`
	Actor    string    `json:"actor"`
	Approver string    `json:"approver"`
	Reason   string    `json:"reason"`
	Unfrozen bool      `json:"unfrozen"`
	At       time.Time `json:"at"`
}

type auditLog struct {
	mu       sync.Mutex
	manual   []ManualRecord
	liveDeny []LiveDeny
	kv       persist.KV
}

// LiveDeny 自动解冻被 live hash 拒绝的记录。
type LiveDeny struct {
	TransferID string `json:"transfer_id"`
	TxHash     string `json:"tx_hash,omitempty"`
	Path  string    `json:"path"`
	Alert string    `json:"alert"`
	At    time.Time `json:"at"`
}

   func newAuditLog() *auditLog { return &auditLog{} }

  func (a *auditLog) addManual(r ManualRecord) {
	   if a == nil {
		  return
	   }
	  a.mu.Lock()
	   a.manual = append(a.manual, r)
	  a.mu.Unlock()
	  _ = a.saveManual(r)
  }

  func (a *auditLog) addDeny(d LiveDeny) {
	   if a == nil {
		   return
	   }
	  a.mu.Lock()
	   a.liveDeny = append(a.liveDeny, d)
	  a.mu.Unlock()
	   _ = a.saveDeny(d)
  }

   func (a *auditLog) manuals() []ManualRecord {
	  if a == nil {
		   return nil
	   }
	  a.mu.Lock()
	  defer a.mu.Unlock()
	   return append([]ManualRecord(nil), a.manual...)
  }

  func (a *auditLog) denies() []LiveDeny {
	  if a == nil {
		  return nil
	   }
	   a.mu.Lock()
	  defer a.mu.Unlock()
	  return append([]LiveDeny(nil), a.liveDeny...)
   }

   func (s *Service) noteLiveDeny(trID, hash, path string) {
	  if s == nil || s.audits == nil {
		   return
	   }
	   s.audits.addDeny(LiveDeny{
		   TransferID: trID, TxHash: hash, Path: path, Alert: AlertLiveHashDenied, At: time.Now().UTC(),
	   })
   }

// LiveDenies 自动解冻被拒清单。
   func (s *Service) LiveDenies() []LiveDeny {
	  if s == nil || s.audits == nil {
		  return nil
	  }
	  return s.audits.denies()
   }

// ManualAdvances 人工推进审计。

   func (s *Service) ManualAdvances() []ManualRecord {
	  if s == nil || s.audits == nil {
		   return nil
	   }
	   return s.audits.manuals()
   }

   func validateDual(actor, approver, reason string) (string, string, error) {
	  a, err := validate.Actor(actor)
	  if err != nil {
		  return "", "", ErrDualApproval
	   }
	   b, err := validate.Actor(approver)
	  if err != nil {
		   return "", "", ErrDualApproval
	   }
	  if strings.EqualFold(a, b) || strings.TrimSpace(reason) == "" {
		  return "", "", ErrDualApproval
	  }
	  return a, b, nil
  }

// ManualAdvance 人工推进冻结态。actor 与 approver 必须是两名不同的人。
// live hash 下禁止解冻；允许向前记状态，但 Unfrozen 保持为假。
   func (s *Service) ManualAdvance(ctx context.Context, trID string, to FreezeState, actor, approver, reason string) (ManualRecord, error) {
	  if s == nil {
		  return ManualRecord{},
			   ErrDualApproval
	   }
	  a, b, err := validateDual(actor, approver, reason)
	   if err != nil {
		  return ManualRecord{}, err
	   }
	   trID = strings.TrimSpace(trID)
	   run, ok := s.runFor(trID)
	   if !ok {
		   return ManualRecord{},
			   ErrFreezeUnlockDenied
	  }
	   from := run.Freeze
	   if from == FreezeNone {
		   from = freezeFromStep(run.Step)
	  }
	  row, tracked := s.Broadcasts().Get(trID)
	   rec := ManualRecord{
		   TransferID: trID, From: from, To: to, Actor: a, Approver: b, Reason: strings.TrimSpace(reason), At: time.Now().UTC(),
	  }
	   if tracked {
		   if uerr := RefuseAutoUnfreeze(row); uerr != nil && (to == FreezeCancelled || to == FreezeFailed) {
			  if to == FreezeCancelled {
				   s.noteLiveDeny(trID, row.TxHash, "manual")
				   s.addManual(rec)
				  return rec,
					  ErrLiveHashUnfreeze
			  }
			  run.TransferID = trID
			   if err := s.AdvanceFreeze(ctx, &run, to, a+"+"+b, reason); err != nil {
				  return rec, err
			   }
			   s.book.Put(run)
			   s.addManual(rec)
			   return rec, nil
		   }
	   }
	  if to == FreezeCancelled {
		   st := from
		  asset := run.Asset
		   if tracked && asset == "" {
			   asset = row.Asset
		   }
		   if s.abuseHoldsUnfreeze(run.UserID, asset, st, row) {
			  s.addManual(rec)
			  return rec, ErrAbuseInFlight
		  }
		   if err := s.Cancel(ctx, trID, reason); err != nil {
			  return rec, err
		  }
		  rec.Unfrozen = true
		  s.addManual(rec)
		  return rec, nil
	   }
	   run.TransferID = trID
	   if err := s.AdvanceFreeze(ctx, &run, to, a+"+"+b, reason); err != nil {
		  return rec, err
	  }
	  s.book.Put(run)
	   s.addManual(rec)
	  return rec, nil
  }

  func (s *Service) addManual(rec ManualRecord) {
	   if s == nil || s.audits == nil {
		   return
	  }
	  s.audits.addManual(rec)
  }

// ProbeAutoUnfreeze 给批次清扫用。无跟踪且无 live hash 时允许接着走未广播恢复。

   func (s *Service) ProbeAutoUnfreeze(trID, path string) error {
	   if s == nil {
		   return ErrLiveHashUnfreeze
	  }
	  row, ok := s.Broadcasts().Get(trID)
	   if !ok {
		   return nil
	  }
	  if err := RefuseAutoUnfreeze(row); err != nil {
		   s.noteLiveDeny(trID, row.TxHash, path)
		   return err
	   }
	  run, has := s.runFor(trID)
	  if has {
		   st := run.Freeze
		   if st == FreezeNone {
			   st = freezeFromStep(run.Step)
		   }
		   asset := run.Asset
		  if asset == "" {
			   asset = row.Asset
		   }
		  if s.abuseHoldsUnfreeze(run.UserID, asset, st, row) {
			  return ErrAbuseInFlight
		  }
	  }
	   return nil
   }
