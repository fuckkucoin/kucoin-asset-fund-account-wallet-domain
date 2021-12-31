package xferclose

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

const (
	actorSystem = "system"
	maxAttempts = 5
)

// PlanManager 负责 CloseoutPlan 的生命周期管理。

type PlanManager struct {
	store Store
}

// NewPlanManager 构造计划管理器。
  func NewPlanManager(store Store) *PlanManager {
	   return &PlanManager{store: store}
  }

// OpenPlan 为指定 HalfHop 开启收口计划。
// 若已有 open/running 计划则返回 ErrPlanRunning；已有 done 计划则返回 ErrPlanDone（幂等保护）。

  func (m *PlanManager) OpenPlan(ctx context.Context, hop HalfHop, action CloseoutAction, actor string) (CloseoutPlan, error) {
	  if strings.TrimSpace(hop.HopID) == "" {
		   return CloseoutPlan{}, ErrBadHopID
	   }
	   if actor == "" {
		   actor = actorSystem
	   }
	   existing, found, err := m.store.GetPlanByHopID(ctx, hop.HopID)
	   if err != nil {
		  return CloseoutPlan{}, err
	   }
	   if found {
		  switch existing.State {
		  case PlanOpen, PlanRunning:
			  return existing, ErrPlanRunning
		  case PlanDone:
			   return existing, ErrPlanDone
		  }
	  }
	  now := time.Now().UTC()
	   plan := CloseoutPlan{
		   ID:        uuid.NewString(),
		   HopID:     hop.HopID,
		   HalfHopID: hop.ID,
		   State:     PlanOpen,
		   SelectedAction: action,
		  Reason:    reasonForAction(action, hop),
		  Actor:     actor,
		   CreatedAt: now,
		   UpdatedAt: now,
	   }
	   if err2 := m.store.PutPlan(ctx, plan); err2 != nil {
		   return CloseoutPlan{}, err2
	  }
	  return plan, nil
   }

// StartPlan open→running。幂等：已经 running 直接返回。

  func (m *PlanManager) StartPlan(ctx context.Context, planID string) (CloseoutPlan, error) {
	  plan, err := m.store.GetPlan(ctx, planID)
	  if err != nil {
		  return CloseoutPlan{}, err
	  }
	  if plan.State == PlanRunning {
		   return plan, nil
	   }
	   if plan.State != PlanOpen {
		   return plan, fmt.Errorf("plan %s state=%s: cannot start", planID, plan.State)
	  }
	  plan.State = PlanRunning
	   plan.UpdatedAt = time.Now().UTC()
	   if err2 := m.store.UpdatePlan(ctx, plan); err2 != nil {
		   return plan, err2
	   }
	   return plan, nil
   }

// MarkDone running→done。记录完成时间。

   func (m *PlanManager) MarkDone(ctx context.Context, planID string) (CloseoutPlan, error) {
	   plan, err := m.store.GetPlan(ctx, planID)
	  if err != nil {
		  return CloseoutPlan{}, err
	  }
	   if plan.State == PlanDone {
		   return plan, nil
	   }
	   if plan.State != PlanRunning {
		  return plan, fmt.Errorf("plan %s state=%s: cannot mark done", planID, plan.State)
	   }
	   now := time.Now().UTC()
	   plan.State = PlanDone
	  plan.DoneAt = &now
	   plan.UpdatedAt = now
	  if err2 := m.store.UpdatePlan(ctx, plan); err2 != nil {
		   return plan, err2
	  }
	  return plan, nil
   }

// AbortPlan running/open→aborted。记录中止原因。

  func (m *PlanManager) AbortPlan(ctx context.Context, planID, reason string) (CloseoutPlan, error) {
	   plan, err := m.store.GetPlan(ctx, planID)
	  if err != nil {
		  return CloseoutPlan{}, err
	   }
	  if plan.State == PlanAborted {
		  return plan, nil
	   }
	   if plan.State == PlanDone {
		  return plan, fmt.Errorf("plan %s already done, cannot abort", planID)
	  }
	  plan.State = PlanAborted
	  plan.AbortReason = reason
	   plan.UpdatedAt = time.Now().UTC()
	  if err2 := m.store.UpdatePlan(ctx, plan); err2 != nil {
		   return plan, err2
	   }
	   return plan, nil
  }

// RecordAttempt 记录一次执行尝试（成功或失败）。
  func (m *PlanManager) RecordAttempt(ctx context.Context, planID string, lastErr error) error {
	  plan, err := m.store.GetPlan(ctx, planID)
	   if err != nil {
		  return err
	  }
	  plan.Attempts++
	  if lastErr != nil {
		  plan.LastError = lastErr.Error()
	  } else {
		  plan.LastError = ""
	   }
	   plan.UpdatedAt = time.Now().UTC()
	  return m.store.UpdatePlan(ctx, plan)
  }

// ExhaustedAttempts 判断计划是否已超出最大重试次数。

  func (m *PlanManager) ExhaustedAttempts(ctx context.Context, planID string) (bool, error) {
	  plan,
		  err := m.store.GetPlan(ctx, planID)
	  if err != nil {
		  return false, err
	  }
	  return plan.Attempts >= maxAttempts, nil
  }

// BuildLeg 为计划创建一条新的收口腿。
// 若同 idemKey 的腿已存在则返回 ErrIdempotent；调用方可忽略此错误读取已有腿。
  func (m *PlanManager) BuildLeg(ctx context.Context, plan CloseoutPlan, seqNo int, action CloseoutAction, idemKey string) (CloseoutLeg, error) {
	  existing, err := m.store.GetLegByIdem(ctx, idemKey)
	   if err == nil {
		  return existing, ErrIdempotent
	   }
	  now := time.Now().UTC()
	  leg := CloseoutLeg{
		  ID:     uuid.NewString(),
		   PlanID: plan.ID,
		   HopID:  plan.HopID,
		  SeqNo:  seqNo,
		   Action: action,
		  State:  LegPending,
		  IdemKey:   idemKey,
		   CreatedAt: now,
	   }
	   if err2 := m.store.PutLeg(ctx, leg); err2 != nil {
		   return CloseoutLeg{}, err2
	  }
	   return leg, nil
   }

// UpdateLegState 更新腿的状态，记录结果引用和说明。
   func (m *PlanManager) UpdateLegState(ctx context.Context, legID string, state LegState, entryRef, note string) error {
	  leg, err := m.store.GetLeg(ctx, legID)
	  if err != nil {
		  return err
	  }
	  leg.State = state
	  leg.EntryRef = entryRef
	   leg.Note = note
	  now := time.Now().UTC()
	  switch state {
	  case LegRunning:
		   leg.StartedAt = &now
	  case LegDone, LegFailed, LegSkipped:
		   leg.FinishedAt = &now
	  }
	  return m.store.UpdateLeg(ctx,
		  leg)
  }

// IncrLegAttempts 增加腿的重试计数。
   func (m *PlanManager) IncrLegAttempts(ctx context.Context, legID string) error {
	  leg, err := m.store.GetLeg(ctx, legID)
	  if err != nil {
		  return err
	   }
	  leg.Attempts++
	   return m.store.UpdateLeg(ctx,
		  leg)
  }

// AllLegsTerminal 判断计划的所有腿是否都已终态。
  func (m *PlanManager) AllLegsTerminal(ctx context.Context, planID string) (bool, error) {
	  legs, err := m.store.ListLegs(ctx, planID)
	   if err != nil {
		   return false, err
	   }
	   if len(legs) == 0 {
		   return false, nil
	   }
	  for _, l := range legs {
		  if l.State == LegPending || l.State == LegRunning {
			  return false, nil
		   }
	   }
	  return true, nil
  }

// AnyLegFailed 判断是否有失败腿（用于中止计划）。

  func (m *PlanManager) AnyLegFailed(ctx context.Context, planID string) (bool, error) {
	   legs, err := m.store.ListLegs(ctx, planID)
	   if err != nil {
		  return false, err
	  }
	  for _, l := range legs {
		  if l.State == LegFailed {
			   return true, nil
		   }
	  }
	   return false, nil
   }

   func reasonForAction(action CloseoutAction, hop HalfHop) string {
	  switch action {
	   case ActionRetryAdvance:
		  return fmt.Sprintf("hop %s: egress done but ingress not reached, retry advance", hop.HopID)
	  case ActionRollbackEgress:
		   return fmt.Sprintf("hop %s: sla breached, rollback egress to unfreeze initiator", hop.HopID)
	  case ActionBackfillIngress:
		  return fmt.Sprintf("hop %s: counterparty confirmed not credited, backfill ingress", hop.HopID)
	   case ActionEscalateDesk:
		   return fmt.Sprintf("hop %s: auto closeout not applicable, escalate to desk", hop.HopID)
	   case ActionOpenIncident:
		   return fmt.Sprintf("hop %s: repeated failures, open incident", hop.HopID)
	  default:
		  return fmt.Sprintf("hop %s: closeout action %s", hop.HopID, action)
	  }
   }
