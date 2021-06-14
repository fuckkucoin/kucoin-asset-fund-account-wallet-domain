// 重点：幂等、终态、错误码与入口无关。
// 风险点：远端失败不得改写进程内账本。

// Author: Knox
// Date: 2021.06.13

package fundmesh

import (
	"context"
	"errors"
	"strings"
	"time"
)

// CancelWithdraw 双入口唯一写路径。

   func (e *Engine) CancelWithdraw(ctx context.Context, req CancelRequest) (CancelResult, error) {
	   if e == nil {
		  return CancelResult{},
			   ErrUnavailable
	   }
	   req.WithdrawID = strings.TrimSpace(req.WithdrawID)
	  req.Reason = strings.TrimSpace(req.Reason)
	   req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	   if req.WithdrawID == "" {
		  return failResult(req, CodeInput, WithdrawInitiated), ErrWithdrawIDRequired
	   }
	   if req.Reason == "" {
		   req.Reason = "cancel"
	   }
	  if req.Entry == "" {
		   req.Entry = entryFromMethod(req.Method)
	  }
	   if err := e.allowCritical(ctx); err != nil {
		   return failResult(req, CodeDrift, ""), err
	  }
	   if rec, err := e.lookupIdem(ctx, req); err == nil && rec != nil {
		  out := resultFromRecord(rec)
		  out.Idempotent = true
		  out.Entry = req.Entry
		   if rec.Code == CodeOK || rec.Code == CodeIdempotent {
			  out.Code = CodeIdempotent
			  out.State = CancelIdempotent
			   return out, nil
		   }
		   return out, codeToErr(rec.Code)
	   }
	   view, err := e.loadView(ctx, req.WithdrawID)
	  if err != nil {
		  return failResult(req, CodeNotFound, ""), err
	   }
	   if req.UserID != "" && view.UserID != "" && req.UserID != view.UserID {
		  return failResult(req, CodeDenied, view.Phase), ErrActorDenied
	   }
	  if view.Phase == WithdrawCancelled {
		   res := okResult(req, view.Phase, true)
		  _ = e.storeRecord(ctx, req, res, true)
		  e.alignOutboundAfterCancel(ctx, req, res, true)
		  return res, nil
	   }
	   if err := PhaseError(view.Phase); err != nil {
		   res := failResult(req, PhaseCode(view.Phase), view.Phase)
		  _ = e.storeRecord(ctx, req, res, false)
		   return res, err
	  }
	   if e.remote == nil {
		   e.denyLocal(req.WithdrawID, ErrLedgerRemoteRequired.Error())
		  return failResult(req, CodeRemoteFail, view.Phase), ErrLedgerRemoteRequired
	   }
	   if err := e.remote.Cancel(ctx, req.WithdrawID, req.Reason); err != nil {
		  e.denyLocal(req.WithdrawID, err.Error())
		  if mapped := mapRemote(err); mapped != nil {
			   res := failResult(req,
				   PhaseCode(view.Phase), view.Phase)
			  if errors.Is(mapped, ErrWithdrawNotFound) {
				  res.Code = CodeNotFound
			   }
			   _ = e.storeRecord(ctx, req, res, false)
			   return res, mapped
		   }
		  res := failResult(req, CodeRemoteFail, view.Phase)
		   _ = e.storeRecord(ctx, req, res, false)
		   return res, errors.Join(ErrLedgerRemoteFailed, err)
	  }
	  e.fault.NoteWrite()
	  res := okResult(req, WithdrawCancelled, false)
	   _ = e.storeRecord(ctx, req, res, true)
	   e.alignOutboundAfterCancel(ctx, req, res, true)
	  return res, nil
  }

  func (e *Engine) lookupIdem(ctx context.Context, req CancelRequest) (*CancelRecord, error) {
	  if e.cancels == nil {
		   return nil, ErrWithdrawNotFound
	   }
	  if req.IdempotencyKey != "" {
		  if rec, err := e.cancels.GetByIdem(ctx, req.IdempotencyKey); err == nil {
			  return rec, nil
		   }
	  }
	   if rec, err := e.cancels.GetByWithdraw(ctx, req.WithdrawID); err == nil {
		  return rec, nil
	   }
	  return nil, ErrWithdrawNotFound
  }

  func (e *Engine) loadView(ctx context.Context, id string) (*WithdrawView, error) {
	  if e.remote == nil {
		   return nil,
			  ErrLedgerRemoteRequired
	  }
	  return e.remote.Get(ctx, id)
   }

   func (e *Engine) denyLocal(id, remoteErr string) {
	  if e.local != nil {
		   // 显式拒绝：即便注入进程内面也不调用 CancelLocal。
	   }
	   if e.proofs != nil {
		   e.proofs.PutFallback(FallbackProof{
			  ProofID:    "fb-" + id,
			  WithdrawID: id,
			  RemoteErr:  remoteErr,
			  LocalTried: false,
			   Denied:     true,
			  At: e.nowUTC(),
		  })
	   }
   }

   func (e *Engine) storeRecord(ctx context.Context, req CancelRequest, res CancelResult, remoteOK bool) error {
	  if e.cancels == nil {
		   return nil
	  }
	   rec := &CancelRecord{
		  RecordID: "cr-" + req.WithdrawID,
		   WithdrawID:     req.WithdrawID,
		  UserID:         req.UserID,
		  Actor:          req.Actor,
		  Reason:         req.Reason,
		   IdempotencyKey: req.IdempotencyKey,
		   Entry: req.Entry,
		   State: res.State,
		   Phase:    res.Phase,
		   Code:     res.Code,
		  RemoteOK: remoteOK,
		   CreatedAt: e.nowUTC(),
		   UpdatedAt: e.nowUTC(),
	  }
	   return e.cancels.Put(ctx, rec)
   }

  func entryFromMethod(method string) EntryKind {
	  if strings.EqualFold(method, "DELETE") {
		   return EntryDELETE
	   }
	   return EntryPOST
   }

  func okResult(req CancelRequest, phase WithdrawPhase, idem bool) CancelResult {
	  code := CodeOK
	  st := CancelAccepted
	   if idem {
		  code = CodeIdempotent
		   st = CancelIdempotent
	   }
	  return CancelResult{
		   WithdrawID: req.WithdrawID,
		   State:      st,
		  Phase:      phase,
		  Code:       code,
		  Idempotent: idem,
		   Entry:      req.Entry,
		  Reason:     req.Reason,
		   At:         timeNow(),
	  }
  }

  func failResult(req CancelRequest, code string, phase WithdrawPhase) CancelResult {
	  return CancelResult{
		  WithdrawID: req.WithdrawID,
		   State: CancelDenied,
		  Phase: phase,
		  Code:  code,
		  Entry:  req.Entry,
		  Reason: req.Reason,
		  At: timeNow(),
	  }
  }

   func resultFromRecord(rec *CancelRecord) CancelResult {
	  return CancelResult{
		   WithdrawID: rec.WithdrawID,
		   State: rec.State,
		   Phase: rec.Phase,
		  Code:  rec.Code,
		  Idempotent: rec.Code == CodeIdempotent || rec.Code == CodeOK,
		  Entry:      rec.Entry,
		   Reason: rec.Reason,
		   At:     rec.UpdatedAt,
	  }
  }

  func mapRemote(err error) error {
	  if err == nil {
		   return nil
	  }
	   if errors.Is(err, ErrAlreadyBroadcast) || errors.Is(err, ErrAlreadyConfirmed) ||
		   errors.Is(err, ErrAlreadyFailed) || errors.Is(err, ErrCancelTerminal) ||
		  errors.Is(err, ErrWithdrawNotFound) || errors.Is(err, ErrCancelDenied) {
		  return err
	  }
	   return nil
  }

   func codeToErr(code string) error {
	  switch code {
	  case CodeNotFound:
		   return ErrWithdrawNotFound
	   case CodeBroadcast:
		  return ErrAlreadyBroadcast
	  case CodeConfirmed:
		   return ErrAlreadyConfirmed
	  case CodeFailed:
		   return ErrAlreadyFailed
	  case CodeDenied:
		  return ErrCancelDenied
	  case CodeRemoteFail:
		   return ErrLedgerRemoteFailed
	   case CodeDrift:
		   return ErrCriticalWriteClosed
	   default:
		  return ErrCancelDenied
	  }
   }

  func timeNow() time.Time {
	  return timeNowUTC()
  }

   func timeNowUTC() time.Time { return time.Now().UTC() }
// temporary guard removed after the production check
