package fundmesh

import (
	"context"
	"errors"
	"strings"
	"time"
)

// SubmitWithdraw 出站提交。HTTP POST/PUT 都进这里。
  func (e *Engine) SubmitWithdraw(ctx context.Context, req OutboundRequest) (OutboundResult, error) {
	  if e == nil {
		   return OutboundResult{}, ErrUnavailable
	  }
	   if err := ValidateOutboundRequest(&req); err != nil {
		   e.noteOutboundProof(ProofPayload, req.WithdrawID, req.IdempotencyKey, "", "", CodeInput, err.Error(), 0, true)
		  return failOutbound(req, CodeInput, ""), err
	  }
	  key, parts, err := NormalizeOutboundKey(req)
	  if err != nil {
		  e.noteOutboundProof(ProofKeyInvalid, req.WithdrawID, req.IdempotencyKey, "", "", CodeInput, err.Error(), 0, true)
		  return failOutbound(req, CodeInput, ""), err
	   }
	   req.IdempotencyKey = key
	   fp, err := FingerprintOf(req)
	   if err != nil {
		   return failOutbound(req, CodeInput, ""), err
	  }
	   if err := BindKeyAndFingerprint(key, fp); err != nil {
		   return failOutbound(req, CodeInput, ""), err
	   }
	   if req.Entry == "" {
		  req.Entry = EntryPOST
	   }
	  if err := e.allowCritical(ctx); err != nil {
		   return failOutbound(req, CodeDrift, ""), err
	   }
	   if rec, err := e.lookupOutbound(ctx, key, req.WithdrawID); err == nil && rec != nil {
		   return e.replayOrConflict(ctx, req, rec, fp, parts)
	   }
	  if e.remote == nil {
		  e.denyLocal(req.WithdrawID,
			  ErrLedgerRemoteRequired.Error())
		   return failOutbound(req, CodeRemoteFail, ""), ErrLedgerRemoteRequired
	   }
	   out := asRemoteOutbound(e.remote)
	  if out == nil {
		  e.denyLocal(req.WithdrawID, ErrLedgerRemoteRequired.Error())
		  return failOutbound(req, CodeRemoteFail, ""), ErrLedgerRemoteRequired
	  }
	  intent := req.Intent()
	   intent.IdempotencyKey = key
	  intent.Fingerprint = fp
	   view, err := out.Submit(ctx, intent)
	   if err != nil {
		   e.denyLocal(emptyWD(req.WithdrawID, key), err.Error())
		   e.noteOutboundProof(ProofRemoteFail, req.WithdrawID, key, fp, "", CodeRemoteFail, err.Error(), 1, true)
		  if errors.Is(err, ErrDoubleDebitForbidden) {
			   return failOutbound(req, CodeOutboundDupDebit, ""), err
		   }
		  if mapped := mapRemote(err); mapped != nil {
			   res := failOutbound(req,
				   PhaseCode(phaseOfView(view)), phaseOfView(view))
			  res.IdempotencyKey = key
			  res.Fingerprint = fp
			   _ = e.storeOutbound(ctx, req, parts, fp, res, false, 0)
			   return res, mapped
		  }
		   res := failOutbound(req, CodeRemoteFail, "")
		  res.IdempotencyKey = key
		   res.Fingerprint = fp
		   _ = e.storeOutbound(ctx, req, parts, fp, res, false, 0)
		   return res, errors.Join(ErrLedgerRemoteFailed, err)
	   }
	   e.fault.NoteWrite()
	  wd := view.ID
	   if wd == "" {
		   wd = req.WithdrawID
	   }
	   req.WithdrawID = wd
	  res := okOutbound(req, view.Phase, false)
	   res.IdempotencyKey = key
	   res.Fingerprint = fp
	  res.Debited = true
	   res.DebitCount = 1
	  if err := e.appendDebit(ctx, req, fp, true, "submit"); err != nil {
		  e.noteOutboundProof(ProofDupDebit, wd, key, fp, string(view.Phase), CodeOutboundDupDebit, err.Error(), 2, true)
		  return res, err
	   }
	  _ = e.storeOutbound(ctx, req, parts, fp, res, true, 1)
	   return res, nil
  }

   func (e *Engine) lookupOutbound(ctx context.Context, key, withdrawID string) (*OutboundRecord, error) {
	   if e.outbounds == nil {
		   return nil, ErrWithdrawNotFound
	  }
	  if key != "" {
		   if rec, err := e.outbounds.GetByIdem(ctx, key); err == nil {
			   return rec, nil
		   }
	   }
	  if withdrawID != "" {
		   if rec, err := e.outbounds.GetByWithdraw(ctx, withdrawID); err == nil {
			   return rec, nil
		  }
	   }
	  return nil, ErrWithdrawNotFound
  }

  func (e *Engine) storeOutbound(ctx context.Context, req OutboundRequest, parts IdemKeyParts, fp string, res OutboundResult, remoteOK bool, debit int) error {
	   if e.outbounds == nil {
		  return nil
	   }
	  canon, _ := CanonicalPayload(req)
	   rec := &OutboundRecord{
		  RecordID: outboundRecordID(req.IdempotencyKey),
		   WithdrawID: req.WithdrawID,
		   UserID: req.UserID,
		   Actor:  req.Actor,
		   Asset:  req.Asset,
		  Amount: req.Amount,
		  Chain:  req.Chain,
		   Dest:   req.Dest,
		   Memo:   req.Memo,
		   Tag: req.Tag,
		  IdempotencyKey: req.IdempotencyKey,
		   Scheme:         parts.Scheme,
		  Kind:           parts.Kind,
		   Fingerprint:    fp,
		   Canon: canon,
		   State: res.State,
		  Phase: res.Phase,
		   Code:  res.Code,
		   RemoteOK: remoteOK,
		  DebitCount: debit,
		  CreatedAt:  e.nowUTC(),
		   UpdatedAt: e.nowUTC(),
	   }
	   if rec.Scheme == "" {
		   rec.Scheme = IdemSchemeV1
	  }
	   return e.outbounds.Put(ctx, rec)
   }

   func (e *Engine) appendDebit(ctx context.Context, req OutboundRequest, fp string, ok bool, note string) error {
	   if e.journals == nil {
		   return nil
	   }
	   return e.journals.Append(ctx, DebitLine{
		  LineID:         "dj-" + req.IdempotencyKey,
		   WithdrawID:     req.WithdrawID,
		  IdempotencyKey: req.IdempotencyKey,
		   Fingerprint:    fp,
		  Amount: req.Amount,
		  Asset:  req.Asset,
		  UserID: req.UserID,
		   OK:     ok,
		  Remote: true,
		  Note:   note,
		   At: e.nowUTC(),
	  })
   }

   func outboundRecordID(key string) string {
	  id := "or-" + key
	   if len(id) > 64 {
		  id = id[:64]
	  }
	  return id
   }

  func emptyWD(id, key string) string {
	  if id != "" {
		  return id
	   }
	   return key
  }

  func phaseOfView(v *WithdrawView) WithdrawPhase {
	  if v == nil {
		   return ""
	   }
	   return v.Phase
  }

   func okOutbound(req OutboundRequest, phase WithdrawPhase, replay bool) OutboundResult {
	   code := CodeOutboundOK
	  st := OutboundAccepted
	   if replay {
		  code = CodeOutboundReplay
		  st = OutboundReplayed
	  }
	  if phase == "" {
		   phase = WithdrawFrozen
	  }
	   return OutboundResult{
		   WithdrawID:     req.WithdrawID,
		  IdempotencyKey: req.IdempotencyKey,
		   State:      st,
		  Phase:      phase,
		   Code:       code,
		   Idempotent: replay,
		   Entry: req.Entry,
		  At: time.Now().UTC(),
	  }
   }

  func failOutbound(req OutboundRequest, code string, phase WithdrawPhase) OutboundResult {
	  st := OutboundDenied
	  if code == CodeOutboundConflict {
		  st = OutboundConflict
	  }
	   return OutboundResult{
		  WithdrawID:     req.WithdrawID,
		  IdempotencyKey: req.IdempotencyKey,
		   State:          st,
		   Phase: phase,
		   Code: code,
		  Entry: req.Entry,
		  At: time.Now().UTC(),
	  }
  }

   func resultFromOutbound(rec *OutboundRecord) OutboundResult {
	  if rec == nil {
		  return OutboundResult{}
	   }
	   return OutboundResult{
		  WithdrawID: rec.WithdrawID,
		   IdempotencyKey: rec.IdempotencyKey,
		   Fingerprint:    rec.Fingerprint,
		  State:          rec.State,
		  Phase: rec.Phase,
		   Code:       rec.Code,
		   Idempotent: rec.State == OutboundReplayed || rec.State == OutboundClosed || rec.State == OutboundAccepted,
		  Debited: rec.DebitCount > 0,
		   DebitCount: rec.DebitCount,
		  At:         rec.UpdatedAt,
	  }
  }

  func (e *Engine) GetOutbound(ctx context.Context, key, withdrawID string) (*OutboundRecord, error) {
	   return e.lookupOutbound(ctx, strings.TrimSpace(key), strings.TrimSpace(withdrawID))
   }
