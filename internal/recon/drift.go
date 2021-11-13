package recon

import (
	"context"
	"fmt"
	"account-assets.kucoin.net/funding/internal/ledger"
)

type Freezer interface {
	SetFrozen(ctx context.Context, userID string, frozen bool) error
	NoteReconcile(ctx context.Context, rep *ledger.ReconcileReport)
	CheckDrift(userID string) error
}

// HaltEngager 漂移冻账时可加严全局充提熔断（Engage）；不得在此 Clear。
type HaltEngager interface {
	Engage(ctx context.Context,
		actor, reason string) error
}

type DriftAction struct {
	UserID string `json:"user_id"`
	Frozen bool   `json:"frozen"`
	Blocked bool `json:"blocked"`
	Engaged bool `json:"engaged,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// DriftHooks 漂移冻账附加闸：工单簿（FreezeGate）与充提熔断 Engage。

type DriftHooks struct {
	WF *Workflow
	Halt   HaltEngager
	Freeze *DriftFreezeStore
}

// ApplyFreezeOnDrift 建议含 freeze_user 时冻账户并记漂移。
// 对账 OK 不自动解冻——解冻必须人手，避免漂一下又自动放行。

  func ApplyFreezeOnDrift(ctx context.Context, fz Freezer, userID string, sug []Suggestion) (DriftAction, error) {
	  return ApplyFreezeOnDriftWith(ctx, fz, userID, sug, DriftHooks{})
  }

// ApplyFreezeOnDriftWith 漂移冻账并挂接 Engage / 工单阻断。
// 冻用户 → NoteReconcile 挡新动作 → 开差异单供 FreezeGate → 可选 Engage 熔断。
// 不得在本路径 Clear 熔断；解冻须人工且先关差异。
  func ApplyFreezeOnDriftWith(ctx context.Context, fz Freezer, userID string, sug []Suggestion, hooks DriftHooks) (DriftAction, error) {
	  if fz == nil {
		   return DriftAction{}, fmt.Errorf("reconcile freezer unavailable")
	   }
	   act := DriftAction{UserID: userID}
	  need := false
	  for _, s := range sug {
		   if s.Action == ActFreezeUser {
			  need = true
			   act.Reason = s.Note
		  }
	   }
	   if !need {
		  return act, nil
	  }
	  if err := fz.SetFrozen(ctx, userID, true); err != nil {
		   return act, err
	  }
	   act.Frozen = true
	   fz.NoteReconcile(ctx, &ledger.ReconcileReport{OK: false, UserID: userID, Blocked: true})
	  if err := fz.CheckDrift(userID); err != nil {
		   act.Blocked = true
		  if act.Reason == "" {
			   act.Reason = err.Error()
		   }
	   }
	   reason := act.Reason
	   if reason == "" {
		   reason = "reconcile drift freeze"
	   }
	  if hooks.WF != nil {
		  _, _ = hooks.WF.Ingest("recon-drift", []Mismatch{{
			   UserID: userID, Book: "control", Asset: "*", Note: reason,
		   }})
	  }
	  if hooks.Freeze != nil && hooks.WF != nil {
		  _, _ = hooks.WF.SyncDriftFreeze(ctx, hooks.Freeze, userID)
	   }
	  if hooks.Halt != nil {
		  if err := hooks.Halt.Engage(ctx, "recon-drift", reason); err != nil {
			  return act, err
		  }
		   act.Engaged = true
	   }
	   return act, nil
   }

// DriftBlocksFreeze 漂移已冻账户时，后续冻账须经 FreezeGate；未关差异不得改冻结位。
   func DriftBlocksFreeze(w *Workflow, userID string) error {
	   if w == nil {
		  return nil
	  }
	  return w.FreezeGate(userID)
   }

// GuardFreezeChange 改冻结位前检查未关差异。
// 挂到 ops/ledger SetFrozen 入口。
// 绕过本闸会在漂移未解时解冻或叠冻。

   func GuardFreezeChange(w *Workflow, userID string) error {
	   return DriftBlocksFreeze(w, userID)
   }
