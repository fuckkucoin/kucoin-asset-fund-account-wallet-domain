package georebal

import (
	"context"
	"sync"
	"time"
)

// Book 进程内计划 / 腿 / 区域库存簿。

type Book struct {
	mu      sync.RWMutex
	plans   map[string]Plan
	legs    map[string]Leg
	byPlan  map[string][]string
	regions map[string]Region
	intents map[string]CollectIntent
	moves []InventoryMove
	moveByID map[string]int
	idemLeg  map[string]string
}

// NewBook 空簿。
   func NewBook() *Book {
	  return &Book{
		   plans:   make(map[string]Plan),
		  legs:    make(map[string]Leg),
		   byPlan:  make(map[string][]string),
		  regions: make(map[string]Region),
		   intents: make(map[string]CollectIntent),
		   moveByID: make(map[string]int),
		  idemLeg:  make(map[string]string),
	   }
  }

  func (b *Book) PutRegion(r Region) {
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  if r.Balances == nil {
		  r.Balances = map[string]int64{}
	   }
	  r.UpdatedAt = time.Now().UTC()
	   b.regions[r.RegionID] = r
  }

  func (b *Book) GetRegion(_ context.Context, id string) (Region, error) {
	   b.mu.RLock()
	  defer b.mu.RUnlock()
	   r, ok := b.regions[id]
	  if !ok {
		   return Region{}, ErrRegionNotFound
	   }
	   return cloneRegion(r), nil
   }

   func (b *Book) HaltRegion(id, _ string) error {
	  b.mu.Lock()
	   defer b.mu.Unlock()
	   r, ok := b.regions[id]
	   if !ok {
		  return ErrRegionNotFound
	  }
	  r.Status = RegionHalted
	  r.HaltGen++
	  r.UpdatedAt = time.Now().UTC()
	  b.regions[id] = r
	  return nil
  }

   func (b *Book) ResumeRegion(id string) error {
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   r, ok := b.regions[id]
	   if !ok {
		  return ErrRegionNotFound
	  }
	   r.Status = RegionLive
	   r.UpdatedAt = time.Now().UTC()
	   b.regions[id] = r
	   return nil
   }

   func (b *Book) PutPlan(p Plan) {
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   now := time.Now().UTC()
	   if p.CreatedAt.IsZero() {
		   p.CreatedAt = now
	   }
	   p.UpdatedAt = now
	   b.plans[p.PlanID] = p
  }

  func (b *Book) GetPlan(_ context.Context, id string) (Plan, error) {
	  b.mu.RLock()
	  defer b.mu.RUnlock()
	   p, ok := b.plans[id]
	  if !ok {
		  return Plan{}, ErrPlanNotFound
	  }
	  return p, nil
   }

  func (b *Book) PutLeg(l Leg) {
	   b.mu.Lock()
	  defer b.mu.Unlock()
	   now := time.Now().UTC()
	   if l.CreatedAt.IsZero() {
		  l.CreatedAt = now
	  }
	  l.UpdatedAt = now
	  if _, ok := b.legs[l.LegID]; !ok {
		  b.byPlan[l.PlanID] = append(b.byPlan[l.PlanID], l.LegID)
	   }
	  b.legs[l.LegID] = l
	  if l.IdempotencyKey != "" {
		   b.idemLeg[l.IdempotencyKey] = l.LegID
	   }
  }

   func (b *Book) GetLeg(_ context.Context, id string) (Leg, error) {
	  b.mu.RLock()
	   defer b.mu.RUnlock()
	  l, ok := b.legs[id]
	   if !ok {
		  return Leg{}, ErrLegNotFound
	  }
	  return l, nil
   }

   func (b *Book) LegsByPlan(_ context.Context, planID string) []Leg {
	   b.mu.RLock()
	   defer b.mu.RUnlock()
	   ids := b.byPlan[planID]
	   out := make([]Leg, 0, len(ids))
	  for _, id := range ids {
		  if l, ok := b.legs[id]; ok {
			  out = append(out, l)
		  }
	  }
	  return out
  }

   func (b *Book) UpdateLegState(_ context.Context, legID string, st LegState, reason string) error {
	  b.mu.Lock()
	  defer b.mu.Unlock()
	  l, ok := b.legs[legID]
	   if !ok {
		  return ErrLegNotFound
	  }
	   l.State = st
	  l.FailReason = reason
	  l.UpdatedAt = time.Now().UTC()
	  b.legs[legID] = l
	   return nil
   }

   func (b *Book) AttachFence(_ context.Context, legID, fenceID string) error {
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  l, ok := b.legs[legID]
	   if !ok {
		   return ErrLegNotFound
	   }
	  l.FenceID = fenceID
	   l.UpdatedAt = time.Now().UTC()
	  b.legs[legID] = l
	  return nil
  }

   func (b *Book) SaveIntent(in CollectIntent) {
	   b.mu.Lock()
	   defer b.mu.Unlock()
	   b.intents[in.IntentID] = in
	   if l, ok := b.legs[in.LegID]; ok {
		  l.IntentRef = in.IntentID
		   l.UpdatedAt = time.Now().UTC()
		   b.legs[in.LegID] = l
	  }
  }

   func (b *Book) IntentByLeg(legID string) *CollectIntent {
	  b.mu.RLock()
	   defer b.mu.RUnlock()
	  for i := range b.intents {
		   in := b.intents[i]
		   if in.LegID == legID {
			   cp := in
			   return &cp
		   }
	   }
	  return nil
   }

   func (b *Book) RecordMove(m InventoryMove) {
	   b.mu.Lock()
	   defer b.mu.Unlock()
	   b.moveByID[m.MoveID] = len(b.moves)
	  b.moves = append(b.moves, m)
	  if l, ok := b.legs[m.LegID]; ok {
		   l.TransferRef = m.MoveID
		   l.UpdatedAt = time.Now().UTC()
		   b.legs[m.LegID] = l
	   }
  }

   func (b *Book) LastMove(legID string) *InventoryMove {
	   b.mu.RLock()
	  defer b.mu.RUnlock()
	   for i := len(b.moves) - 1; i >= 0; i-- {
		  if b.moves[i].LegID == legID {
			  cp := b.moves[i]
			  return &cp
		   }
	   }
	   return nil
  }

   func (b *Book) SuccessfulMoves(legID string) int {
	  b.mu.RLock()
	  defer b.mu.RUnlock()
	  n := 0
	   for _, m := range b.moves {
		  if m.LegID == legID && m.Success {
			   n++
		   }
	   }
	  return n
  }

  func (b *Book) Balance(region, asset string) int64 {
	   b.mu.RLock()
	  defer b.mu.RUnlock()
	   r := b.regions[region]
	   if r.Balances == nil {
		  return 0
	  }
	   return r.Balances[asset]
  }

   func (b *Book) applyTransfer(src, dst, asset string, amount int64) error {
	  sr, ok := b.regions[src]
	  if !ok {
		   return ErrRegionNotFound
	   }
	   dr, ok := b.regions[dst]
	  if !ok {
		  return ErrRegionNotFound
	   }
	  if sr.Status == RegionHalted || dr.Status == RegionHalted {
		  return ErrRegionHalted
	   }
	  if sr.Balances == nil {
		   sr.Balances = map[string]int64{}
	   }
	  if dr.Balances == nil {
		  dr.Balances = map[string]int64{}
	   }
	   if sr.Balances[asset] < amount {
		   return ErrInsufficientInv
	  }
	  sr.Balances[asset] -= amount
	   dr.Balances[asset] += amount
	   now := time.Now().UTC()
	   sr.UpdatedAt = now
	   dr.UpdatedAt = now
	  b.regions[src] = sr
	  b.regions[dst] = dr
	  return nil
   }

   func (b *Book) Snapshot(_ context.Context, planID, legID string) (LegSnapshot, error) {
	  b.mu.RLock()
	  defer b.mu.RUnlock()
	  p, ok := b.plans[planID]
	  if !ok {
		   return LegSnapshot{}, ErrPlanNotFound
	  }
	  l, ok := b.legs[legID]
	   if !ok {
		   return LegSnapshot{}, ErrLegNotFound
	   }
	  src, ok := b.regions[l.SrcRegion]
	  if !ok {
		  return LegSnapshot{}, ErrRegionNotFound
	   }
	  dst, ok := b.regions[l.DstRegion]
	  if !ok {
		  return LegSnapshot{}, ErrRegionNotFound
	  }
	  snap := LegSnapshot{
		  Plan: p, Leg: l, Src: cloneRegion(src), Dst: cloneRegion(dst),
		  ObservedAt: time.Now().UTC(),
	   }
	   if in := b.intentLocked(legID); in != nil {
		  snap.Intent = in
	  }
	   if m := b.lastMoveLocked(legID); m != nil {
		   snap.LastMove = m
	   }
	   return snap, nil
  }

   func (b *Book) intentLocked(legID string) *CollectIntent {
	   for i := range b.intents {
		   in := b.intents[i]
		  if in.LegID == legID {
			   cp := in
			   return &cp
		   }
	  }
	   return nil
  }

  func (b *Book) lastMoveLocked(legID string) *InventoryMove {
	  for i := len(b.moves) - 1; i >= 0; i-- {
		  if b.moves[i].LegID == legID {
			   cp := b.moves[i]
			  return &cp
		  }
	  }
	  return nil
   }

  func (b *Book) AllLegsDone(planID string) bool {
	   b.mu.RLock()
	   defer b.mu.RUnlock()
	   ids := b.byPlan[planID]
	  if len(ids) == 0 {
		  return false
	  }
	   for _, id := range ids {
		   if b.legs[id].State != LegDone {
			   return false
		   }
	  }
	  return true
  }

  func (b *Book) Finalise(planID string) error {
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   p, ok := b.plans[planID]
	   if !ok {
		  return ErrPlanNotFound
	  }
	  ids := b.byPlan[planID]
	   if len(ids) == 0 {
		  return ErrPlanNotOpen
	  }
	   for _, id := range ids {
		   if b.legs[id].State != LegDone {
			  return ErrPlanNotOpen
		   }
	   }
	   p.State = FencePlanFinalised
	   p.UpdatedAt = time.Now().UTC()
	   b.plans[planID] = p
	   return nil
   }

   func cloneRegion(r Region) Region {
	  cp := r
	   if r.Balances != nil {
		  cp.Balances = make(map[string]int64, len(r.Balances))
		  for k, v := range r.Balances {
			  cp.Balances[k] = v
		   }
	   }
	   return cp
   }
