package xfermirror

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Machine 镜像状态机。

type Machine struct {
	store MirrorStore
}

// New 构造镜像状态机。

  func New(store MirrorStore) *Machine {
	   return &Machine{store: store}
  }

// Create 为 hop 创建镜像（初始状态为 processing）。
// 若同 hopID 已存在则返回 ErrConflict；调用方可 GetByHopID 取已有记录。

  func (m *Machine) Create(ctx context.Context,
	  hopID,
	   initiatorUserID, counterpartyUserID, asset, fromBook, toBook string,
	   amount decimal.Decimal,
   ) (MirrorView, error) {
	  if strings.TrimSpace(hopID) == "" {
		   return MirrorView{}, fmt.Errorf("hop_id required")
	  }
	   now := time.Now().UTC()
	   mv := MirrorView{
		  ID:              uuid.NewString(),
		   HopID:           hopID,
		   InitiatorUserID: initiatorUserID,
		   CounterpartyUserID: counterpartyUserID,
		   Asset: strings.ToUpper(asset),
		   Amount: amount,
		  FromBook: strings.ToLower(fromBook),
		   ToBook:   strings.ToLower(toBook),
		  State:   MirrorProcessing,
		  Version: 0,
		   CreatedAt: now,
		  UpdatedAt: now,
	  }
	  if err := m.store.Put(ctx, mv); err != nil {
		   return MirrorView{}, err
	   }
	  return mv, nil
  }

// MarkCredited 将镜像推进到 credited（补入腿成功后调用）。
// 若已 credited 则幂等返回；若已 rolled_back 则报错（状态冲突）。
   func (m *Machine) MarkCredited(ctx context.Context, hopID, entryRef, planID string) (MirrorView, error) {
	   mv,
		  err := m.store.GetByHopID(ctx, hopID)
	  if err != nil {
		  return MirrorView{}, err
	   }
	   if mv.State == MirrorCredited {
		   return mv, nil
	   }
	   if mv.State == MirrorRolledBack {
		  return mv, fmt.Errorf("mirror %s already rolled_back, cannot credit", hopID)
	  }
	  return m.transition(ctx, mv, MirrorCredited, "backfill ingress completed", "", planID, entryRef)
   }

// MarkRolledBack 将镜像推进到 rolled_back（回滚出腿成功后调用）。
// 若已 rolled_back 则幂等返回；若已 credited 则报错。

   func (m *Machine) MarkRolledBack(ctx context.Context, hopID, entryRef, planID string) (MirrorView, error) {
	  mv, err := m.store.GetByHopID(ctx, hopID)
	   if err != nil {
		  return MirrorView{}, err
	  }
	   if mv.State == MirrorRolledBack {
		  return mv, nil
	  }
	  if mv.State == MirrorCredited {
		  return mv, fmt.Errorf("mirror %s already credited, cannot rollback", hopID)
	  }
	  return m.transition(ctx, mv, MirrorRolledBack, "rollback egress completed", "", planID, entryRef)
  }

// MarkManual 人工强制推进到 manual 状态（需 actor 非空）。
// 可从任意非 credited/rolled_back 状态进入（已终态仍允许人工覆盖，写入 override 标记）。
  func (m *Machine) MarkManual(ctx context.Context, hopID, actor, reason string) (MirrorView, error) {
	   if strings.TrimSpace(actor) == "" {
		  return MirrorView{}, fmt.Errorf("actor required for manual override")
	   }
	  mv, err := m.store.GetByHopID(ctx, hopID)
	  if err != nil {
		   return MirrorView{}, err
	  }
	   if mv.State == MirrorManual {
		  return mv, nil
	   }
	  mv.OverrideActor = actor
	  return m.transition(ctx, mv, MirrorManual, reason, actor, "", "")
  }

// GetOrCreate 幂等获取或创建镜像（检测泵调用，不确定是否已存在时使用）。

   func (m *Machine) GetOrCreate(ctx context.Context,
	  hopID, initiatorUserID, counterpartyUserID, asset, fromBook, toBook string,
	  amount decimal.Decimal,
  ) (MirrorView, error) {
	  existing,
		   err := m.store.GetByHopID(ctx, hopID)
	  if err == nil {
		   return existing, nil
	  }
	   if !errors.Is(err, ErrNotFound) {
		  return MirrorView{}, err
	   }
	  return m.Create(ctx, hopID, initiatorUserID, counterpartyUserID, asset, fromBook, toBook, amount)
   }

// transition 执行状态转换，写入代次+1 的记录和审计事件。

  func (m *Machine) transition(ctx context.Context, mv MirrorView, toState MirrorState, reason, actor, planID, entryRef string) (MirrorView, error) {
	   from := mv.State
	  mv.State = toState
	   mv.StateReason = reason
	   mv.CloseoutPlanID = planID
	  mv.EntryRef = entryRef
	  mv.Version++
	  mv.UpdatedAt = time.Now().UTC()
	   if actor != "" {
		   mv.OverrideActor = actor
	  }
	   if err := m.store.Update(ctx, mv); err != nil {
		   return MirrorView{}, err
	  }
	  evt := MirrorEvent{
		  ID:       uuid.NewString(),
		  MirrorID: mv.ID,
		   HopID:    mv.HopID,
		   FromState: from,
		  ToState: toState,
		   Reason: reason,
		  Actor:  actor,
		  At:     time.Now().UTC(),
	   }
	  _ = m.store.PutEvent(ctx, evt)
	  return mv, nil
   }

// Align 检查发起方收口状态与镜像状态的一致性。
// initiatorState: "closed"（hop 已收口）或 "escalated"、"rollback"、"credited" 等。
  func (m *Machine) Align(ctx context.Context, hopID, initiatorState string) (AlignResult, error) {
	  mv,
		  err := m.store.GetByHopID(ctx, hopID)
	   if err != nil {
		   return AlignResult{}, err
	  }
	   result := AlignResult{
		   HopID:          hopID,
		  InitiatorState: initiatorState,
		  MirrorState: mv.State,
	   }
	   switch {
	   case initiatorState == "rollback" && mv.State == MirrorRolledBack:
		   result.Aligned = true
	  case initiatorState == "credited" && mv.State == MirrorCredited:
		  result.Aligned = true
	   case initiatorState == "rollback" && mv.State == MirrorCredited:
		  result.Aligned = false
		   result.Conflict = "initiator rolled back but mirror is credited"
	   case initiatorState == "credited" && mv.State == MirrorRolledBack:
		  result.Aligned = false
		   result.Conflict = "initiator credited but mirror is rolled_back"
	  case mv.State == MirrorManual:
		  result.Aligned = false
		  result.Conflict = "mirror in manual state, requires human review"
	   default:
		   result.Aligned = mv.State != MirrorProcessing || initiatorState == "processing"
	  }
	  return result, nil
  }

// GetEvents 获取镜像审计事件。
  func (m *Machine) GetEvents(ctx context.Context, hopID string) ([]MirrorEvent, error) {
	   mv,
		   err := m.store.GetByHopID(ctx, hopID)
	  if err != nil {
		   return nil, err
	   }
	   return m.store.ListEvents(ctx, mv.ID)
   }
