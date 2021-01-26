package accrual

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/pkg/money"
)

// BatchStatus 应计批次状态。
type BatchStatus string

const (
	// BatchDraft 草稿，分录尚未全部入账。
	BatchDraft BatchStatus = "draft"
	// BatchPosted 已过账，分录全部落账。
	BatchPosted BatchStatus = "posted"
	// BatchReversed 已冲正，对应冲正批次存在。
	BatchReversed BatchStatus = "reversed"
)

// LineKind 应计分录科目类型。
type LineKind string

const (
	LineAccruedInterest LineKind = "accrued_interest" // 在途利息
	LineAccruedFee  LineKind = "accrued_fee"  // 在途费用
	LineNettingDiff LineKind = "netting_diff" // 轧差差额
	LineReversal    LineKind = "reversal"     // 冲正分录
)

// ErrBatchNotDraft 批次非 draft 状态，不可再追加分录。
var ErrBatchNotDraft = fmt.Errorf("accrual batch is not in draft status")

// ErrBatchImbalanced 批次借贷不平衡。
var ErrBatchImbalanced = fmt.Errorf("accrual batch is imbalanced: debit != credit")

// ErrBatchAlreadyPosted 批次已过账。

var ErrBatchAlreadyPosted = fmt.Errorf("accrual batch already posted")

// ErrReversalRef 冲正批次须引用有效的原批次。

var ErrReversalRef = fmt.Errorf("accrual reversal must reference a valid posted batch")

// ErrLineAmountInvalid 应计分录金额无效（须为正）。
var ErrLineAmountInvalid = fmt.Errorf("accrual line amount must be positive")

// AccrualBatch 跨日应计批次。
type AccrualBatch struct {
	ID          string `json:"id"`
	BusinessDay string `json:"business_day"` // YYYY-MM-DD，产生应计的营业日
	DayCloseID string `json:"day_close_id"` // 关联日切 ID
	OrigBatchID string `json:"orig_batch_id,omitempty"` // 冲正时引用原批次
	Status     BatchStatus `json:"status"`
	Version    int         `json:"version"` // 代次号，单调递增
	TotalDebit string      `json:"total_debit"`
	TotalCredit string `json:"total_credit"`
	Actor       string `json:"actor"`
	Note        string `json:"note,omitempty"`
	PostedAt   *time.Time    `json:"posted_at,omitempty"`
	ReversedAt *time.Time    `json:"reversed_at,omitempty"`
	Fence      persist.Fence `json:"fence"`
	CreatedAt time.Time `json:"created_at"`
}

// AccrualLine 应计批次下的单条分录（必须成对出现以保持平衡）。
type AccrualLine struct {
	ID string `json:"id"`
	BatchID string   `json:"batch_id"`
	Seq     int      `json:"seq"` // 批次内序号，从 1 开始
	Kind    LineKind `json:"kind"`
	UserID string `json:"user_id"`
	Asset      string `json:"asset"`
	Amount     string `json:"amount"`      // 正数，由 Direction 决定借贷方向
	Direction  string `json:"direction"`   // "debit" | "credit"
	CounterRef string `json:"counter_ref"` // 对应分录 ID（借记行引用贷记行，反之亦然）
	MovementID string `json:"movement_id,omitempty"` // 在途资金 movement 引用
	Note      string    `json:"note,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Engine 跨日应计引擎，单进程内存实现。
type Engine struct {
	mu      sync.Mutex
	batches map[string]*AccrualBatch
	lines  map[string][]*AccrualLine // key: batch_id
	seqMap map[string]int            // key: batch_id → next seq
	fence  *persist.Table
	clock  func() time.Time
}

// NewEngine 初始化应计引擎。
   func NewEngine() *Engine {
	  return &Engine{
		  batches: map[string]*AccrualBatch{},
		   lines:  map[string][]*AccrualLine{},
		   seqMap: map[string]int{},
		   fence:  persist.NewTable(),
		   clock: func() time.Time { return time.Now().UTC() },
	  }
  }

// WithClock 注入时钟（用）。

   func (e *Engine) WithClock(c func() time.Time) *Engine {
	  if e != nil && c != nil {
		  e.clock = c
	  }
	  return e
  }

   // CreateBatch 为指定营业日创建应计批次（draft 状态）。
  func (e *Engine) CreateBatch(businessDay, dayCloseID, actor, note string) (AccrualBatch, error) {
	   if e == nil {
		  return AccrualBatch{}, ErrEngineNil
	  }
	   businessDay = strings.TrimSpace(businessDay)
	   if _, err := time.Parse("2006-01-02", businessDay); err != nil {
		   return AccrualBatch{},
			  ErrInvalidDay
	  }
	  act := strings.TrimSpace(actor)
	   if act == "" {
		  return AccrualBatch{}, ErrActorRequired
	  }
	  now := e.clock()
	   bat := AccrualBatch{
		  ID:          uuid.NewString(),
		  BusinessDay: businessDay,
		   DayCloseID: strings.TrimSpace(dayCloseID),
		   Status: BatchDraft,
		   Version:     1,
		   TotalDebit:  "0",
		  TotalCredit: "0",
		  Actor: act,
		  Note: strings.TrimSpace(note),
		   CreatedAt: now,
	  }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	  f, err := e.fence.Bump(bat.ID, persist.NewToken())
	  if err != nil {
		  return AccrualBatch{}, err
	   }
	  bat.Fence = f
	   cp := bat
	   e.batches[bat.ID] = &cp
	   e.seqMap[bat.ID] = 1
	   return cp, nil
   }

// AddLinePair 向 draft 批次追加一对平衡分录（借记+贷记）。
// debitUserID/creditUserID 可以不同（跨账户应计）。
   func (e *Engine) AddLinePair(batchID, kind, debitUserID, creditUserID, asset, amount, movementID, note string) (AccrualLine, AccrualLine, error) {
	  if e == nil {
		   return AccrualLine{}, AccrualLine{}, ErrEngineNil
	  }
	  batchID = strings.TrimSpace(batchID)
	   amt, err := money.Parse(amount)
	  if err != nil || !amt.IsPositive() {
		  return AccrualLine{}, AccrualLine{}, ErrLineAmountInvalid
	   }
	   lk := LineKind(strings.TrimSpace(kind))
	   validKind := lk == LineAccruedInterest || lk == LineAccruedFee || lk == LineNettingDiff || lk == LineReversal
	  if !validKind {
		  return AccrualLine{}, AccrualLine{}, ErrInvalidKind
	   }
	   e.mu.Lock()
	   defer e.mu.Unlock()
	   bat, ok := e.batches[batchID]
	  if !ok {
		   return AccrualLine{}, AccrualLine{}, ErrBatchNotFound
	   }
	  if bat.Status != BatchDraft {
		  return AccrualLine{}, AccrualLine{}, ErrBatchNotDraft
	  }
	  now := e.clock()
	   seq := e.seqMap[batchID]
	  debitID := uuid.NewString()
	   creditID := uuid.NewString()
	  debit := AccrualLine{
		  ID: debitID,
		   BatchID: batchID,
		   Seq: seq,
		   Kind:   lk,
		  UserID: strings.TrimSpace(debitUserID),
		   Asset:  strings.TrimSpace(asset),
		  Amount: amount,
		   Direction: "debit",
		   CounterRef: creditID,
		  MovementID: strings.TrimSpace(movementID),
		  Note:       strings.TrimSpace(note),
		  CreatedAt:  now,
	  }
	  credit := AccrualLine{
		  ID:      creditID,
		   BatchID: batchID,
		   Seq:     seq + 1,
		  Kind: lk,
		  UserID: strings.TrimSpace(creditUserID),
		  Asset:     strings.TrimSpace(asset),
		   Amount:    amount,
		  Direction: "credit",
		  CounterRef: debitID,
		   MovementID: strings.TrimSpace(movementID),
		   Note:      strings.TrimSpace(note),
		  CreatedAt: now,
	   }
	  dcp, ccp := debit, credit
	  e.lines[batchID] = append(e.lines[batchID], &dcp, &ccp)
	  e.seqMap[batchID] = seq + 2
	  // 更新批次累计额
	   prevDebit,
		   _ := decimal.NewFromString(bat.TotalDebit)
	  prevCredit, _ := decimal.NewFromString(bat.TotalCredit)
	   amtDec := amt.Dec()
	  bat.TotalDebit = prevDebit.Add(amtDec).String()
	  bat.TotalCredit = prevCredit.Add(amtDec).String()
	  return dcp, ccp, nil
   }

// Post 将 draft 批次过账（校验借贷平衡）。
  func (e *Engine) Post(batchID, actor string, epoch int64, token string) (AccrualBatch, error) {
	  if e == nil {
		  return AccrualBatch{}, ErrEngineNil
	  }
	   act := strings.TrimSpace(actor)
	  if act == "" {
		  return AccrualBatch{}, ErrActorRequired
	   }
	   if err := e.fence.Check(batchID, epoch, token); err != nil {
		   return AccrualBatch{}, err
	   }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	   bat,
		  ok := e.batches[strings.TrimSpace(batchID)]
	   if !ok {
		  return AccrualBatch{}, ErrBatchNotFound
	  }
	   if bat.Status != BatchDraft {
		  return AccrualBatch{}, ErrBatchNotDraft
	   }
	  // 平衡校验
	  debit, _ := decimal.NewFromString(bat.TotalDebit)
	   credit,
		   _ := decimal.NewFromString(bat.TotalCredit)
	  if !debit.Equal(credit) {
		   return *bat, ErrBatchImbalanced
	   }
	  f, err := e.fence.Bump(batchID, persist.NewToken())
	   if err != nil {
		  return AccrualBatch{}, err
	   }
	   now := e.clock()
	   bat.Status = BatchPosted
	   bat.PostedAt = &now
	   bat.Actor = act
	  bat.Fence = f
	   bat.Version++
	   return *bat, nil
  }

// Reverse 对已过账的批次生成冲正批次。
// 冲正批次自动生成对应的 LineReversal 分录对。

   func (e *Engine) Reverse(origBatchID, actor, note string) (AccrualBatch, error) {
	  if e == nil {
		  return AccrualBatch{}, ErrEngineNil
	  }
	  act := strings.TrimSpace(actor)
	  if act == "" {
		  return AccrualBatch{}, ErrActorRequired
	   }
	  origBatchID = strings.TrimSpace(origBatchID)
	   e.mu.Lock()
	   defer e.mu.Unlock()
	   orig, ok := e.batches[origBatchID]
	  if !ok {
		   return AccrualBatch{}, ErrBatchNotFound
	  }
	  if orig.Status != BatchPosted {
		  return AccrualBatch{}, ErrReversalRef
	  }
	   now := e.clock()
	   revBat := AccrualBatch{
		  ID: uuid.NewString(),
		   BusinessDay: orig.BusinessDay,
		  DayCloseID:  orig.DayCloseID,
		   OrigBatchID: origBatchID,
		  Status: BatchDraft,
		  Version: 1,
		   TotalDebit:  "0",
		   TotalCredit: "0",
		   Actor:       act,
		   Note:        strings.TrimSpace(note),
		   CreatedAt: now,
	  }
	   f, err := e.fence.Bump(revBat.ID, persist.NewToken())
	  if err != nil {
		   return AccrualBatch{}, err
	  }
	   revBat.Fence = f
	   // 为每对原始分录生成反向冲正分录
	   origLines := e.lines[origBatchID]
	  revLines := make([]*AccrualLine, 0, len(origLines))
	   seq := 1
	   for i := 0; i+1 < len(origLines); i += 2 {
		   orig1 := origLines[i] // debit
		  orig2 := origLines[i+1] // credit
		  revCreditID := uuid.NewString()
		   revDebitID := uuid.NewString()
		  // 冲正：原借记变贷记
		   revCredit := &AccrualLine{
			   ID:      revCreditID,
			  BatchID: revBat.ID,
			   Seq: seq,
			   Kind:   LineReversal,
			   UserID: orig1.UserID,
			   Asset:  orig1.Asset,
			   Amount:     orig1.Amount,
			   Direction:  "credit",
			   CounterRef: revDebitID,
			   MovementID: orig1.MovementID,
			   Note: "reversal of " + orig1.ID,
			   CreatedAt: now,
		   }
		  // 冲正：原贷记变借记
		  revDebit := &AccrualLine{
			   ID: revDebitID,
			   BatchID: revBat.ID,
			  Seq:     seq + 1,
			   Kind:    LineReversal,
			  UserID: orig2.UserID,
			  Asset: orig2.Asset,
			  Amount:     orig2.Amount,
			   Direction:  "debit",
			  CounterRef: revCreditID,
			  MovementID: orig2.MovementID,
			  Note:       "reversal of " + orig2.ID,
			   CreatedAt:  now,
		  }
		   revLines = append(revLines, revCredit, revDebit)
		   seq += 2
		   // 累计
		  amtDec, _ := decimal.NewFromString(orig1.Amount)
		  prevD, _ := decimal.NewFromString(revBat.TotalDebit)
		   prevC, _ := decimal.NewFromString(revBat.TotalCredit)
		   revBat.TotalDebit = prevD.Add(amtDec).String()
		   revBat.TotalCredit = prevC.Add(amtDec).String()
	  }
	  cpBat := revBat
	   e.batches[revBat.ID] = &cpBat
	   e.lines[revBat.ID] = revLines
	  e.seqMap[revBat.ID] = seq
	  // 标记原批次为 reversed
	  orig.Status = BatchReversed
	   revAt := now
	  orig.ReversedAt = &revAt
	   orig.Version++
	   return cpBat, nil
  }

// GetBatch 按 ID 查应计批次。
   func (e *Engine) GetBatch(id string) (AccrualBatch, error) {
	   if e == nil {
		  return AccrualBatch{}, ErrEngineNil
	   }
	  e.mu.Lock()
	  defer e.mu.Unlock()
	   bat, ok := e.batches[strings.TrimSpace(id)]
	  if !ok {
		  return AccrualBatch{}, ErrBatchNotFound
	  }
	  return *bat, nil
  }

// ListLines 列出批次下的所有应计分录。

   func (e *Engine) ListLines(batchID string) []AccrualLine {
	   if e == nil {
		   return nil
	   }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	   raw := e.lines[strings.TrimSpace(batchID)]
	   out := make([]AccrualLine, 0, len(raw))
	   for _, l := range raw {
		  out = append(out, *l)
	   }
	  return out
  }

// ListBatches 列出所有批次。
  func (e *Engine) ListBatches() []AccrualBatch {
	  if e == nil {
		   return nil
	   }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	   out := make([]AccrualBatch, 0, len(e.batches))
	  for _, bat := range e.batches {
		  out = append(out, *bat)
	  }
	  return out
  }

// ListByDay 按营业日列出批次。

   func (e *Engine) ListByDay(businessDay string) []AccrualBatch {
	  if e == nil {
		   return nil
	  }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	  out := make([]AccrualBatch, 0)
	   for _, bat := range e.batches {
		   if bat.BusinessDay == strings.TrimSpace(businessDay) {
			  out = append(out, *bat)
		   }
	  }
	   return out
  }
