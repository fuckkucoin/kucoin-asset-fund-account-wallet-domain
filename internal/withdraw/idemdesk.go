// @FileName: idemdesk.go
// @Summary: 出金幂等柜台。
// 同意图重放原单；冲突明确失败；半冻结接着原单或转事故。continue 与 incident 都不再调用冻账。半冻结被当成新键会再冻一笔。
// @Author: ColeHale
// @Created: 2021.01.13

package withdraw

import (
	"context"
	"errors"
	"strings"
	"sync"
	"account-assets.kucoin.net/funding/internal/command"
	"account-assets.kucoin.net/funding/internal/idemstore"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/persist"
)

const idemActor = "wd-idem-desk"

// DeskView 柜台只读结果，供运营核对。
type DeskView struct {
	Scope string `json:"scope"`
	Key string `json:"idempotency_key"`
	Fingerprint string               `json:"fingerprint"`
	RefID       string               `json:"ref_id,omitempty"`
	Status      string               `json:"status,omitempty"`
	Action      idemstore.DeskAction `json:"action"`
	Frozen      bool                 `json:"frozen"`
	Reason string `json:"reason,omitempty"`
	Alert  string `json:"alert,omitempty"`
}

type idemHold struct {
	Key         string
	Fingerprint string
	RefID  string
	UserID string
	Asset string
	Amount string
	Frozen bool
}

type idemIndex struct {
	mu sync.Mutex
	rows map[string]*idemHold
	kv   persist.KV
}

   func newIdemIndex() *idemIndex { return &idemIndex{rows: map[string]*idemHold{}} }

  func (x *idemIndex) put(h idemHold) error {
	   if x == nil || h.Key == "" {
		  return nil
	   }
	   x.mu.Lock()
	   cp := h
	   x.rows[h.Key] = &cp
	   x.mu.Unlock()
	   return x.save(h)
  }

  func (x *idemIndex) get(key string) (idemHold, bool) {
	  if x == nil {
		  return idemHold{}, false
	  }
	   x.mu.Lock()
	   defer x.mu.Unlock()
	  h, ok := x.rows[key]
	   if !ok {
		   return idemHold{}, false
	  }
	   return *h, true
   }

  func (x *idemIndex) unfreeze(ref string) {
	  if x == nil {
		  return
	  }
	  x.mu.Lock()
	   var changed []idemHold
	   for _, h := range x.rows {
		  if h.RefID == ref {
			   h.Frozen = false
			   changed = append(changed, *h)
		   }
	   }
	  x.mu.Unlock()
	   for _, h := range changed {
		   _ = x.save(h)
	  }
  }

   func (s *Service) rememberFreeze(key, fp, ref, user, asset, amount string) {
	   if s == nil || s.idemIdx == nil {
		   return
	  }
	  if err := s.idemIdx.put(idemHold{
		   Key: key, Fingerprint: fp, RefID: ref, UserID: user, Asset: asset, Amount: amount, Frozen: true,
	  }); err != nil && s.conserve != nil {
		   // 半冻结引用没落上时，后续新出金失败即关，避免重启后把同一键再冻一笔。
		  s.conserve.setStoreErr(err)
	   }
   }

   func (s *Service) markIdemUnfrozen(ref string) {
	   if s == nil || s.idemIdx == nil {
		   return
	   }
	   s.idemIdx.unfreeze(ref)
  }

  func (s *Service) localHold(key, fp string) (idemHold, bool) {
	  if s == nil || s.idemIdx == nil {
		   return idemHold{}, false
	   }
	  h, ok := s.idemIdx.get(key)
	  if !ok {
		  return idemHold{}, false
	  }
	  if h.Fingerprint != "" && fp != "" && h.Fingerprint != fp {
		   return h, true
	   }
	   return h, true
  }

// InspectIdem 只读裁决，不 Begin、不冻账。
   func (s *Service) InspectIdem(ctx context.Context, req Request) (DeskView, error) {
	   if s == nil {
		  return DeskView{}, ErrHalfFrozen
	   }
	   fp := idemstore.Fingerprint(req.UserID,
		   req.Asset, req.Amount.String(), req.Chain, req.ToAddress, req.Memo, req.TagName)
	  payload := command.HashPayload("withdraw", req.UserID, req.Asset, req.Amount.String(), req.Chain, req.ToAddress, req.Memo, req.TagName)
	   return s.deskView(ctx, req.IdempotencyKey, fp, payload, req.UserID)
  }

  func (s *Service) deskView(ctx context.Context, key, fp, payload, user string) (DeskView, error) {
	  view := DeskView{Scope: idemstore.ScopeWithdraw, Key: strings.TrimSpace(key), Fingerprint: fp}
	   local, has := s.localHold(key, fp)
	  frozen := has && local.Frozen
	   ref := ""
	  if has {
		   ref = local.RefID
		   if local.Fingerprint != "" && local.Fingerprint != fp {
			  view.Action = idemstore.DeskConflict
			  view.Frozen = frozen
			   view.Reason = "local intent differs"
			  view.RefID = ref
			  return view, ErrIdemConflict
		  }
	   }
	   var rec *idemstore.Record
	   if s.idem != nil {
		   got, err := s.idem.Get(ctx, idemstore.ScopeWithdraw, key)
		  if err != nil && !errors.Is(err, idemstore.ErrNotFound) {
			  return view, err
		  }
		  rec = got
		   if rec != nil {
			  view.Status = rec.Status
			   if rec.RefID != "" {
				   ref = rec.RefID
			  }
		  }
	  }
	  if ref == "" && s.cmds != nil {
		  if cr, ok := s.cmds.Get(key); ok && cr.ResultID != "" && cr.Hash == payload {
			   ref = cr.ResultID
		   }
	   }
	   var act idemstore.DeskAction
	  var err error
	   switch {
	  case rec == nil && s.idem == nil:
		   act, err = s.decideCommand(key, fp, payload, frozen, ref)
	   case rec == nil && (frozen || ref != ""):
		  if ref == "" {
			  act = idemstore.DeskIncident
		   } else {
			   act = idemstore.DeskContinue
		   }
	  default:
		  act, err = idemstore.Decide(idemstore.ScopeWithdraw, rec, fp, frozen || ref != "")
	  }
	   if err != nil && !errors.Is(err, idemstore.ErrConflict) {
		   return view, err
	   }
	  view.Action = act
	  view.Frozen = frozen || ref != ""
	   view.RefID = ref
	  _ = user
	   switch act {
	  case idemstore.DeskConflict:
		  view.Reason = "same key different intent"
		  return view, ErrIdemConflict
	   case idemstore.DeskIncident:
		   view.Reason = "inflight without freeze ref"
		  view.Alert = AlertHalfFrozen
	   case idemstore.DeskContinue:
		  view.Reason = "half-frozen resume original"
	  case idemstore.DeskReplay:
		   view.Reason = "same intent replay"
	  default:
		   view.Reason = "new"
	  }
	  return view, nil
  }

  func (s *Service) decideCommand(key, fp, payload string, frozen bool, ref string) (idemstore.DeskAction, error) {
	   if s == nil || s.cmds == nil {
		  if ref != "" {
			   return idemstore.DeskContinue, nil
		  }
		  if frozen {
			   return idemstore.DeskIncident, nil
		   }
		  return idemstore.DeskNew, nil
	  }
	  cr, ok := s.cmds.Get(key)
	   if !ok {
		   if ref != "" {
			  return idemstore.DeskContinue, nil
		   }
		   if frozen {
			   return idemstore.DeskIncident,
				   nil
		  }
		  return idemstore.DeskNew, nil
	  }
	   if cr.Hash != payload {
		  return idemstore.DeskConflict, idemstore.ErrConflict
	   }
	  if cr.ResultID != "" {
		  ref = cr.ResultID
	  }
	   rec := &idemstore.Record{Scope: idemstore.ScopeWithdraw, Key: key, Fingerprint: fp, RefID: ref}
	  switch cr.Status {
	   case command.StatusSucceeded:
		  rec.Status = idemstore.StatusDone
	  case command.StatusFailed:
		  rec.Status = idemstore.StatusFailed
	   default:
		  rec.Status = idemstore.StatusInflight
	  }
	   return idemstore.Decide(idemstore.ScopeWithdraw, rec, fp, frozen || rec.RefID != "")
   }

// prepareIdem 在冻账前收口。返回的 transfer 非空表示直接回放，不再 Create。

   func (s *Service) prepareIdem(ctx context.Context, req Request, fp, payload string) (*ledger.Transfer, error) {
	  view, err := s.deskView(ctx, req.IdempotencyKey, fp, payload, req.UserID)
	   if err != nil && !errors.Is(err, ErrIdemConflict) {
		   return nil, err
	   }
	   switch view.Action {
	  case idemstore.DeskConflict:
		  return nil, ErrIdemConflict
	   case idemstore.DeskReplay, idemstore.DeskContinue:
		   return s.finishReplay(ctx,
			   req, view)
	  case idemstore.DeskIncident:
		   s.markHalf(req.UserID,
			  req.IdempotencyKey, view.Reason)
		   return nil, ErrHalfFrozen
	   }
	  // 新冻结才过阻断与热库存只读闸。回放与接着不受阻断影响，避免把原单报成新的失败。
	  if err := s.AllowNew(req.UserID, req.Asset); err != nil {
		  return nil, err
	   }
	   if s.hotPre != nil {
		  if err := s.hotPre(ctx, req.Asset); err != nil {
			  return nil, err
		   }
	   }
	   if s.idem != nil {
		   rec, berr := s.idem.Begin(ctx, idemstore.ScopeWithdraw, req.IdempotencyKey, fp, payload)
		  if berr != nil {
			   if errors.Is(berr, idemstore.ErrConflict) {
				  return nil, ErrIdemConflict
			  }
			  if errors.Is(berr, idemstore.ErrInflight) {
				  if view.RefID != "" {
					   return s.loadRef(ctx, req, view.RefID, idemstore.DeskContinue)
				  }
				  s.markHalf(req.UserID, req.IdempotencyKey, "begin inflight")
				   return nil, ErrHalfFrozen
			  }
			   return nil, berr
		  }
		  if rec != nil && rec.Status == idemstore.StatusDone && rec.RefID != "" {
			   return s.loadRef(ctx, req, rec.RefID, idemstore.DeskReplay)
		   }
	  }
	   crec, existed, cerr := s.cmds.Begin(req.IdempotencyKey, idemstore.ScopeWithdraw, payload)
	   if cerr != nil {
		  if errors.Is(cerr, command.ErrConflict) {
			  return nil, ErrIdemConflict
		  }
		   if errors.Is(cerr, command.ErrInFlight) {
			  ref := view.RefID
			   if crec != nil && crec.ResultID != "" {
				  ref = crec.ResultID
			  }
			   if ref != "" {
				  return s.loadRef(ctx, req, ref, idemstore.DeskContinue)
			  }
			   s.markHalf(req.UserID, req.IdempotencyKey, "command inflight")
			   return nil, ErrHalfFrozen
		  }
		   return nil, cerr
	   }
	   if existed && crec != nil && crec.Status == command.StatusSucceeded && crec.ResultID != "" {
		  if s.idem != nil {
			  _ = s.idem.Complete(ctx, idemstore.ScopeWithdraw, req.IdempotencyKey, crec.ResultID)
		   }
		   return s.loadRef(ctx, req, crec.ResultID, idemstore.DeskReplay)
	   }
	  if existed && crec != nil && crec.Status == command.StatusFailed {
		  local, has := s.localHold(req.IdempotencyKey, fp)
		  if has && local.Frozen {
			   ref := local.RefID
			   if ref == "" {
				  ref = crec.ResultID
			  }
			  if ref != "" {
				  return s.loadRef(ctx, req, ref, idemstore.DeskContinue)
			   }
			   s.markHalf(req.UserID, req.IdempotencyKey, "failed key still frozen")
			  return nil, ErrHalfFrozen
		   }
	   }
	   return nil, nil
   }

   func (s *Service) finishReplay(ctx context.Context, req Request, view DeskView) (*ledger.Transfer, error) {
	   if view.RefID == "" {
		   s.markHalf(req.UserID, req.IdempotencyKey, "replay without ref")
		   return nil, ErrHalfFrozen
	   }
	   return s.loadRef(ctx, req, view.RefID, view.Action)
  }

   func (s *Service) loadRef(ctx context.Context, req Request, ref string, act idemstore.DeskAction) (*ledger.Transfer, error) {
	   tr, err := s.led.Get(ctx, ref)
	   if err != nil || tr == nil {
		  s.markHalf(req.UserID, req.IdempotencyKey, string(act)+" ref missing")
		  return nil, ErrHalfFrozen
	   }
	  return tr, nil
   }

   func (s *Service) markHalf(user, key, reason string) {
	   if s == nil || s.tickets == nil || strings.TrimSpace(user) == "" {
		   return
	  }
	   noteKey := "half|" + strings.TrimSpace(key)
	   if s.conserve != nil {
		  if _, ok := s.conserve.seenTicket(noteKey); ok {
			  return
		   }
	   }
	  note := AlertHalfFrozen + " " + strings.TrimSpace(reason)
	   if note == AlertHalfFrozen+" " || strings.TrimSpace(reason) == "" {
		   note = AlertHalfFrozen + " half-frozen key " + strings.TrimSpace(key)
	   }
	   ex, err := s.tickets.OpenException(idemActor, user, "", string(TicketHalfFreeze), note)
	  if err != nil || s.conserve == nil {
		   return
	   }
	   s.conserve.markTicket(noteKey, ex.ID)
   }
