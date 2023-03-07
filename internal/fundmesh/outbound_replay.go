// 功能：出站重复提交回放与冲突判定。
// 重点：同键同指纹回同一终态；同键异指纹冲突；都不二次扣减。
// 风险点：回放路径若再调 Submit 会双花。

// 作者：Hux
// 日期：2021.06.14

package fundmesh

import "context"

  func (e *Engine) replayOrConflict(ctx context.Context, req OutboundRequest, rec *OutboundRecord, fp string, parts IdemKeyParts) (OutboundResult, error) {
	  if rec == nil {
		   return failOutbound(req, CodeOutboundDenied, ""), ErrOutboundDenied
	  }
	   if FingerprintDrift(rec.Fingerprint, fp) {
		   e.noteOutboundProof(ProofConflict, rec.WithdrawID, rec.IdempotencyKey, fp, string(rec.Phase), rec.Code, "fingerprint drift", 0, true)
		   res := failOutbound(req, CodeOutboundConflict, rec.Phase)
		  res.WithdrawID = rec.WithdrawID
		  res.IdempotencyKey = rec.IdempotencyKey
		   res.Fingerprint = rec.Fingerprint
		  res.DebitCount = rec.DebitCount
		   return res, ErrOutboundIdemConflict
	  }
	   if !OutboundDebitAllowed(rec.State, rec.Phase) {
		   return e.finishReplay(ctx, req, rec, fp, parts)
	  }
	  return e.finishReplay(ctx, req, rec, fp, parts)
   }

   func (e *Engine) finishReplay(ctx context.Context, req OutboundRequest, rec *OutboundRecord, fp string, parts IdemKeyParts) (OutboundResult, error) {
	   st := ReplayStateFor(rec)
	  code := ReplayCodeFor(rec)
	   e.noteOutboundProof(proofKindForReplay(st), rec.WithdrawID, rec.IdempotencyKey, fp, string(rec.Phase), rec.Code, "replay same terminal", 0, true)
	  req.WithdrawID = rec.WithdrawID
	   req.IdempotencyKey = rec.IdempotencyKey
	   res := OutboundResult{
		   WithdrawID: rec.WithdrawID,
		  IdempotencyKey: rec.IdempotencyKey,
		   Fingerprint: rec.Fingerprint,
		   State: st,
		   Phase: rec.Phase,
		   Code:       code,
		   Idempotent: true,
		  Debited: false,
		   DebitCount: rec.DebitCount,
		   Entry:      req.Entry,
		   At:         e.nowUTC(),
	   }
	  if rec.State == OutboundAccepted {
		   rec.State = OutboundReplayed
		  rec.Code = CodeOutboundReplay
		  _ = e.outbounds.Put(ctx, rec)
	  }
	  if st == OutboundClosed {
		   return res, nil
	  }
	  if st == OutboundDenied && rec.Code != CodeOutboundOK && rec.Code != CodeOutboundReplay && rec.Code != "" && rec.Code != CodeOutboundDenied {
		  return res, codeToOutboundErr(rec.Code)
	  }
	   if st == OutboundConflict {
		   return res, ErrOutboundIdemConflict
	  }
	  _ = parts
	   return res, nil
   }

   func proofKindForReplay(st OutboundState) string {
	  switch st {
	  case OutboundClosed:
		  return ProofClosed
	   case OutboundConflict:
		  return ProofConflict
	   default:
		   return ProofReplay
	   }
  }

   func codeToOutboundErr(code string) error {
	   switch code {
	  case CodeRemoteFail:
		  return ErrLedgerRemoteFailed
	   case CodeDrift:
		  return ErrCriticalWriteClosed
	  case CodeOutboundConflict:
		  return ErrOutboundIdemConflict
	  case CodeOutboundDupDebit:
		   return ErrDoubleDebitForbidden
	   case CodeInput:
		   return ErrOutboundPayload
	  default:
		   return ErrOutboundDenied
	   }
  }
