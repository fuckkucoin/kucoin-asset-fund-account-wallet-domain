// @FileName: service.go
// @Summary: 回收世代闸服务。
// MarkWithEpoch / ReadyAssert / AssignUser / ObserveCreditAssertEpoch
// 推进 recycle_epoch；迟观察旧用户 fail-closed 并开 MiscreditProof。
// 不重写 dustleak / attribrt / claimmutex FSM。
// @Author: Kian
// @Created: 2021.10.19

package recycleepoch

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

// Service 回收 epoch 错户封堵闸。
type Service struct {
	store Store
	clock func() time.Time
	recycle RecycleBookReader
	inv InventoryReader
	dep DepositObserveReader
	disp MiscreditDisposition
}

  func New() *Service {
	   return NewWithStore(NewMemStore())
   }

  func NewWithStore(store Store) *Service {
	  return &Service{
		   store: store,
		   clock: func() time.Time { return time.Now().UTC() },
		   recycle: NopRecycleBook{},
		  inv:     NopInventory{},
		   dep:     NopDepositObserve{},
		   disp: DispReject,
	   }
   }

   func (s *Service) WithClock(c func() time.Time) *Service {
	  if c != nil {
		  s.clock = c
	  }
	  return s
   }

  func (s *Service) WithHooks(r RecycleBookReader, inv InventoryReader, d DepositObserveReader) *Service {
	   if r != nil {
		   s.recycle = r
	   }
	  if inv != nil {
		  s.inv = inv
	  }
	   if d != nil {
		  s.dep = d
	  }
	   return s
   }

  func (s *Service) WithDisposition(d MiscreditDisposition) *Service {
	  if d == DispQuarantine || d == DispReject {
		  s.disp = d
	   }
	   return s
   }

  func (s *Service) now() time.Time { return s.clock().UTC() }

// MarkWithEpoch 进入冷却并建立/推进 recycle_epoch。
  func (s *Service) MarkWithEpoch(ctx context.Context, req MarkRequest) (*RecycleEpoch, error) {
	   slot, prev, err := normalizeSlotUser(req.Chain, req.Address, req.PrevUser)
	  if err != nil {
		   return nil, err
	  }
	   actor := strings.TrimSpace(req.ActorID)
	   if actor == "" {
		  return nil, ErrActorEmpty
	  }
	  now := s.now()
	  cool := req.CoolUntil.UTC()
	   if cool.IsZero() || !cool.After(now) {
		  cool = now.Add(24 * time.Hour)
	   }
	   cur, err := s.store.GetEpoch(ctx, slot)
	  if err != nil && !errors.Is(err, ErrEpochNotFound) {
		  return nil, err
	   }
	   if cur != nil && req.IdemKey != "" && cur.IdemKey == req.IdemKey && cur.State == EpochCooling {
		   return cur, nil
	  }
	   nextState,
		  gen, err := NextOnMark(cur)
	   if err != nil {
		   return nil, err
	  }
	   if cur == nil {
		  e := RecycleEpoch{
			   EpochID: uuid.NewString(),
			  Slot:    slot,
			  Generation: gen,
			   State:      nextState,
			   PrevUser:   prev,
			  CoolUntil:  cool,
			  MarkedAt:   now,
			   ActorID: actor,
			  IdemKey:    strings.TrimSpace(req.IdemKey),
			  RecycleRef: strings.TrimSpace(req.RecycleRef),
			   CreatedAt: now,
			   UpdatedAt: now,
		   }
		  if err := s.store.InsertEpoch(ctx, e); err != nil {
			   return nil, err
		   }
		  _ = s.store.RevokeBind(ctx, slot, "mark_cooling", now)
		   s.audit(ctx, slot, "mark", actor, fmt.Sprintf("gen=%d cooling", gen), now)
		  return &e, nil
	   }
	  next := *cur
	  next.Generation = gen
	  next.State = nextState
	   next.PrevUser = prev
	   if cur.State == EpochAssigned && strings.TrimSpace(cur.NextUser) != "" {
		  next.PrevUser = cur.NextUser
	  }
	   next.NextUser = ""
	  next.CoolUntil = cool
	  next.MarkedAt = now
	  next.ReadyAt = nil
	  next.AssignedAt = nil
	   next.ActorID = actor
	   next.IdemKey = strings.TrimSpace(req.IdemKey)
	   next.UpdatedAt = now
	  if err := s.store.CASUpdateEpoch(ctx, cur.EpochID, cur.Generation, cur.State, next); err != nil {
		  return nil, err
	   }
	   _ = s.store.RevokeBind(ctx, slot, "mark_cooling", now)
	  s.audit(ctx, slot, "mark", actor, fmt.Sprintf("gen=%d cooling prev=%s", gen, next.PrevUser), now)
	  return &next, nil
  }

// ReadyAssert 冷却期满推进 ready。未满 fail-closed。Force 开证明且不得进入成功复用。

  func (s *Service) ReadyAssert(ctx context.Context, req ReadyRequest) (*RecycleEpoch, error) {
	  slot,
		   actor, err := normalizeSlotActor(req.Chain, req.Address, req.ActorID)
	  if err != nil {
		  return nil, err
	  }
	   now := s.now()
	   cur, err := s.store.GetEpoch(ctx, slot)
	   if err != nil {
		   if errors.Is(err, ErrEpochNotFound) {
			  if req.Force {
				  _ = s.openProof(ctx, nil, slot, 0, 0, ProofMissingEpoch, "", "", "", "", "", "", actor, now)
			  }
			  return nil, ErrEpochMissing
		  }
		  return nil, err
	   }
	  if cur.State == EpochReady {
		  return cur, nil
	   }
	  if err := CheckReady(cur, now, req.Force); err != nil {
		  kind := ProofReadyBeforeCool
		  if errors.Is(err, ErrForcedReuse) {
			   kind = ProofForcedReuse
		   }
		  _ = s.openProof(ctx, cur, slot, cur.Generation, cur.Generation, kind, cur.PrevUser, "", cur.NextUser, "", "", "", actor, now)
		   return nil, err
	  }
	   next := *cur
	  next.State = EpochReady
	  t := now
	   next.ReadyAt = &t
	  next.ActorID = actor
	   next.UpdatedAt = now
	  if err := s.store.CASUpdateEpoch(ctx, cur.EpochID, cur.Generation, cur.State, next); err != nil {
		   return nil, err
	  }
	   s.audit(ctx, slot, "ready", actor, fmt.Sprintf("gen=%d", next.Generation), now)
	  return &next, nil
   }

// AssignUser Ready 后再分配给新用户；旧绑定立即失效。

  func (s *Service) AssignUser(ctx context.Context, req AssignRequest) (*RecycleEpoch, *ObserveBind, error) {
	  slot, nextUser, err := normalizeSlotUser(req.Chain, req.Address, req.NextUser)
	  if err != nil {
		  return nil, nil, err
	   }
	  actor := strings.TrimSpace(req.ActorID)
	  if actor == "" {
		   return nil, nil, ErrActorEmpty
	   }
	  now := s.now()
	   cur,
		   err := s.store.GetEpoch(ctx, slot)
	   if err != nil {
		  if errors.Is(err, ErrEpochNotFound) {
			  return nil, nil, ErrEpochMissing
		  }
		  return nil, nil, err
	   }
	  if cur.State == EpochAssigned && cur.NextUser == nextUser {
		   b, _ := s.store.GetBind(ctx, slot)
		   return cur, b, nil
	   }
	  if !cur.CanAssign() {
		  if cur.State == EpochCooling && !cur.CoolElapsed(now) {
			   _ = s.openProof(ctx, cur, slot, cur.Generation, cur.Generation, ProofForcedReuse, cur.PrevUser, nextUser, "", "", "", "", actor, now)
			   return nil, nil, ErrForcedReuse
		  }
		  return nil, nil, ErrNotReady
	   }
	   next := *cur
	   next.State = EpochAssigned
	   next.NextUser = nextUser
	   t := now
	  next.AssignedAt = &t
	  next.ActorID = actor
	   next.UpdatedAt = now
	   if err := s.store.CASUpdateEpoch(ctx, cur.EpochID, cur.Generation, cur.State, next); err != nil {
		   return nil, nil, err
	  }
	  _ = s.store.RevokeBind(ctx, slot, "reassign", now)
	   bind := ObserveBind{
		  BindID: uuid.NewString(),
		   Slot:       slot,
		  EpochID:    next.EpochID,
		   Generation: next.Generation,
		  UserID:  nextUser,
		   State:   BindActive,
		   BoundAt: now,
	  }
	  if err := s.store.UpsertBind(ctx, bind); err != nil {
		   return nil, nil, err
	   }
	  s.audit(ctx, slot, "assign", actor, fmt.Sprintf("gen=%d next=%s", next.Generation, nextUser), now)
	  return &next, &bind, nil
  }

// Retire 槽位退役，吊销绑定。

   func (s *Service) Retire(ctx context.Context, req RetireRequest) (*RecycleEpoch, error) {
	  slot, actor, err := normalizeSlotActor(req.Chain, req.Address, req.ActorID)
	  if err != nil {
		  return nil, err
	  }
	  now := s.now()
	   cur, err := s.store.GetEpoch(ctx, slot)
	   if err != nil {
		   return nil, err
	   }
	   if cur.State == EpochRetired {
		  return cur, nil
	  }
	   next := *cur
	  next.State = EpochRetired
	  next.ActorID = actor
	  next.UpdatedAt = now
	  if err := s.store.CASUpdateEpoch(ctx, cur.EpochID, cur.Generation, cur.State, next); err != nil {
		   return nil, err
	   }
	   _ = s.store.RevokeBind(ctx, slot, "retired", now)
	  s.audit(ctx, slot, "retire", actor, fmt.Sprintf("gen=%d", next.Generation), now)
	   return &next, nil
  }

  func (s *Service) GetEpoch(ctx context.Context, chain, address string) (*RecycleEpoch, error) {
	   ch := strings.ToUpper(strings.TrimSpace(chain))
	  addr := strings.ToLower(strings.TrimSpace(address))
	  if ch == "" || addr == "" {
		   return nil, ErrAddressEmpty
	  }
	  return s.store.GetEpoch(ctx, NormalizeSlot(ch, addr))
  }

   func (s *Service) CurrentGeneration(ctx context.Context, chain, address string) (int64, error) {
	   e, err := s.GetEpoch(ctx, chain, address)
	  if errors.Is(err, ErrEpochNotFound) {
		   return 0, nil
	   }
	   if err != nil {
		   return 0, err
	  }
	   return e.Generation, nil
   }

  func (s *Service) audit(ctx context.Context, slot SlotKey, kind, actor, detail string, now time.Time) {
	   _ = s.store.InsertAudit(ctx, AuditEvent{
		  EventID: uuid.NewString(),
		  Slot:    slot,
		   Kind: kind,
		   Actor: actor,
		   Detail: detail,
		   OccurredAt: now,
	  })
  }

   func (s *Service) View(ctx context.Context, chain, address string) (*RecycleView, error) {
	  slot := NormalizeSlot(chain, address)
	  if slot.Empty() {
		   return nil, ErrAddressEmpty
	   }
	  v := &RecycleView{Slot: slot}
	  e, err := s.store.GetEpoch(ctx, slot)
	   if err != nil && !errors.Is(err, ErrEpochNotFound) {
		   return nil, err
	  }
	   v.Epoch = e
	  b, err := s.store.GetBind(ctx, slot)
	   if err != nil && !errors.Is(err, ErrBindNotFound) {
		   return nil, err
	  }
	   v.Bind = b
	  if rec, rerr := s.recycle.Peek(ctx, address); rerr == nil && rec.Address != "" {
		  cp := rec
		  v.RecycleRec = &cp
	   }
	  if h, ok := s.inv.Hint(chain, address); ok {
		   cp := h
		  v.Inventory = &cp
	  }
	  v.OpenProofs, _ = s.store.ListProofsBySlot(ctx, slot)
	  obs,
		  _ := s.store.ListObservesBySlot(ctx, slot, 1)
	  if len(obs) > 0 {
		   v.LastObserve = &obs[0]
	  }
	   return v, nil
   }
