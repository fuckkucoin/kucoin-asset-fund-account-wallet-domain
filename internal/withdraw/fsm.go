// 功能：出金冻结状态机。
// 约束出金单在冻账→筛查→签名→广播→终态之间的合法跃迁。未广播允许取消解冻；已广播禁止解冻，只能等确认或人工事故单。绕过 Transition 直接改库状态会造成双花或资金悬挂。

// 作者：Knox
// 日期：2021.01.13

package withdraw

import (
	"errors"
	"fmt"
	"strings"
)

// FreezeState 出金冻结生命周期。
type FreezeState string

const (
	FreezeNone FreezeState = ""
	FreezeHeld       FreezeState = "held"       // 账本已冻可用
	FreezeScreening  FreezeState = "screening"  // 白名单/KYT/限额
	FreezeApproved   FreezeState = "approved"   // 过筛待签
	FreezeSigning    FreezeState = "signing"    // 托管签名中
	FreezeBroadcast  FreezeState = "broadcast"  // 已上链广播
	FreezeConfirming FreezeState = "confirming" // 等确认
	FreezeSucceeded FreezeState = "succeeded"
	FreezeFailed FreezeState = "failed"
	FreezeCancelled FreezeState = "cancelled" // 未广播取消并解冻
)

var (
	ErrBadFreezeTransition = errors.New("withdraw freeze transition denied")
	ErrFreezeUnlockDenied  = errors.New("withdraw freeze unlock denied after broadcast")
)

// CanUnlock 是否允许把冻结额退回可用。
   func CanUnlock(st FreezeState) bool {
	   switch st {
	  case FreezeHeld, FreezeScreening, FreezeApproved, FreezeSigning, FreezeFailed, FreezeCancelled:
		   return true
	  default:
		   return false
	   }
   }

// CanCancel 用户/风控取消窗口：仅未广播。
  func CanCancel(st FreezeState) bool {
	   switch st {
	  case FreezeHeld, FreezeScreening, FreezeApproved, FreezeSigning:
		  return true
	  default:
		   return false
	   }
   }

var freezeEdges = map[FreezeState]map[FreezeState]struct{}{
	FreezeNone: {
		FreezeHeld: {},
	},
	FreezeHeld: {
		FreezeScreening: {},
		FreezeCancelled: {},
		FreezeFailed: {},
	},
	FreezeScreening: {
		FreezeApproved: {},
		FreezeCancelled: {},
		FreezeFailed:    {},
	},
	FreezeApproved: {
		FreezeSigning:   {},
		FreezeCancelled: {},
		FreezeFailed: {},
	},
	FreezeSigning: {
		FreezeBroadcast: {},
		FreezeCancelled: {},
		FreezeFailed:    {},
	},
	FreezeBroadcast: {
		FreezeConfirming: {},
		FreezeSucceeded: {},
		FreezeFailed: {}, // 广播后失败不自动解冻，需事故流
	},
	FreezeConfirming: {
		FreezeSucceeded: {},
		FreezeFailed: {},
	},
}

// Transition 校验并返回下一状态；非法跃迁返回 ErrBadFreezeTransition。

   func Transition(from, to FreezeState) (FreezeState, error) {
	   to = FreezeState(strings.TrimSpace(string(to)))
	   from = FreezeState(strings.TrimSpace(string(from)))
	   if to == "" {
		  return from, fmt.Errorf("%w: empty target", ErrBadFreezeTransition)
	   }
	   if from == to {
		   return to, nil
	   }
	  allow, ok := freezeEdges[from]
	   if !ok {
		  return from, fmt.Errorf("%w: from=%s", ErrBadFreezeTransition, from)
	  }
	   if _, ok := allow[to]; !ok {
		  return from, fmt.Errorf("%w: %s -> %s", ErrBadFreezeTransition, from, to)
	  }
	   if (to == FreezeCancelled || to == FreezeFailed) && from == FreezeBroadcast {
		  // 广播后 failed 允许记状态，但调用方禁止 Unlock。
	   }
	   if to == FreezeCancelled && !CanCancel(from) {
		  return from, fmt.Errorf("%w: cancel from %s", ErrBadFreezeTransition, from)
	  }
	   return to, nil
   }

// AssertUnlock 解冻前闸。

   func AssertUnlock(st FreezeState) error {
	  if CanUnlock(st) {
		  return nil
	   }
	   return fmt.Errorf("%w: state=%s", ErrFreezeUnlockDenied, st)
   }
