package pumpmarkfence

import (
	"strings"
	"time"
)

// FailClosedMark 判定是否允许把事件 Mark 为已投递成功业务效果。
   func FailClosedMark(gen *DeliverEffectGen, now time.Time, payloadHash string, fromLag bool) error {
	   if fromLag {
		   return ErrLagReadonly
	   }
	   if gen == nil {
		   return ErrGenMissing
	  }
	  if strings.TrimSpace(gen.GenID) == "" {
		  return ErrGenMissing
	  }
	   if gen.Generation <= 0 {
		   return ErrGenStale
	  }
	  if strings.TrimSpace(gen.PayloadHash) == "" {
		  return ErrFingerprint
	  }
	   if payloadHash != "" && !FingerprintMatch(gen.PayloadHash, payloadHash) {
		   return ErrFingerprint
	  }
	   if gen.State == GenExpired || gen.Expired(now) {
		  return ErrGenExpired
	  }
	   if gen.State == GenMarked || gen.Applied {
		  return ErrAlreadyApplied
	   }
	  if gen.State != GenDelivered && gen.State != GenIssued {
		   return ErrMarkDenied
	   }
	  return nil
  }

// FailClosedReplay 毒丸重投闸。

   func FailClosedReplay(gen *DeliverEffectGen, expectGen int64, now time.Time, payloadHash string) error {
	  if gen == nil {
		  return ErrGenMissing
	   }
	  if err := FailClosedMark(gen, now, payloadHash, false); err != nil && err != ErrMarkDenied {
		   if err == ErrAlreadyApplied {
			   return ErrPoisonReplay
		  }
		   return err
	  }
	   if expectGen != 0 && expectGen != gen.Generation {
		   return ErrCASFailed
	   }
	  if gen.Applied || gen.State == GenMarked {
		  return ErrPoisonReplay
	  }
	  return nil
   }

// ClassifyFail 把错误归到证明类型。

  func ClassifyFail(err error) ProofKind {
	  if err == nil {
		   return ""
	   }
	  switch {
	   case err == ErrGenMissing:
		  return ProofMarkNoGen
	  case err == ErrGenExpired:
		   return ProofExpired
	   case err == ErrFingerprint:
		   return ProofFingerprint
	  case err == ErrLagReadonly:
		   return ProofLagMark
	  case err == ErrPoisonReplay, err == ErrAlreadyApplied, err == ErrDupEffect:
		   return ProofPoisonReplay
	  default:
		  return ProofTickReplay
	  }
  }

   func isDupErr(err error) bool {
	   return err == ErrAlreadyApplied || err == ErrDupEffect || err == ErrPoisonReplay || err == ErrHalfMarkClosed
  }
