package fiatbreak

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/fiatrail"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/validate"
)

// BreakService 断裂检测与生命周期管理。
// 进程内内存存储，支持从 SQL 持久化替换（须接 SQL 层）。
type BreakService struct {
	mu sync.RWMutex
	// breaks 按 ID 存储所有断裂记录。
	breaks map[string]*RailBreak
	// breaksByBooking 按 booking_id 索引断裂 ID。
	breaksByBooking map[string]string
	// receiptsSeen 已处理 PSP 收据 ID 集合，防重复入账。
	// receipt_id → break_id（若为空字符串表示该 receipt 已处理但无断裂）。
	receiptsSeen map[string]string
	// bookingFirstReceipt 每个预约第一次处理的收据 ID（用于检测后续重复收据）。
	bookingFirstReceipt map[string]string // booking_id → first_receipt_id
	// suspense 挂账子服务（组合）。
	suspense *SuspenseService
	// proof 双边证明子服务（组合）。
	proof *ProofService
	// rail 轨道预约服务（只读引用；本包绝不修改 railGraph）。
	rail *fiatrail.Service
	// policies SLA 策略列表。
	policies []*AgingPolicy
	// clock 可注入时钟，使用 time.Now.UTC。
	clock func() time.Time
}

// NewBreakService 创建断裂服务实例。
   func NewBreakService(rail *fiatrail.Service) *BreakService {
	  svc := &BreakService{
		  breaks:          map[string]*RailBreak{},
		   breaksByBooking: map[string]string{},
		  receiptsSeen:    map[string]string{},
		   bookingFirstReceipt: map[string]string{},
		  suspense:            NewSuspenseService(),
		   proof:               NewProofService(),
		   rail:                rail,
		  clock: func() time.Time { return time.Now().UTC() },
	   }
	  svc.loadDefaultPolicies()
	  return svc
   }

// loadDefaultPolicies 装载默认 SLA 老化策略。
// 可通过配置覆盖。
   func (s *BreakService) loadDefaultPolicies() {
	  s.policies = []*AgingPolicy{
		   {
			   ID: "default-underpay",
			   BreakKind:    string(BreakUnderpay),
			  MaxAgeSec:    int64(72 * time.Hour / time.Second), // 72 小时
			  EscalateDesk: "break_desk",
		   },
		  {
			  ID: "default-overpay",
			  BreakKind: string(BreakOverpay),
			  MaxAgeSec: int64(48 * time.Hour / time.Second), // 48 小时
			   EscalateDesk: "finance_desk",
		   },
		  {
			  ID: "default-duplicate",
			  BreakKind:    string(BreakDuplicate),
			   MaxAgeSec:    int64(24 * time.Hour / time.Second),
			   EscalateDesk: "break_desk",
			  AutoWriteOff: true,
			   WriteOffThreshold: "1.00", // 差额≤1 USD 自动核销
		   },
		  {
			   ID:           "default-mid-refund",
			   BreakKind:    string(BreakMidRefund),
			   MaxAgeSec:    int64(48 * time.Hour / time.Second),
			  EscalateDesk: "break_desk",
		  },
		   {
			  ID: "default-status-mismatch",
			   BreakKind: string(BreakStatusMismatch),
			   MaxAgeSec:    int64(24 * time.Hour / time.Second),
			   EscalateDesk: "finance_desk",
		  },
	   }
   }

// Suspense 返回挂账子服务（供 HTTP 层访问）。
  func (s *BreakService) Suspense() *SuspenseService { return s.suspense }

// Proof 返回双边证明子服务（供 HTTP 层访问）。

  func (s *BreakService) Proof() *ProofService { return s.proof }

// now 返回当前时间（可注入）。

  func (s *BreakService) now() time.Time {
	  if s == nil || s.clock == nil {
		   return time.Now().UTC()
	   }
	   return s.clock()
  }

// ─────────────────────────────────────────────────────────
// 核心对账检测
// ─────────────────────────────────────────────────────────
// Detect 对账检测：将 PSP 收据与轨道账本核对，生成断裂记录与挂账。
// 重复 PSP 收据不二次入账；断裂记录保存重复凭证。
  func (s *BreakService) Detect(ctx context.Context, req DetectReq) (DetectResult, error) {
	   if s == nil {
		  return DetectResult{}, ErrUnavailable
	   }
	   // 清洗入参
	  bookingID := strings.TrimSpace(req.BookingID)
	   if bookingID == "" {
		   return DetectResult{}, ErrBooking
	  }
	  receiptID := strings.TrimSpace(req.PSPReceiptID)
	   if receiptID == "" {
		   return DetectResult{}, fmt.Errorf("%w: psp_receipt_id required", ErrAmount)
	  }
	   currency := strings.ToUpper(strings.TrimSpace(req.Currency))
	   if currency == "" {
		  currency = "USD"
	   }
	   pspAmt, err := decimal.NewFromString(strings.TrimSpace(req.PSPAmount))
	   if err != nil || pspAmt.IsNegative() {
		   return DetectResult{}, fmt.Errorf("%w: psp_amount", ErrAmount)
	  }
	   // 检查重复收据（同一 receipt_id 再次到达）
	   s.mu.Lock()
	  if existingBreakID, seen := s.receiptsSeen[receiptID]; seen {
		   // 同一 receipt_id 再次到达：幂等返回
		   if existingBreakID == "" {
			   // 首次是无断裂的干净收据，再次到达直接返回无断裂
			  s.mu.Unlock()
			   return DetectResult{HasBreak: false}, nil
		  }
		  br := s.breaks[existingBreakID]
		   if br != nil {
			   result := s.appendDuplicateEvidence(br, req)
			  s.mu.Unlock()
			   return result, nil
		  }
		  s.mu.Unlock()
		   return DetectResult{}, ErrDuplicate
	  }
	  // 检查同一预约是否已有另一张收据被处理过（不同 receipt_id → duplicate）
	  firstReceiptID, bookingHasReceipt := s.bookingFirstReceipt[bookingID]
	   s.mu.Unlock()
	   if bookingHasReceipt && firstReceiptID != receiptID {
		  // 同一预约已有收据处理记录，新收据是重复收据 → BreakDuplicate
		  // 先查是否已有断裂记录
		   s.mu.RLock()
		   existBreakID, hasBr := s.breaksByBooking[bookingID]
		  s.mu.RUnlock()
		   now := s.now()
		   if hasBr {
			  // 已有断裂，升级为 duplicate 并追加证据
			   s.mu.Lock()
			   br := s.breaks[existBreakID]
			   if br != nil {
				   br.Kind = BreakDuplicate
				   br.Evidence = mergeEvidence(br.Evidence, req.Evidence, receiptID)
				  br.UpdatedAt = now
				   s.receiptsSeen[receiptID] = existBreakID
			  }
			   s.mu.Unlock()
			  if br != nil {
				  brc := br.copy()
				   return DetectResult{HasBreak: true, Break: &brc}, nil
			   }
		  }
		   // 无现有断裂，创建新的 Duplicate 断裂
		   // 重新获取账本预约（需用于构建断裂记录）
		   if s.rail == nil {
			  return DetectResult{}, ErrUnavailable
		   }
		  booking, berr := s.rail.Get(bookingID)
		   if berr != nil {
			   return DetectResult{}, fmt.Errorf("%w: %v", ErrBooking, berr)
		  }
		   bookAmtDup,
			  _ := decimal.NewFromString(booking.Amount)
		  dupBreak := &RailBreak{
			   ID:        uuid.NewString(),
			   BookingID: bookingID,
			  Kind:      BreakDuplicate,
			   State:     BreakOpen,
			   PSPReceiptID: firstReceiptID + "+" + receiptID,
			   PSPAmount:    pspAmt.StringFixed(8),
			   BookAmount:   bookAmtDup.StringFixed(8),
			  Delta:    decimal.Zero.StringFixed(8),
			   Currency: currency,
			  Evidence:  mergeEvidence(buildEvidence("", firstReceiptID), req.Evidence, receiptID),
			   CreatedAt: now,
			   UpdatedAt: now,
		  }
		  s.mu.Lock()
		   s.breaks[dupBreak.ID] = dupBreak
		   s.breaksByBooking[bookingID] = dupBreak.ID
		  s.receiptsSeen[receiptID] = dupBreak.ID
		   s.mu.Unlock()
		  metrics.FiatBreak.WithLabelValues(string(BreakDuplicate), string(BreakOpen)).Inc()
		   brc := dupBreak.copy()
		   return DetectResult{HasBreak: true, Break: &brc}, nil
	  }
	  // 查询轨道预约
	   if s.rail == nil {
		  return DetectResult{},
			   ErrUnavailable
	  }
	  booking, err := s.rail.Get(bookingID)
	   if err != nil {
		   return DetectResult{}, fmt.Errorf("%w: %v", ErrBooking, err)
	  }
	   bookAmt, err := decimal.NewFromString(booking.Amount)
	   if err != nil {
		  return DetectResult{}, fmt.Errorf("%w: book amount parse", ErrAmount)
	   }
	  // 计算差额（PSP - Book）
	  delta := pspAmt.Sub(bookAmt)
	   // 确定断裂类型
	  kind, err := s.classifyBreak(booking, pspAmt, bookAmt, req.PSPStatus)
	  if err != nil {
		   return DetectResult{}, err
	  }
	   // 无差异且状态一致 → 生成双边证明，无断裂
	  if kind == "" {
		   proof, perr := s.proof.Generate(ctx, ProofInput{
			  BookingID: bookingID,
			   PSPReceiptID: receiptID,
			   PSPAmount:    pspAmt.String(),
			   BookAmount: bookAmt.String(),
			   LedgerID:   booking.LedgerID,
			  Currency:   currency,
		   })
		  if perr != nil {
			   return DetectResult{}, perr
		  }
		   // 记录该收据已处理（无断裂），并标记该预约的首条收据
		   s.mu.Lock()
		   s.receiptsSeen[receiptID] = "" // 空字符串表示无断裂
		   if _, already := s.bookingFirstReceipt[bookingID]; !already {
			  s.bookingFirstReceipt[bookingID] = receiptID
		   }
		  s.mu.Unlock()
		  return DetectResult{HasBreak: false, Proof: &proof}, nil
	   }
	  now := s.now()
	  // 检查该预约是否已有断裂记录（不允许同一预约重复开裂）
	   s.mu.Lock()
	   existingID,
		   hasExisting := s.breaksByBooking[bookingID]
	  var existingBreak *RailBreak
	  if hasExisting {
		   existingBreak = s.breaks[existingID]
	   }
	   s.mu.Unlock()
	  if hasExisting && existingBreak != nil {
		   // 已有断裂，追加证据
		   s.mu.Lock()
		   s.receiptsSeen[receiptID] = existingID
		   existingBreak.Evidence = mergeEvidence(existingBreak.Evidence,
			  req.Evidence, receiptID)
		   existingBreak.UpdatedAt = now
		  s.mu.Unlock()
		   br := existingBreak.copy()
		   return DetectResult{HasBreak: true, Break: &br}, nil
	   }
	   // 创建新断裂记录
	   br := &RailBreak{
		   ID:        uuid.NewString(),
		  BookingID: bookingID,
		  Kind: kind,
		   State: BreakOpen,
		   PSPReceiptID: receiptID,
		  PSPAmount:  pspAmt.StringFixed(8),
		  BookAmount: bookAmt.StringFixed(8),
		   Delta:      delta.StringFixed(8),
		   Currency:   currency,
		   Evidence: buildEvidence(req.Evidence,
			  receiptID),
		  CreatedAt: now,
		  UpdatedAt: now,
	   }
	   // 创建挂账条目
	   slaDeadline := s.slaDeadlineFor(kind, now)
	   suspReason := suspenseReasonFor(kind)
	  susp, serr := s.suspense.Create(ctx, SuspenseCreateReq{
		   BreakID: br.ID,
		   BookingID: bookingID,
		   Reason:    suspReason,
		  Asset:     booking.Asset,
		  Amount:    absDecimal(delta).StringFixed(8),
		   Currency: currency,
		   SLADeadline: slaDeadline,
	   })
	   if serr != nil {
		  return DetectResult{}, serr
	   }
	  br.SuspenseID = susp.ID
	  s.mu.Lock()
	  s.breaks[br.ID] = br
	  s.breaksByBooking[bookingID] = br.ID
	  s.receiptsSeen[receiptID] = br.ID
	  if _, already := s.bookingFirstReceipt[bookingID]; !already {
		   s.bookingFirstReceipt[bookingID] = receiptID
	   }
	   s.mu.Unlock()
	  // 生成双边证明
	   proof, perr := s.proof.Generate(ctx, ProofInput{
		  BookingID: bookingID,
		   BreakID:      br.ID,
		   PSPReceiptID: receiptID,
		   PSPAmount:    pspAmt.String(),
		  BookAmount: bookAmt.String(),
		   LedgerID: booking.LedgerID,
		  Currency: currency,
	   })
	   if perr != nil {
		  // 证明生成失败不阻断断裂创建，仅告警
		   _ = perr
	   }
	  metrics.FiatBreak.WithLabelValues(string(kind), string(BreakOpen)).Inc()
	   brCopy := br.copy()
	   suspCopy := susp
	  var proofPtr *BilateralProof
	  if perr == nil {
		   proofPtr = &proof
	   }
	   return DetectResult{HasBreak: true, Break: &brCopy, Suspense: &suspCopy, Proof: proofPtr}, nil
   }

// classifyBreak 根据金额差异和 PSP 状态确定断裂类型。
// 返回空字符串表示无断裂。
  func (s *BreakService) classifyBreak(
	  booking fiatrail.Booking,
	   pspAmt, bookAmt decimal.Decimal,
	  pspStatus string,
   ) (BreakKind, error) {
	  delta := pspAmt.Sub(bookAmt)
	   zero := decimal.Zero
	  // 状态不符检查（PSP failed 但 booking settled）
	   pspStatus = strings.ToLower(strings.TrimSpace(pspStatus))
	  if pspStatus == "failed" || pspStatus == "declined" {
		  if booking.State == fiatrail.StateSettled {
			  return BreakStatusMismatch, nil
		  }
	  }
	   // PSP refunded 但 booking 未退款
	   if pspStatus == "refunded" {
		   if booking.State != fiatrail.StateRefunded {
			   return BreakMidRefund, nil
		  }
		   // 退款金额也需检查
		   if !delta.Equal(zero) {
			  if delta.IsNegative() {
				  return BreakUnderpay, nil
			   }
			   return BreakOverpay, nil
		  }
		  return "", nil
	   }
	   // 金额差异
	   cmp := delta.Sign()
	  switch {
	   case cmp < 0:
		   return BreakUnderpay, nil
	  case cmp > 0:
		  return BreakOverpay, nil
	   default:
		  return "", nil
	  }
   }

// appendDuplicateEvidence 对已有断裂追加重复收据证据。
   func (s *BreakService) appendDuplicateEvidence(br *RailBreak, req DetectReq) DetectResult {
	  now := s.now()
	   // 若原断裂不是 Duplicate 类型，升级为 Duplicate
	   if br.Kind != BreakDuplicate {
		   br.Kind = BreakDuplicate
	  }
	  br.Evidence = mergeEvidence(br.Evidence, req.Evidence, req.PSPReceiptID)
	  br.UpdatedAt = now
	   metrics.FiatBreak.WithLabelValues(string(BreakDuplicate), "dup_receipt").Inc()
	  brCopy := br.copy()
	   return DetectResult{HasBreak: true, Break: &brCopy}
  }

// ─────────────────────────────────────────────────────────
// 断裂生命周期
// ─────────────────────────────────────────────────────────
// Get 按 ID 获取断裂记录。

  func (s *BreakService) Get(id string) (RailBreak, error) {
	  if s == nil {
		  return RailBreak{}, ErrUnavailable
	  }
	  id = strings.TrimSpace(id)
	   s.mu.RLock()
	   br, ok := s.breaks[id]
	  s.mu.RUnlock()
	   if !ok {
		   return RailBreak{}, ErrNotFound
	   }
	  return br.copy(), nil
   }

// GetByBooking 按预约 ID 获取断裂记录。

  func (s *BreakService) GetByBooking(bookingID string) (RailBreak, error) {
	  if s == nil {
		  return RailBreak{}, ErrUnavailable
	   }
	  bookingID = strings.TrimSpace(bookingID)
	  s.mu.RLock()
	   id, ok := s.breaksByBooking[bookingID]
	  if !ok {
		   s.mu.RUnlock()
		  return RailBreak{}, ErrNoBreak
	   }
	  br := s.breaks[id]
	   s.mu.RUnlock()
	  if br == nil {
		   return RailBreak{}, ErrNotFound
	  }
	  return br.copy(), nil
   }

// List 列举断裂记录（按状态/类型过滤；limit≤0 默认 50）。
   func (s *BreakService) List(filterState BreakState, filterKind BreakKind, limit int) []RailBreak {
	  if s == nil {
		  return nil
	  }
	  if limit <= 0 {
		  limit = 50
	  }
	   s.mu.RLock()
	   defer s.mu.RUnlock()
	   out := make([]RailBreak, 0, limit)
	  for _, br := range s.breaks {
		   if filterState != "" && br.State != filterState {
			  continue
		  }
		   if filterKind != "" && br.Kind != filterKind {
			  continue
		   }
		  out = append(out, br.copy())
		   if len(out) >= limit {
			  break
		  }
	  }
	   return out
   }

// Investigate 断裂状态转为 investigating，分配专项桌面。

   func (s *BreakService) Investigate(id, assignedTo, actor string) (RailBreak, error) {
	   if s == nil {
		  return RailBreak{}, ErrUnavailable
	   }
	  if _, err := validate.Actor(actor); err != nil {
		  return RailBreak{}, ErrActor
	  }
	  id = strings.TrimSpace(id)
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   br, ok := s.breaks[id]
	   if !ok {
		   return RailBreak{}, ErrNotFound
	   }
	   if err := CanAdvanceBreak(br.State, BreakInvestigating); err != nil {
		  return br.copy(), err
	  }
	   br.State = BreakInvestigating
	  br.AssignedTo = strings.TrimSpace(assignedTo)
	   br.UpdatedAt = s.now()
	  metrics.FiatBreak.WithLabelValues(string(br.Kind),
		  string(BreakInvestigating)).Inc()
	   return br.copy(), nil
   }

// Resolve 断裂处置为 resolved（差额已平账）。

  func (s *BreakService) Resolve(ctx context.Context, id, actor, note string) (RailBreak, error) {
	  if s == nil {
		   return RailBreak{}, ErrUnavailable
	   }
	  if _, err := validate.Actor(actor); err != nil {
		  return RailBreak{}, ErrActor
	  }
	  note = strings.TrimSpace(note)
	   s.mu.Lock()
	  br, ok := s.breaks[id]
	  if !ok {
		  s.mu.Unlock()
		   return RailBreak{}, ErrNotFound
	  }
	   if err := CanAdvanceBreak(br.State, BreakResolved); err != nil {
		   s.mu.Unlock()
		   return br.copy(), err
	  }
	  br.State = BreakResolved
	   br.ResolvedBy = actor
	   br.ResolveNote = note
	  br.UpdatedAt = s.now()
	   suspID := br.SuspenseID
	  s.mu.Unlock()
	  // 关联挂账消费
	   if suspID != "" {
		   if _, err := s.suspense.Consume(ctx, suspID, actor, "break resolved: "+note); err != nil {
			  // 挂账消费失败不回滚断裂状态，但记录到 ResolveNote
			  s.mu.Lock()
			  br.ResolveNote = note + " [suspense consume failed: " + err.Error() + "]"
			   s.mu.Unlock()
		  }
	   }
	  metrics.FiatBreak.WithLabelValues(string(br.Kind), string(BreakResolved)).Inc()
	  s.mu.RLock()
	   result := br.copy()
	   s.mu.RUnlock()
	  return result, nil
  }

// WriteOff 断裂核销为 written_off（差额商业决策销账）。

   func (s *BreakService) WriteOff(ctx context.Context, id, actor, note string) (RailBreak, error) {
	  if s == nil {
		   return RailBreak{}, ErrUnavailable
	   }
	  if _, err := validate.Actor(actor); err != nil {
		   return RailBreak{}, ErrActor
	  }
	  note = strings.TrimSpace(note)
	  s.mu.Lock()
	  br, ok := s.breaks[id]
	  if !ok {
		   s.mu.Unlock()
		   return RailBreak{}, ErrNotFound
	   }
	  if err := CanAdvanceBreak(br.State, BreakWrittenOff); err != nil {
		   s.mu.Unlock()
		   return br.copy(), err
	  }
	  br.State = BreakWrittenOff
	  br.ResolvedBy = actor
	   br.ResolveNote = note
	   br.UpdatedAt = s.now()
	   suspID := br.SuspenseID
	   s.mu.Unlock()
	   // 关联挂账释放
	  if suspID != "" {
		   if _, err := s.suspense.Release(ctx, suspID, actor, "break written_off: "+note); err != nil {
			   s.mu.Lock()
			  br.ResolveNote = note + " [suspense release failed: " + err.Error() + "]"
			  s.mu.Unlock()
		   }
	   }
	   metrics.FiatBreak.WithLabelValues(string(br.Kind), string(BreakWrittenOff)).Inc()
	  s.mu.RLock()
	  result := br.copy()
	   s.mu.RUnlock()
	  return result, nil
  }

// ─────────────────────────────────────────────────────────
// 辅助函数
// ─────────────────────────────────────────────────────────
// slaDeadlineFor 按断裂类型查找对应 SLA 策略，计算截止时间。

  func (s *BreakService) slaDeadlineFor(kind BreakKind, from time.Time) time.Time {
	  if s == nil {
		  return from.Add(72 * time.Hour)
	   }
	  for _, p := range s.policies {
		   if p.BreakKind == string(kind) {
			  return from.Add(time.Duration(p.MaxAgeSec) * time.Second)
		   }
	  }
	   return from.Add(72 * time.Hour)
   }

// suspenseReasonFor 断裂类型映射挂账原因。

  func suspenseReasonFor(kind BreakKind) SuspenseReason {
	  switch kind {
	   case BreakUnderpay, BreakOverpay:
		  return SuspenseSettleDrift
	  case BreakDuplicate:
		   return SuspenseDuplicate
	   case BreakMidRefund:
		   return SuspenseMidRefund
	   case BreakStatusMismatch:
		   return SuspenseAuthorizedUnsettled
	  default:
		  return SuspenseSettleDrift
	   }
  }

// absDecimal 取 decimal 绝对值。

  func absDecimal(d decimal.Decimal) decimal.Decimal {
	   if d.IsNegative() {
		  return d.Neg()
	   }
	   return d
  }

// buildEvidence 构建初始证据 JSON 字符串。
  func buildEvidence(raw, receiptID string) string {
	   if raw == "" {
		  return fmt.Sprintf(`{"receipt_ids":[%q]}`, receiptID)
	  }
	   return fmt.Sprintf(`{"receipt_ids":[%q],"raw":%s}`, receiptID, raw)
   }

// mergeEvidence 将新收据 ID 追加到现有证据 JSON。

   func mergeEvidence(existing, newRaw, newReceiptID string) string {
	   if existing == "" {
		  return buildEvidence(newRaw, newReceiptID)
	  }
	  // 简单追加；应做 JSON merge
	  return existing + fmt.Sprintf(`/*dup:%s*/`, newReceiptID)
   }

// copy 深拷贝（RailBreak 无指针字段，值拷贝即安全）。
   func (b *RailBreak) copy() RailBreak {
	  if b == nil {
		   return RailBreak{}
	  }
	  return *b
  }

// Policies 返回当前 SLA 策略列表（只读）。
   func (s *BreakService) Policies() []AgingPolicy {
	  if s == nil {
		   return nil
	   }
	   out := make([]AgingPolicy, 0, len(s.policies))
	  for _, p := range s.policies {
		  out = append(out, *p)
	  }
	  return out
   }

// SetPolicies 替换 SLA 策略（线程安全）。

   func (s *BreakService) SetPolicies(policies []AgingPolicy) error {
	   if s == nil {
		   return ErrUnavailable
	   }
	   for _, p := range policies {
		  if p.MaxAgeSec <= 0 {
			   return fmt.Errorf("%w: max_age_sec must be positive", ErrPolicy)
		   }
		  if p.EscalateDesk == "" {
			  return fmt.Errorf("%w: escalate_desk required", ErrPolicy)
		  }
	   }
	  s.mu.Lock()
	   s.policies = make([]*AgingPolicy, len(policies))
	  for i := range policies {
		   cp := policies[i]
		  s.policies[i] = &cp
	   }
	  s.mu.Unlock()
	   return nil
   }
