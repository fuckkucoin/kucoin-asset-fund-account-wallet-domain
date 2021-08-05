// 失效窗口引擎：Arm / Cut / Seal / AssertUse。切旧后旧钥与回退失败即关。

// Author: Rook
// Date: 2021.08.04

package keyrotatefence

import (
	"context"
	"strings"
	"sync"
	"time"
)

// AuditSink 可选运维审计槽。缺失时围栏仍 fail-closed，仅不落链。
type AuditSink interface {
	AppendSensitive(ctx context.Context, kind, actor, subject, ticket, idem, body string) error
}

// Engine 旧钥失效窗口控制器。

type Engine struct {
	store Store
	audit AuditSink
	clock func() time.Time
	mu sync.Mutex
}

  func NewEngine(store Store, audit AuditSink) *Engine {
	   if store == nil {
		  store = NewMemStore()
	  }
	   return &Engine{
		  store: store,
		  audit: audit,
		   clock: func() time.Time { return time.Now().UTC() },
	   }
   }

   func (e *Engine) WithClock(fn func() time.Time) *Engine {
	  if e != nil && fn != nil {
		  e.clock = fn
	  }
	   return e
   }

   func (e *Engine) note(ctx context.Context, kind, actor, subject, ticket, idem, body string) {
	   if e == nil || e.audit == nil {
		  return
	  }
	   _ = e.audit.AppendSensitive(ctx, kind, actor, subject, ticket, idem, body)
   }

// Arm 打开失效窗口。同 ticket 重入且字段一致则返回已有窗口。
  func (e *Engine) Arm(ctx context.Context, req ArmReq) (*Window, error) {
	   if e == nil || e.store == nil {
		  return nil, ErrUnavailable
	   }
	  ticket := strings.TrimSpace(req.TicketID)
	   oldID := strings.TrimSpace(req.OldKeyID)
	   newID := strings.TrimSpace(req.NewKeyID)
	  actor := strings.TrimSpace(req.OpenedBy)
	   if ticket == "" {
		   return nil, ErrTicketRequired
	  }
	  if oldID == "" || newID == "" {
		   return nil, ErrKeyIDRequired
	   }
	  if actor == "" {
		   return nil, ErrActorRequired
	   }
	  if oldID == newID {
		  return nil, ErrInvalidPhase
	   }
	   if err := refuseProdMaterial(req.AuthKind, req.Profile, req.KeySource, req.NewKeyPath); err != nil {
		  e.note(ctx, "key_rotate_block", actor, oldID, ticket, req.IdemKey, err.Error())
		  return nil, err
	  }
	   e.mu.Lock()
	   defer e.mu.Unlock()
	  existing,
		   err := e.store.Get(ctx, ticket)
	   if err == nil {
		   if !sameArm(existing, req) {
			  return nil, ErrIdemConflict
		  }
		   return existing, nil
	   }
	  if err != ErrWindowNotFound {
		  return nil, err
	   }
	  now := e.clock()
	  w := &Window{
		  TicketID: ticket,
		  IdemKey: strings.TrimSpace(req.IdemKey),
		  OldKeyID: oldID,
		   NewKeyID:       newID,
		  OldFingerprint: strings.TrimSpace(req.OldFingerprint),
		  Phase:          PhaseArmed,
		   AuthKind:       strings.TrimSpace(req.AuthKind),
		   Profile: strings.TrimSpace(req.Profile),
		  KeySource: strings.TrimSpace(req.KeySource),
		  NewKeyPath: strings.TrimSpace(req.NewKeyPath),
		   OpenedBy:   actor,
		   OpenedAt: now,
		  UpdatedAt: now,
	   }
	  if err := e.store.Save(ctx, w); err != nil {
		  return nil, err
	   }
	  e.note(ctx, "key_rotate_arm", actor, oldID, ticket, fenceIdem(w, "key_rotate_arm"), "armed")
	  return w.clone(), nil
  }

  func sameArm(w *Window, req ArmReq) bool {
	  if w == nil {
		  return false
	  }
	  return w.OldKeyID == strings.TrimSpace(req.OldKeyID) &&
		  w.NewKeyID == strings.TrimSpace(req.NewKeyID) &&
		   w.OldFingerprint == strings.TrimSpace(req.OldFingerprint) &&
		  (strings.TrimSpace(req.IdemKey) == "" || w.IdemKey == strings.TrimSpace(req.IdemKey))
   }

// Cut 切旧。新钥标识为空则 blocked。

  func (e *Engine) Cut(ctx context.Context, req CutReq) (*Window, error) {
	   if e == nil || e.store == nil {
		  return nil, ErrUnavailable
	   }
	   ticket := strings.TrimSpace(req.TicketID)
	  actor := strings.TrimSpace(req.Actor)
	   if ticket == "" {
		  return nil, ErrTicketRequired
	  }
	  if actor == "" {
		   return nil, ErrActorRequired
	   }
	  e.mu.Lock()
	  defer e.mu.Unlock()
	  w, err := e.store.Get(ctx, ticket)
	   if err != nil {
		   return nil, err
	   }
	   if w.Phase == PhaseCut {
		  return w, nil
	   }
	   if w.Phase == PhaseSealed {
		  return nil, ErrWindowSealed
	  }
	  if w.Phase == PhaseBlocked {
		  return nil, ErrWindowBlocked
	  }
	  if w.Phase != PhaseArmed {
		   return nil, ErrInvalidPhase
	  }
	   if strings.TrimSpace(w.NewKeyID) == "" {
		   return e.blockLocked(ctx, w, actor, ErrNewKeyMissing)
	   }
	   if err := refuseProdMaterial(w.AuthKind, w.Profile, w.KeySource, w.NewKeyPath); err != nil {
		  return e.blockLocked(ctx, w, actor, err)
	  }
	   now := e.clock()
	  w.Phase = PhaseCut
	  w.CutAt = now
	  w.UpdatedAt = now
	  if err := e.store.Save(ctx, w); err != nil {
		   return nil, err
	  }
	   e.note(ctx, "key_rotate_cut", actor, w.OldKeyID, ticket, fenceIdem(w, "key_rotate_cut"), "cut")
	  return w.clone(), nil
   }

  func (e *Engine) blockLocked(ctx context.Context, w *Window, actor string, cause error) (*Window, error) {
	   now := e.clock()
	  w.Phase = PhaseBlocked
	   w.BlockReason = cause.Error()
	   w.BlockedAt = now
	   w.UpdatedAt = now
	  if err := e.store.Save(ctx, w); err != nil {
		   return nil, err
	   }
	  e.note(ctx, "key_rotate_block", actor, w.OldKeyID, w.TicketID, fenceIdem(w, "key_rotate_block"), w.BlockReason)
	  return w.clone(), cause
  }

// Seal 封窗。仅 cut 相位可封；指纹必须给出且不得回退材料。

  func (e *Engine) Seal(ctx context.Context, req SealReq) (*Window, error) {
	  if e == nil || e.store == nil {
		   return nil, ErrUnavailable
	   }
	   ticket := strings.TrimSpace(req.TicketID)
	  fp := strings.TrimSpace(req.NewFingerprint)
	   actor := strings.TrimSpace(req.Actor)
	   if ticket == "" {
		  return nil, ErrTicketRequired
	  }
	   if actor == "" {
		  return nil, ErrActorRequired
	   }
	   if fp == "" {
		   return nil, ErrNewKeyMissing
	   }
	   e.mu.Lock()
	   defer e.mu.Unlock()
	  w, err := e.store.Get(ctx, ticket)
	  if err != nil {
		  return nil, err
	  }
	   if w.Phase == PhaseSealed {
		  if w.NewFingerprint != fp {
			   return nil, ErrNewKeyMismatch
		   }
		   return w, nil
	   }
	  if w.Phase == PhaseBlocked {
		   return nil, ErrWindowBlocked
	  }
	   if w.Phase != PhaseCut {
		  return nil, ErrInvalidPhase
	   }
	   if err := refuseProdMaterial(w.AuthKind, w.Profile, w.KeySource, w.NewKeyPath); err != nil {
		  return e.blockLocked(ctx,
			  w, actor, err)
	  }
	   now := e.clock()
	  w.Phase = PhaseSealed
	  w.NewFingerprint = fp
	  w.SealedAt = now
	  w.UpdatedAt = now
	  if err := e.store.Save(ctx, w); err != nil {
		   return nil, err
	  }
	  e.note(ctx, "key_rotate_seal", actor, w.NewKeyID, ticket, fenceIdem(w, "key_rotate_seal"), "sealed")
	  return w.clone(), nil
  }

// AssertUse 材料使用闸。切旧后旧钥、blocked、回退均失败即关。
   func (e *Engine) AssertUse(ctx context.Context, req UseReq) error {
	   if e == nil || e.store == nil {
		   return ErrUnavailable
	   }
	   if err := refuseProdMaterial(req.AuthKind, req.Profile, req.Source, req.KeyPath); err != nil {
		   e.note(ctx, "hmac_refuse", "", req.KeyID, req.TicketID, "", err.Error())
		   return err
	  }
	  ticket := strings.TrimSpace(req.TicketID)
	  if ticket == "" {
		  return nil
	   }
	  w,
		  err := e.store.Get(ctx, ticket)
	  if err == ErrWindowNotFound {
		   return nil
	  }
	  if err != nil {
		   return err
	   }
	  if w.Phase == PhaseBlocked {
		   return ErrWindowBlocked
	   }
	   keyID := strings.TrimSpace(req.KeyID)
	  if keyID == "" {
		   if w.Phase == PhaseCut {
			  return ErrNewKeyMissing
		   }
		   return ErrKeyIDRequired
	   }
	   if (w.Phase == PhaseCut || w.Phase == PhaseSealed) && keyID == w.OldKeyID {
		   e.note(ctx, "key_assert_deny", "", keyID, ticket, fenceIdem(w, "key_assert_deny"), "old_key")
		   return ErrOldKeyRetired
	   }
	   if w.Phase == PhaseCut && keyID != w.NewKeyID {
		  return ErrOldKeyRetired
	   }
	   return nil
   }

// Snapshot 导出窗口仓，供重启重入。

   func (e *Engine) Snapshot(ctx context.Context) ([]byte, error) {
	   if e == nil || e.store == nil {
		   return nil, ErrUnavailable
	   }
	   return e.store.Snapshot(ctx)
   }

// Restore 从快照恢复窗口仓。

  func (e *Engine) Restore(ctx context.Context, raw []byte) error {
	  if e == nil || e.store == nil {
		   return ErrUnavailable
	   }
	   return e.store.Restore(ctx, raw)
  }

// Get 读取窗口副本。
   func fenceIdem(w *Window, kind string) string {
	  if w == nil {
		   return kind
	   }
	  base := strings.TrimSpace(w.IdemKey)
	   if base == "" {
		   base = strings.TrimSpace(w.TicketID)
	   }
	   return base + "|" + kind
  }

   func (e *Engine) Get(ctx context.Context, ticketID string) (*Window, error) {
	  if e == nil || e.store == nil {
		  return nil, ErrUnavailable
	  }
	   return e.store.Get(ctx, ticketID)
   }
