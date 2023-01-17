package wdshard

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Book 进程内分片账本。供 shardholdgen 只读挂钩与夹具。
type Book struct {
	mu    sync.RWMutex
	plans map[string]Plan
	legs  map[string]Leg
	holds map[string]Hold
	advCnt map[string]int64
	relCnt map[string]int64
	now func() time.Time
}

  func NewBook() *Book {
	  return &Book{
		  plans: make(map[string]Plan),
		   legs:  make(map[string]Leg),
		   holds:  make(map[string]Hold),
		  advCnt: make(map[string]int64),
		   relCnt: make(map[string]int64),
		  now: time.Now,
	  }
  }

  func (b *Book) WithClock(fn func() time.Time) *Book {
	   if b != nil && fn != nil {
		   b.now = fn
	   }
	  return b
  }

  func (b *Book) nowUTC() time.Time {
	  if b == nil || b.now == nil {
		  return time.Now().UTC()
	   }
	   return b.now().UTC()
   }

  func legKey(planID, legID string) string {
	  return strings.TrimSpace(planID) + "/" + strings.TrimSpace(legID)
   }

// PutPlan 登记计划。

  func (b *Book) PutPlan(p Plan) error {
	   if b == nil {
		  return ErrUnavailable
	  }
	   if strings.TrimSpace(p.PlanID) == "" || p.TotalAmt <= 0 {
		  return ErrInvalid
	  }
	  now := b.nowUTC()
	   if p.CreatedAt.IsZero() {
		   p.CreatedAt = now
	   }
	  p.UpdatedAt = now
	   if p.Status == "" {
		   p.Status = PlanOpen
	  }
	  b.mu.Lock()
	  defer b.mu.Unlock()
	  b.plans[p.PlanID] = p
	   return nil
   }

// PutLeg 登记腿。

   func (b *Book) PutLeg(l Leg) error {
	   if b == nil {
		   return ErrUnavailable
	   }
	   if strings.TrimSpace(l.PlanID) == "" || strings.TrimSpace(l.LegID) == "" || l.Amount <= 0 {
		  return ErrInvalid
	   }
	   if l.RemainAmt == 0 && l.FilledAmt == 0 {
		   l.RemainAmt = l.Amount
	  }
	  if l.Status == "" {
		  l.Status = HoldLegPending
	   }
	  l.UpdatedAt = b.nowUTC()
	  b.mu.Lock()
	   defer b.mu.Unlock()
	   if _, ok := b.plans[l.PlanID]; !ok {
		  return ErrPlanNotFound
	  }
	   b.legs[legKey(l.PlanID, l.LegID)] = l
	  return nil
   }

// Snapshot 读取计划与腿。

   func (b *Book) Snapshot(planID, legID string) (Snapshot, error) {
	  if b == nil {
		  return Snapshot{}, ErrUnavailable
	  }
	  b.mu.RLock()
	  defer b.mu.RUnlock()
	  p, ok := b.plans[strings.TrimSpace(planID)]
	  if !ok {
		  return Snapshot{}, ErrPlanNotFound
	   }
	  l, ok := b.legs[legKey(planID, legID)]
	   if !ok {
		   return Snapshot{}, ErrLegNotFound
	  }
	   var hp *Hold
	  if l.HoldID != "" {
		   if h, ok := b.holds[l.HoldID]; ok {
			  cp := h
			  hp = &cp
		  }
	   }
	  return Snapshot{Plan: p, Leg: l, Hold: hp}, nil
  }

// GetPlan 只读计划。
   func (b *Book) GetPlan(planID string) (Plan, error) {
	   if b == nil {
		   return Plan{}, ErrUnavailable
	  }
	  b.mu.RLock()
	  defer b.mu.RUnlock()
	   p, ok := b.plans[strings.TrimSpace(planID)]
	   if !ok {
		   return Plan{}, ErrPlanNotFound
	   }
	   return p, nil
  }

// ListLegs 列出计划下全部腿。
  func (b *Book) ListLegs(planID string) []Leg {
	   if b == nil {
		  return nil
	   }
	   b.mu.RLock()
	  defer b.mu.RUnlock()
	   out := make([]Leg, 0)
	  prefix := strings.TrimSpace(planID) + "/"
	  for k, l := range b.legs {
		  if strings.HasPrefix(k, prefix) {
			  out = append(out, l)
		  }
	   }
	  return out
   }

// FreezeHold HoldFunc.Freeze。

   func (b *Book) FreezeHold(planID, legID, userID, asset string, amount int64) (Hold, error) {
	   if b == nil {
		   return Hold{}, ErrUnavailable
	  }
	   if amount <= 0 || strings.TrimSpace(planID) == "" || strings.TrimSpace(legID) == "" {
		  return Hold{}, ErrInvalid
	  }
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   l, ok := b.legs[legKey(planID, legID)]
	  if !ok {
		  return Hold{}, ErrLegNotFound
	  }
	  if l.HoldID != "" {
		  if h, ok := b.holds[l.HoldID]; ok && h.Active && h.Residual() > 0 {
			   return Hold{}, ErrAlreadyHeld
		   }
	  }
	  id := newHoldID(planID, legID, amount, b.nowUTC())
	  h := Hold{
		   HoldID: id,
		   PlanID: planID, LegID: legID, UserID: userID, Asset: asset,
		   Amount: amount, Active: true, CreatedAt: b.nowUTC(),
	   }
	  b.holds[id] = h
	  l.HoldID = id
	  l.HoldAmt = amount
	  l.Status = HoldLegAdvancing
	   l.UpdatedAt = b.nowUTC()
	   b.legs[legKey(planID, legID)] = l
	  b.advCnt[legID]++
	   return h, nil
   }

// ReleaseHold HoldFunc.Release。释放额不得超过残额。
   func (b *Book) ReleaseHold(holdID string, amount int64) (Hold, error) {
	  if b == nil {
		   return Hold{}, ErrUnavailable
	   }
	   if strings.TrimSpace(holdID) == "" || amount <= 0 {
		  return Hold{}, ErrInvalid
	  }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	   h, ok := b.holds[holdID]
	   if !ok {
		   return Hold{}, ErrHoldNotFound
	  }
	   if amount > h.Residual() {
		  return Hold{}, ErrHoldExceed
	   }
	  h.Released += amount
	  if h.Residual() == 0 {
		  h.Active = false
	   }
	   b.holds[holdID] = h
	  if l, ok := b.legs[legKey(h.PlanID, h.LegID)]; ok {
		  l.HoldAmt = h.Residual()
		  if !h.Active {
			  l.Status = HoldLegReleased
			   l.HoldID = h.HoldID
		  }
		   l.UpdatedAt = b.nowUTC()
		  b.legs[legKey(h.PlanID, h.LegID)] = l
	   }
	   b.relCnt[h.LegID]++
	   return h, nil
  }

   func (b *Book) GetHold(holdID string) (Hold, error) {
	   if b == nil {
		  return Hold{}, ErrUnavailable
	   }
	   b.mu.RLock()
	  defer b.mu.RUnlock()
	   h, ok := b.holds[holdID]
	  if !ok {
		   return Hold{}, ErrHoldNotFound
	   }
	   return h, nil
  }

// SubmitBroadcast 提交广播，腿进入 broadcasting 并钉 live_hash。
   func (b *Book) SubmitBroadcast(planID, legID, broadcastID string) (Leg, error) {
	   if b == nil {
		   return Leg{}, ErrUnavailable
	  }
	   b.mu.Lock()
	  defer b.mu.Unlock()
	  k := legKey(planID, legID)
	  l, ok := b.legs[k]
	  if !ok {
		   return Leg{}, ErrLegNotFound
	  }
	   if l.HoldID == "" {
		  return Leg{}, ErrNoHold
	  }
	  if l.Status != HoldLegAdvancing && l.Status != HoldLegPending && l.Status != HoldLegFailed {
		   return Leg{}, ErrState
	   }
	  if broadcastID == "" {
		  broadcastID = "bc-" + l.LegID
	  }
	  l.BroadcastID = broadcastID
	  l.Status = HoldLegBroadcasting
	   l.LiveHash = liveHash(l.PlanID, l.LegID, l.HoldID, l.Amount, broadcastID)
	   l.FailReason = ""
	   l.UpdatedAt = b.nowUTC()
	   b.legs[k] = l
	  return l, nil
  }

// FailBroadcast 广播失败，腿变 failed，冻结仍在。Release 与重 Advance 互斥由世代层处理。
   func (b *Book) FailBroadcast(planID, legID, reason string) (Leg, error) {
	  if b == nil {
		  return Leg{}, ErrUnavailable
	   }
	   b.mu.Lock()
	  defer b.mu.Unlock()
	  k := legKey(planID, legID)
	  l, ok := b.legs[k]
	   if !ok {
		  return Leg{}, ErrLegNotFound
	   }
	   if l.Status != HoldLegBroadcasting && l.Status != HoldLegAdvancing {
		   return Leg{}, ErrState
	  }
	   l.Status = HoldLegFailed
	  l.FailReason = reason
	  l.UpdatedAt = b.nowUTC()
	  b.legs[k] = l
	   return l, nil
   }

// AlignFill 只推进成交计数，不改部分成交公式主链。
  func (b *Book) AlignFill(hint AlignHint) (Leg, error) {
	  if b == nil {
		  return Leg{}, ErrUnavailable
	  }
	  if hint.FillDelta < 0 {
		   return Leg{}, ErrInvalid
	  }
	  b.mu.Lock()
	   defer b.mu.Unlock()
	   k := legKey(hint.PlanID, hint.LegID)
	  l, ok := b.legs[k]
	   if !ok {
		  return Leg{}, ErrLegNotFound
	  }
	  if l.Status != HoldLegBroadcasting && l.Status != HoldLegFilled {
		   return Leg{}, ErrState
	  }
	  l.FilledAmt += hint.FillDelta
	  if l.FilledAmt > l.Amount {
		  l.FilledAmt = l.Amount
	   }
	  l.RemainAmt = l.Amount - l.FilledAmt
	   if hint.TxHash != "" {
		   l.TxHash = hint.TxHash
	   }
	  if l.RemainAmt == 0 {
		  l.Status = HoldLegFilled
	   }
	  l.UpdatedAt = b.nowUTC()
	  b.legs[k] = l
	   return l, nil
  }

// MarkPending 将失败腿重置为 pending（仅账本状态，须由世代闸决定是否允许）。

   func (b *Book) MarkPending(planID, legID string) (Leg, error) {
	  if b == nil {
		   return Leg{}, ErrUnavailable
	  }
	   b.mu.Lock()
	  defer b.mu.Unlock()
	   k := legKey(planID, legID)
	   l, ok := b.legs[k]
	  if !ok {
		  return Leg{}, ErrLegNotFound
	  }
	  l.Status = HoldLegPending
	   l.BroadcastID = ""
	   l.LiveHash = ""
	  l.FailReason = ""
	  l.UpdatedAt = b.nowUTC()
	   b.legs[k] = l
	  return l, nil
   }

   func (b *Book) AdvanceCount(legID string) int64 {
	   if b == nil {
		  return 0
	   }
	   b.mu.RLock()
	  defer b.mu.RUnlock()
	  return b.advCnt[legID]
   }

   func (b *Book) ReleaseCount(legID string) int64 {
	   if b == nil {
		  return 0
	  }
	   b.mu.RLock()
	   defer b.mu.RUnlock()
	   return b.relCnt[legID]
   }

   func (b *Book) ActiveHoldCount() int {
	   if b == nil {
		   return 0
	   }
	   b.mu.RLock()
	  defer b.mu.RUnlock()
	  n := 0
	  for _, h := range b.holds {
		  if h.Active && h.Residual() > 0 {
			  n++
		   }
	   }
	  return n
  }

  func newHoldID(planID, legID string, amount int64, now time.Time) string {
	  sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%d|%d", planID, legID, amount, now.UnixNano())))
	  return "hld-" + hex.EncodeToString(sum[:8])
  }

  func liveHash(planID, legID, holdID string, amount int64, bc string) string {
	  sum := sha256.Sum256([]byte(fmt.Sprintf("live|%s|%s|%s|%d|%s", planID, legID, holdID, amount, bc)))
	   return hex.EncodeToString(sum[:])
   }

// SeedDemo 与启动夹具。

  func SeedDemo(b *Book, planID, userID, asset string, total int64, n int) error {
	  if b == nil {
		  return ErrUnavailable
	  }
	   if n <= 0 {
		   n = 2
	   }
	  if total <= 0 {
		  total = 1000
	  }
	  each := total / int64(n)
	   if each <= 0 {
		  each = 1
	  }
	   p := Plan{
		  PlanID: planID, UserID: userID, Asset: asset, Chain: "eth",
		  DestAddr: "seed-dest", TotalAmt: each * int64(n), SplitCount: n, Status: PlanOpen,
	  }
	  if err := b.PutPlan(p); err != nil {
		  return err
	  }
	  for i := 0; i < n; i++ {
		   legID := fmt.Sprintf("%s-L%d", planID, i+1)
		  if err := b.PutLeg(Leg{
			  PlanID: planID, LegID: legID, Seq: i + 1, Amount: each, RemainAmt: each, Status: HoldLegPending,
		  }); err != nil {
			  return err
		  }
	   }
	  return nil
   }

// SeedFailedLeg 构造已冻结且广播失败的腿，供 Advance↔Release 竞态夹具。
  func SeedFailedLeg(b *Book, planID, legID, userID, asset string, amount int64) (Hold, error) {
	   if err := b.PutPlan(Plan{PlanID: planID, UserID: userID, Asset: asset, TotalAmt: amount, SplitCount: 1, Status: PlanOpen}); err != nil {
		   return Hold{}, err
	   }
	   if err := b.PutLeg(Leg{PlanID: planID, LegID: legID, Seq: 1, Amount: amount, RemainAmt: amount, Status: HoldLegPending}); err != nil {
		   return Hold{}, err
	  }
	  h,
		  err := b.FreezeHold(planID, legID, userID, asset, amount)
	   if err != nil {
		  return Hold{}, err
	   }
	   if _, err := b.SubmitBroadcast(planID, legID, "bc-fail"); err != nil {
		   return Hold{}, err
	   }
	  if _, err := b.FailBroadcast(planID, legID, "chain_reject"); err != nil {
		   return Hold{}, err
	  }
	  return h, nil
   }

// SeedBroadcastingLeg 构造在播腿。

  func SeedBroadcastingLeg(b *Book, planID, legID, userID, asset string, amount int64) (Hold, error) {
	   if err := b.PutPlan(Plan{PlanID: planID, UserID: userID, Asset: asset, TotalAmt: amount, SplitCount: 1, Status: PlanOpen}); err != nil {
		   return Hold{}, err
	   }
	  if err := b.PutLeg(Leg{PlanID: planID, LegID: legID, Seq: 1, Amount: amount, RemainAmt: amount, Status: HoldLegPending}); err != nil {
		  return Hold{}, err
	   }
	   h, err := b.FreezeHold(planID, legID, userID, asset, amount)
	   if err != nil {
		   return Hold{}, err
	   }
	  if _, err := b.SubmitBroadcast(planID, legID, "bc-live"); err != nil {
		  return Hold{}, err
	   }
	  return h, nil
  }
