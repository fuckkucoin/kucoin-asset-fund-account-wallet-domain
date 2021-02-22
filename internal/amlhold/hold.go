package amlhold

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
)

// LedgerObserver 只读观察账本冻结位。调用方不得在此路径写账本。

type LedgerObserver interface {
	// FrozenBit 返回用户+资产的账本冻结位与实际冻结金额描述。
	// 若账本观察失败，返回 false 与空字符串（不视为漂移，仅记录观察失败）。
	FrozenBit(userID, asset string) (frozen bool, ledgerState string)
}

// Book 检疫 Hold 账册（in-memory，持久化通过 Store 接口扩展）。

type Book struct {
	mu          sync.Mutex
	holds       map[string]*QuarantineHold // holdID → hold
	byCaseAsset map[string][]string        // caseID:asset → []holdID
	byCase map[string][]string // caseID → []holdID
	byUser map[string][]string // userID → []holdID
	tickets map[string]*ReleaseTicket // ticketID → ticket
	byHold  map[string][]string       // holdID → []ticketID
	events  map[string][]AgingEvent   // holdID → []AgingEvent
	proofs  map[string][]HoldProof    // holdID → []HoldProof
	gaps    map[string]*GapTicket     // gapID → gap
	fence map[string]fenceRow // holdID → fence state
	tFence map[string]fenceRow // ticketID → fence state
	ledger LedgerObserver
	clock func() time.Time
}

type fenceRow struct {
	epoch int64
	token string
}

// NewBook 初始化空账册。
  func NewBook(obs LedgerObserver) *Book {
	  return &Book{
		  holds:       make(map[string]*QuarantineHold),
		   byCaseAsset: make(map[string][]string),
		   byCase:      make(map[string][]string),
		  byUser: make(map[string][]string),
		  tickets: make(map[string]*ReleaseTicket),
		  byHold:  make(map[string][]string),
		  events:  make(map[string][]AgingEvent),
		  proofs:  make(map[string][]HoldProof),
		   gaps: make(map[string]*GapTicket),
		   fence:  make(map[string]fenceRow),
		  tFence: make(map[string]fenceRow),
		   ledger: obs,
		   clock:  func() time.Time { return time.Now().UTC() },
	   }
   }

// WithClock 注入时钟。

   func (b *Book) WithClock(fn func() time.Time) *Book {
	   if b != nil && fn != nil {
		  b.clock = fn
	   }
	  return b
  }

   // WithLedger 注入账本观察者（延迟注入）。
  func (b *Book) WithLedger(obs LedgerObserver) *Book {
	   if b != nil {
		   b.mu.Lock()
		   b.ledger = obs
		  b.mu.Unlock()
	  }
	   return b
   }

// ────────────────────────────────────────────────────────────────────────────
// 围栏辅助
// ────────────────────────────────────────────────────────────────────────────

  func newToken() string {
	  var raw [16]byte
	  // 使用 uuid 随机生成保证唯一性
	   id := uuid.New()
	  copy(raw[:], id[:])
	  return hex.EncodeToString(raw[:])
  }

   func (b *Book) bumpHoldFence(id string) (int64, string) {
	   cur := b.fence[id]
	   cur.epoch++
	   cur.token = newToken()
	  b.fence[id] = cur
	   return cur.epoch, cur.token
   }

  func (b *Book) checkHoldFence(id string, epoch int64, token string) error {
	  cur, ok := b.fence[id]
	  if !ok {
		  return ErrNotFound
	   }
	   if cur.epoch != epoch || cur.token != token {
		  return ErrStaleFence
	   }
	  return nil
  }

  func (b *Book) bumpTicketFence(id string) (int64, string) {
	  cur := b.tFence[id]
	   cur.epoch++
	   cur.token = newToken()
	  b.tFence[id] = cur
	   return cur.epoch, cur.token
  }

  func (b *Book) checkTicketFence(id string, epoch int64, token string) error {
	  cur, ok := b.tFence[id]
	  if !ok {
		   return ErrTicketNotFound
	   }
	   if cur.epoch != epoch || cur.token != token {
		  return ErrStaleFence
	   }
	  return nil
  }

// ────────────────────────────────────────────────────────────────────────────
// OpenHold：开启检疫资金 Hold
// ────────────────────────────────────────────────────────────────────────────
// OpenHold 为一个 AML Case 开启检疫资金冻结占用。
// - 同一 caseID+asset 只允许一个 open 态的 hold（幂等）。
// - 开启后立即检查账本漂移；若漂移则记录缺口单但不阻止 open。
// - slaLimitSec 为零时使用默认 SLA（86400s）。

   func (b *Book) OpenHold(caseID, userID, asset, amount, actor string, slaLimitSec int64) (QuarantineHold, error) {
	  if b == nil {
		   return QuarantineHold{}, ErrUnavailable
	   }
	  actor = strings.TrimSpace(actor)
	  if actor == "" {
		   return QuarantineHold{}, ErrActor
	  }
	  caseID = strings.TrimSpace(caseID)
	  userID = strings.TrimSpace(userID)
	  asset = strings.TrimSpace(strings.ToUpper(asset))
	  amount = strings.TrimSpace(amount)
	   if caseID == "" {
		  return QuarantineHold{}, ErrCaseNotFound
	  }
	  if userID == "" {
		   return QuarantineHold{}, ErrActor
	   }
	  if asset == "" {
		   return QuarantineHold{}, ErrAsset
	   }
	  if amount == "" || amount == "0" {
		  return QuarantineHold{}, ErrAmount
	  }
	   if slaLimitSec <= 0 {
		  slaLimitSec = DefaultAgingConfig.WarnAfter
	   }
	   b.mu.Lock()
	  defer b.mu.Unlock()
	  // 幂等：已有活跃 hold 则直接返回
	   key := caseID + ":" + asset
	   for _, hid := range b.byCaseAsset[key] {
		   if h := b.holds[hid]; h != nil && h.Open() {
			  return *h, nil
		   }
	  }
	   now := b.clock()
	  h := &QuarantineHold{
		   ID:     uuid.NewString(),
		  CaseID: caseID,
		  UserID: userID,
		   Asset:  asset,
		   Amount: amount,
		   Status: HoldStatusOpen,
		  AgingTier: AgingTierNone,
		  OpenedBy:  actor,
		  SLALimit:  slaLimitSec,
		  OpenedAt: now,
	   }
	  // 观察账本冻结位
	  if b.ledger != nil {
		   frozen, state := b.ledger.FrozenBit(userID, asset)
		  h.LedgerFrozenBit = frozen
		  if !frozen {
			  // 账本未冻结但检疫要求冻结 → 漂移，开缺口单
			  gap := b.openGapLocked(h,
				   state, now)
			   h.GapTicketID = gap.ID
		  }
	  }
	   ep, tok := b.bumpHoldFence(h.ID)
	  h.FenceEpoch = ep
	   h.FenceToken = tok
	   b.holds[h.ID] = h
	   b.byCaseAsset[key] = append(b.byCaseAsset[key],
		   h.ID)
	  b.byCase[caseID] = append(b.byCase[caseID], h.ID)
	  b.byUser[userID] = append(b.byUser[userID], h.ID)
	  // 开启占用证明
	  b.appendProofLocked(h.ID, caseID, "", "", ProofKindOccupy, actor,
		   fmt.Sprintf("quarantine hold opened: case=%s asset=%s amount=%s", caseID, asset, amount))
	   cp := *h
	  return cp, nil
  }

// ────────────────────────────────────────────────────────────────────────────
// 查询接口
// ────────────────────────────────────────────────────────────────────────────
// GetHold 按 holdID 查。
  func (b *Book) GetHold(id string) (QuarantineHold, error) {
	  if b == nil {
		   return QuarantineHold{}, ErrUnavailable
	  }
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  h,
		  ok := b.holds[strings.TrimSpace(id)]
	   if !ok {
		  return QuarantineHold{}, ErrNotFound
	   }
	  return *h, nil
  }

// ListByCase 列出一个 Case 下所有 hold。
  func (b *Book) ListByCase(caseID string, openOnly bool) []QuarantineHold {
	   if b == nil {
		   return nil
	  }
	   b.mu.Lock()
	  defer b.mu.Unlock()
	   ids := b.byCase[strings.TrimSpace(caseID)]
	   out := make([]QuarantineHold, 0, len(ids))
	  for _, id := range ids {
		   if h := b.holds[id]; h != nil {
			   if !openOnly || h.Open() {
				   out = append(out, *h)
			   }
		   }
	  }
	  return out
  }

// ListByUser 列出用户下所有 hold。
  func (b *Book) ListByUser(userID string, filter HoldFilter) []QuarantineHold {
	  if b == nil {
		   return nil
	  }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	   ids := b.byUser[strings.TrimSpace(userID)]
	   limit := filter.Limit
	  if limit <= 0 {
		  limit = 100
	  }
	   out := make([]QuarantineHold, 0, limit)
	   for _, id := range ids {
		   h := b.holds[id]
		   if h == nil {
			  continue
		   }
		  if filter.OpenOnly && !h.Open() {
			   continue
		  }
		  if filter.Status != "" && h.Status != filter.Status {
			   continue
		   }
		  if filter.AgingTier != "" && h.AgingTier != filter.AgingTier {
			   continue
		   }
		   out = append(out, *h)
		  if len(out) >= limit {
			  break
		  }
	   }
	   return out
  }

// OpenHoldsForCase 返回 Case 下所有未终结的 hold（用于结案闸检查）。

   func (b *Book) OpenHoldsForCase(caseID string) []QuarantineHold {
	  return b.ListByCase(caseID, true)
   }

// AllOpenHolds 返回所有活跃 hold（供 SLA 巡检使用）。

   func (b *Book) AllOpenHolds() []QuarantineHold {
	   if b == nil {
		   return nil
	   }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	  out := make([]QuarantineHold, 0, len(b.holds))
	   for _, h := range b.holds {
		   if h.Open() {
			   out = append(out, *h)
		  }
	  }
	   return out
  }

// ────────────────────────────────────────────────────────────────────────────
// 状态流转：老化升级（仅升级，不解冻）
// ────────────────────────────────────────────────────────────────────────────
// AdvanceAging 将 hold 推进到下一老化级别。
// SLA 超时不得自动结案，只能升级老化层级。
// 调用方需校验 fence（epoch/token）防止陈旧 worker 并发升级。
  func (b *Book) AdvanceAging(holdID, actor, reason string, epoch int64, token string) (QuarantineHold, AgingEvent, error) {
	  if b == nil {
		   return QuarantineHold{}, AgingEvent{}, ErrUnavailable
	   }
	  actor = strings.TrimSpace(actor)
	  if actor == "" {
		  return QuarantineHold{}, AgingEvent{}, ErrActor
	   }
	   b.mu.Lock()
	  defer b.mu.Unlock()
	  if err := b.checkHoldFence(holdID, epoch, token); err != nil {
		   return QuarantineHold{}, AgingEvent{}, err
	   }
	  h, ok := b.holds[holdID]
	   if !ok {
		   return QuarantineHold{}, AgingEvent{}, ErrNotFound
	   }
	   if h.Terminal() {
		  return *h, AgingEvent{}, ErrTerminal
	  }
	   now := b.clock()
	   fromStatus := h.Status
	  fromTier := h.AgingTier
	   toStatus, toTier := nextAging(fromStatus, fromTier)
	  if toStatus == fromStatus && toTier == fromTier {
		   // 已在最终老化级别
		   return *h, AgingEvent{}, nil
	  }
	  age := int64(now.Sub(h.OpenedAt).Seconds())
	   ev := AgingEvent{
		  ID:     uuid.NewString(),
		  HoldID: holdID,
		   CaseID: h.CaseID,
		   FromTier:   fromTier,
		  ToTier:     toTier,
		   FromStatus: fromStatus,
		  ToStatus: toStatus,
		  Actor: actor,
		  Reason: strings.TrimSpace(reason),
		  Age: age,
		   At:  now,
	   }
	   b.events[holdID] = append(b.events[holdID],
		   ev)
	   h.AgingTier = toTier
	  h.Status = toStatus
	   if toStatus == HoldStatusAging && h.AgingAt.IsZero() {
		  h.AgingAt = now
	  }
	  if toStatus == HoldStatusEscalated && h.EscalatedAt.IsZero() {
		  h.EscalatedAt = now
	   }
	  ep, tok := b.bumpHoldFence(holdID)
	  h.FenceEpoch = ep
	  h.FenceToken = tok
	  proofKind := ProofKindAging
	   if toStatus == HoldStatusEscalated || toStatus == HoldStatusCompelReview {
		  proofKind = ProofKindEscalate
	   }
	  b.appendProofLocked(holdID, h.CaseID, ev.ID, "", proofKind, actor,
		  fmt.Sprintf("aging advance: %s→%s tier=%s age=%ds", fromStatus, toStatus, toTier, age))
	  cp := *h
	   return cp, ev, nil
   }

// nextAging 计算下一个老化状态与级别。
   func nextAging(status HoldStatus, tier AgingTier) (HoldStatus, AgingTier) {
	   switch status {
	  case HoldStatusOpen:
		  return HoldStatusAging,
			   AgingTierWarn
	   case HoldStatusAging:
		  switch tier {
		   case AgingTierNone, AgingTierWarn:
			   return HoldStatusAging, AgingTierForceHold
		  case AgingTierForceHold:
			  return HoldStatusEscalated,
				   AgingTierEscalated
		   default:
			   return HoldStatusEscalated,
				   AgingTierEscalated
		   }
	  case HoldStatusEscalated:
		   return HoldStatusCompelReview, AgingTierCompelReview
	  default:
		  return status, tier
	   }
   }

// AgingEvents 返回 hold 的老化事件列表。
  func (b *Book) AgingEvents(holdID string) []AgingEvent {
	  if b == nil {
		  return nil
	  }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	  src := b.events[holdID]
	   out := make([]AgingEvent, len(src))
	  copy(out, src)
	  return out
   }

// ────────────────────────────────────────────────────────────────────────────
// 解冻票务：申请、审批、执行
// ────────────────────────────────────────────────────────────────────────────
// RequestRelease 申请解冻。requestedBy 不得与结案人相同（Release↔Close Mutex 在结案时校验）。
// 同一 hold 已有 pending ticket 时幂等返回现有 ticket。
   func (b *Book) RequestRelease(holdID, caseID, requestedBy, reason string) (ReleaseTicket, error) {
	   if b == nil {
		  return ReleaseTicket{},
			   ErrUnavailable
	  }
	  requestedBy = strings.TrimSpace(requestedBy)
	  if requestedBy == "" {
		  return ReleaseTicket{}, ErrActor
	   }
	   reason = strings.TrimSpace(reason)
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   h, ok := b.holds[strings.TrimSpace(holdID)]
	   if !ok {
		   return ReleaseTicket{},
			  ErrNotFound
	   }
	  if h.Terminal() {
		   return ReleaseTicket{}, ErrTerminal
	   }
	   // 幂等：已有 pending ticket 则返回
	   for _, tid := range b.byHold[holdID] {
		  if t := b.tickets[tid]; t != nil && t.Status == TicketStatusPending {
			  return *t, nil
		   }
	   }
	  now := b.clock()
	   t := &ReleaseTicket{
		   ID:          uuid.NewString(),
		  HoldID:      holdID,
		   CaseID:      strings.TrimSpace(caseID),
		   RequestedBy: requestedBy,
		  Status:      TicketStatusPending,
		   Reason:    reason,
		   CreatedAt: now,
	  }
	  ep, tok := b.bumpTicketFence(t.ID)
	  t.FenceEpoch = ep
	   t.FenceToken = tok
	   b.tickets[t.ID] = t
	  b.byHold[holdID] = append(b.byHold[holdID], t.ID)
	   cp := *t
	   return cp, nil
   }

// ApproveRelease 批准解冻申请（换人四眼：approvedBy≠requestedBy）。
// 批准后将 hold 置为 released，并生成 HoldProof。

  func (b *Book) ApproveRelease(ticketID, approvedBy string, epoch int64, token string) (ReleaseTicket, QuarantineHold, error) {
	   if b == nil {
		   return ReleaseTicket{},
			   QuarantineHold{}, ErrUnavailable
	   }
	  approvedBy = strings.TrimSpace(approvedBy)
	  if approvedBy == "" {
		  return ReleaseTicket{}, QuarantineHold{}, ErrActor
	   }
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   if err := b.checkTicketFence(ticketID, epoch, token); err != nil {
		  return ReleaseTicket{}, QuarantineHold{}, err
	   }
	  t, ok := b.tickets[strings.TrimSpace(ticketID)]
	  if !ok {
		  return ReleaseTicket{}, QuarantineHold{}, ErrTicketNotFound
	  }
	  if t.Status != TicketStatusPending {
		  return *t, QuarantineHold{}, ErrTicketTerminal
	   }
	   if strings.EqualFold(t.RequestedBy, approvedBy) {
		   return ReleaseTicket{}, QuarantineHold{}, ErrFourEye
	  }
	  h, ok := b.holds[t.HoldID]
	   if !ok {
		   return ReleaseTicket{}, QuarantineHold{}, ErrNotFound
	   }
	   if h.Terminal() {
		  return *t, *h, ErrTerminal
	  }
	  now := b.clock()
	   t.ApprovedBy = approvedBy
	  t.Status = TicketStatusApproved
	  t.DecidedAt = now
	  ep,
		   tok := b.bumpTicketFence(ticketID)
	  t.FenceEpoch = ep
	   t.FenceToken = tok
	  // 执行解冻：hold → released
	   h.Status = HoldStatusReleased
	   h.ReleasedBy = approvedBy
	  h.ReleasedAt = now
	  hEp, hTok := b.bumpHoldFence(h.ID)
	  h.FenceEpoch = hEp
	  h.FenceToken = hTok
	  // 更新 ticket done 状态
	   t.Status = TicketStatusDone
	  // 生成解冻证明
	   summary := fmt.Sprintf("hold released: hold_id=%s case_id=%s requester=%s approver=%s",
		   h.ID, h.CaseID, t.RequestedBy, approvedBy)
	  proof := b.appendProofLocked(h.ID, h.CaseID, "", t.ID, ProofKindRelease, approvedBy, summary)
	  t.ProofID = proof.ID
	  return *t, *h, nil
   }

// RejectRelease 拒绝解冻申请。

   func (b *Book) RejectRelease(ticketID, rejectedBy, reason string, epoch int64, token string) (ReleaseTicket, error) {
	  if b == nil {
		  return ReleaseTicket{}, ErrUnavailable
	   }
	  rejectedBy = strings.TrimSpace(rejectedBy)
	  if rejectedBy == "" {
		   return ReleaseTicket{}, ErrActor
	   }
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   if err := b.checkTicketFence(ticketID, epoch, token); err != nil {
		  return ReleaseTicket{}, err
	  }
	   t, ok := b.tickets[ticketID]
	   if !ok {
		   return ReleaseTicket{},
			  ErrTicketNotFound
	   }
	  if t.Status != TicketStatusPending {
		   return *t, ErrTicketTerminal
	   }
	   if strings.EqualFold(t.RequestedBy, rejectedBy) {
		   return ReleaseTicket{},
			   ErrFourEye
	   }
	   now := b.clock()
	  t.RejectedBy = rejectedBy
	   t.RejectReason = strings.TrimSpace(reason)
	  t.Status = TicketStatusRejected
	  t.DecidedAt = now
	  ep, tok := b.bumpTicketFence(ticketID)
	   t.FenceEpoch = ep
	  t.FenceToken = tok
	   cp := *t
	  return cp, nil
   }

// GetTicket 按 ticketID 查解冻申请单。

  func (b *Book) GetTicket(ticketID string) (ReleaseTicket, error) {
	  if b == nil {
		   return ReleaseTicket{}, ErrUnavailable
	   }
	  b.mu.Lock()
	  defer b.mu.Unlock()
	  t, ok := b.tickets[strings.TrimSpace(ticketID)]
	   if !ok {
		  return ReleaseTicket{}, ErrTicketNotFound
	  }
	   return *t, nil
  }

// TicketsByHold 按 holdID 列出所有解冻申请单。
   func (b *Book) TicketsByHold(holdID string) []ReleaseTicket {
	   if b == nil {
		   return nil
	  }
	  b.mu.Lock()
	  defer b.mu.Unlock()
	  ids := b.byHold[holdID]
	  out := make([]ReleaseTicket, 0, len(ids))
	   for _, id := range ids {
		  if t := b.tickets[id]; t != nil {
			   out = append(out, *t)
		  }
	   }
	   return out
   }

// ────────────────────────────────────────────────────────────────────────────
// Hold Proof 仓
// ────────────────────────────────────────────────────────────────────────────

  func proofChecksum(kind ProofKind, holdID, caseID, actor, summary string) string {
	   raw := fmt.Sprintf("%s|%s|%s|%s|%s", kind, holdID, caseID, actor, summary)
	  sum := sha256.Sum256([]byte(raw))
	  return hex.EncodeToString(sum[:])
   }

// appendProofLocked 追加证明（调用方持 mu）。

   func (b *Book) appendProofLocked(holdID, caseID, eventID, ticketID string, kind ProofKind, actor, summary string) HoldProof {
	   p := HoldProof{
		   ID: uuid.NewString(),
		  HoldID: holdID,
		  CaseID: caseID,
		   TicketID: ticketID,
		   EventID:   eventID,
		  ProofKind: kind,
		  Actor:     actor,
		   Summary:  summary,
		   Checksum: proofChecksum(kind, holdID, caseID, actor, summary),
		  At: b.clock(),
	  }
	   b.proofs[holdID] = append(b.proofs[holdID], p)
	   return p
  }

// Proofs 返回 hold 的全部证明（只读副本）。
  func (b *Book) Proofs(holdID string) []HoldProof {
	  if b == nil {
		  return nil
	  }
	   b.mu.Lock()
	  defer b.mu.Unlock()
	   src := b.proofs[holdID]
	   out := make([]HoldProof, len(src))
	  copy(out, src)
	  return out
  }

// ProofsByCase 返回 Case 下所有 hold 的全部证明。

   func (b *Book) ProofsByCase(caseID string) []HoldProof {
	  if b == nil {
		  return nil
	  }
	  b.mu.Lock()
	  defer b.mu.Unlock()
	  ids := b.byCase[strings.TrimSpace(caseID)]
	   var out []HoldProof
	  for _, hid := range ids {
		  out = append(out,
			  b.proofs[hid]...)
	  }
	  return out
   }

// ────────────────────────────────────────────────────────────────────────────
// 漂移巡检
// ────────────────────────────────────────────────────────────────────────────
// CheckDrift 巡检所有活跃 hold 的账本冻结位。
// 漂移时追加缺口单并生成漂移证明；以账本为准（不自动修改 hold 金额）。
  func (b *Book) CheckDrift() []GapTicket {
	   if b == nil || b.ledger == nil {
		  return nil
	  }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	   now := b.clock()
	   var gaps []GapTicket
	   for _, h := range b.holds {
		   if !h.Open() {
			  continue
		  }
		   frozen, state := b.ledger.FrozenBit(h.UserID, h.Asset)
		  h.LedgerFrozenBit = frozen
		  if !frozen {
			   // 账本已解冻但检疫 hold 仍 open → 漂移
			   gap := b.openGapLocked(h, state, now)
			   h.GapTicketID = gap.ID
			  b.appendProofLocked(h.ID, h.CaseID, "", "", ProofKindDrift, "system",
				  fmt.Sprintf("ledger drift detected: hold=%s user=%s asset=%s ledger_state=%s",
					  h.ID,
					  h.UserID, h.Asset, state))
			  gaps = append(gaps, gap)
		  }
	  }
	   return gaps
  }

  func (b *Book) openGapLocked(h *QuarantineHold, ledgerState string, now time.Time) GapTicket {
	  gap := GapTicket{
		  ID:     uuid.NewString(),
		  HoldID: h.ID,
		  CaseID: h.CaseID,
		  UserID: h.UserID,
		   Asset:  h.Asset,
		  HoldAmount: h.Amount,
		  LedgerState: ledgerState,
		  Reason:      "ledger freeze bit absent while quarantine hold open",
		  CreatedAt:   now,
	  }
	   b.gaps[gap.ID] = &gap
	  return gap
  }

// GapTickets 返回与 holdID 关联的所有缺口单。

   func (b *Book) GapTickets(holdID string) []GapTicket {
	   if b == nil {
		   return nil
	  }
	  b.mu.Lock()
	   defer b.mu.Unlock()
	   var out []GapTicket
	   for _, g := range b.gaps {
		   if g.HoldID == holdID {
			   out = append(out, *g)
		  }
	   }
	   return out
   }

// ResolveGap 标记缺口单为已解决。
  func (b *Book) ResolveGap(gapID, actor string) error {
	  if b == nil {
		   return ErrUnavailable
	  }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	   g, ok := b.gaps[gapID]
	   if !ok {
		  return ErrNotFound
	  }
	  g.Resolved = true
	   g.ResolvedAt = b.clock()
	   return nil
   }

// ────────────────────────────────────────────────────────────────────────────
// Release↔Close 互斥闸
// ────────────────────────────────────────────────────────────────────────────
// AssertCaseMayResolve 结案前调用：如果 Case 下仍有未释放的 hold，返回 ErrHoldBlocked。
// 调用方（amldesk.Book.CloseFourEye 增强版）在结案前必须调此函数。
   func (b *Book) AssertCaseMayResolve(caseID string) error {
	  if b == nil {
		   return ErrUnavailable
	   }
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  for _, hid := range b.byCase[strings.TrimSpace(caseID)] {
		  if h := b.holds[hid]; h != nil && h.Open() {
			   return fmt.Errorf("%w: hold_id=%s status=%s", ErrHoldBlocked, hid, h.Status)
		   }
	   }
	  return nil
   }

// AssertReleaseMayProceed 解冻前调用：解冻申请人不得与已知结案人相同。
// caseCloserActor 由调用方从 amldesk.Book 获取。
  func (b *Book) AssertReleaseMayProceed(holdID, releaseActor, caseCloserActor string) error {
	  if strings.EqualFold(strings.TrimSpace(releaseActor),
		  strings.TrimSpace(caseCloserActor)) &&
		  caseCloserActor != "" {
		  return ErrFourEye
	  }
	   return nil
   }

// AbortHold 管理员级别强制终止 hold（需要独立权限控制，不走正常解冻流程）。
  func (b *Book) AbortHold(holdID, actor, reason string, epoch int64, token string) (QuarantineHold, error) {
	   if b == nil {
		  return QuarantineHold{}, ErrUnavailable
	   }
	   actor = strings.TrimSpace(actor)
	  if actor == "" {
		   return QuarantineHold{}, ErrActor
	   }
	   b.mu.Lock()
	  defer b.mu.Unlock()
	   if err := b.checkHoldFence(holdID, epoch, token); err != nil {
		   return QuarantineHold{}, err
	   }
	   h, ok := b.holds[holdID]
	   if !ok {
		   return QuarantineHold{}, ErrNotFound
	   }
	   if h.Terminal() {
		  return *h, ErrTerminal
	   }
	   now := b.clock()
	   h.Status = HoldStatusAborted
	   h.AbortedBy = actor
	   h.AbortedAt = now
	   ep, tok := b.bumpHoldFence(holdID)
	   h.FenceEpoch = ep
	   h.FenceToken = tok
	   b.appendProofLocked(holdID, h.CaseID, "", "", ProofKindAbort, actor,
		   fmt.Sprintf("hold aborted: hold_id=%s reason=%s", holdID, reason))
	  cp := *h
	   return cp, nil
   }
