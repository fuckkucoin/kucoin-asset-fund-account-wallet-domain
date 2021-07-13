package haltepochgate

import "fmt"

// NextOnEngage 计算 Engage 后的状态与世代。
   func NextOnEngage(cur *HaltEpoch) (EpochState, int64, error) {
	  if cur == nil {
		   return EpochEngaged, 1, nil
	  }
	   switch cur.State {
	   case EpochIdle:
		   return EpochEngaged, cur.Generation + 1, nil
	  case EpochCleared:
		  return EpochReengaged, cur.Generation + 1, nil
	   case EpochEngaged,
		  EpochReengaged:
		  return cur.State, cur.Generation, nil
	  case EpochClearing:
		  return "", 0, ErrClearInProgress
	  default:
		  return "", 0, fmt.Errorf("%w: state=%s", ErrScopeInvalid, cur.State)
	   }
   }

// NextOnClear 计算 Clear 进入 clearing 的世代（抬升）。

   func NextOnClear(cur *HaltEpoch) (EpochState, int64, error) {
	  if cur == nil {
		   return "", 0, ErrEpochNotFound
	   }
	  switch cur.State {
	  case EpochEngaged, EpochReengaged:
		  return EpochClearing,
			  cur.Generation + 1, nil
	  case EpochClearing:
		   return EpochClearing, cur.Generation, nil
	   case EpochCleared:
		  return EpochCleared, cur.Generation, nil
	   case EpochIdle:
		  return "", 0, fmt.Errorf("%w: already idle", ErrScopeInvalid)
	  default:
		  return "", 0, fmt.Errorf("%w: state=%s", ErrScopeInvalid, cur.State)
	  }
   }

// NextOnSeal clearing → cleared。世代不变。
  func NextOnSeal(cur *HaltEpoch) (EpochState, error) {
	  if cur == nil {
		   return "", ErrEpochNotFound
	   }
	   if cur.State != EpochClearing && cur.State != EpochCleared {
		  return "", fmt.Errorf("%w: state=%s", ErrWindowOpen, cur.State)
	   }
	   return EpochCleared, nil
  }

// NextOnResume cleared → idle，LaneOpen=true。世代不变。

   func NextOnResume(cur *HaltEpoch) (EpochState, error) {
	   if cur == nil {
		  return "", ErrEpochNotFound
	  }
	   if cur.State != EpochCleared {
		  return "", fmt.Errorf("%w: state=%s", ErrWindowOpen, cur.State)
	   }
	  return EpochIdle, nil
  }

// CanAdvance 世代单调：next 必须 > current，幂等重复除外。
   func CanAdvance(current, next int64, idempotent bool) error {
	  if next < current {
		  return ErrEpochRegress
	  }
	  if next == current && !idempotent {
		  return ErrEpochRegress
	  }
	   return nil
   }

// TransitionSlip open → proven → closed。

  func TransitionSlip(cur SlipState, next SlipState) error {
	  if !cur.Valid() || !next.Valid() {
		  return ErrProofNotFound
	   }
	  if cur == next {
		   return nil
	   }
	  switch {
	  case cur == SlipOpen && (next == SlipProven || next == SlipClosed):
		  return nil
	  case cur == SlipProven && next == SlipClosed:
		  return nil
	  default:
		  return fmt.Errorf("haltepochgate: slip %s → %s forbidden", cur, next)
	  }
  }

// TransitionIntent 在途意图转移。allowed 可撤销或拒绝；撤销后不得广播/贷记。
   func TransitionIntent(cur IntentState, next IntentState) error {
	  if cur == next {
		  return nil
	   }
	   switch cur {
	  case IntentAllowed:
		  switch next {
		   case IntentRevoked, IntentRejected, IntentBroadcast, IntentCredited:
			   return nil
		  }
	  case IntentBroadcast:
		  if next == IntentCredited || next == IntentRevoked {
			  return nil
		   }
	   }
	  return fmt.Errorf("haltepochgate: intent %s → %s forbidden", cur, next)
   }

// MutexKindOf 互斥占用类别。
  func MutexKindOf(engage bool) string {
	  if engage {
		   return "engage"
	  }
	  return "clear"
  }

// CompatibleMutex 同 actor 同 kind 可重入（幂等）；异 kind 互斥。
   func CompatibleMutex(holder, kind, wantHolder, wantKind string) error {
	  if holder == "" {
		   return nil
	  }
	  if kind != wantKind {
		  if wantKind == "engage" {
			  return ErrClearInProgress
		   }
		  return ErrEngageInProgress
	   }
	  if holder != wantHolder {
		   return ErrRaceMutex
	  }
	   return nil
  }
