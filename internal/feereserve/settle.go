package feereserve

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/pkg/money"
)

// SettleState FeeSettle 状态机枚举。

type SettleState string

const (
	// SettleStatePending 待结算：广播已确认，等待对账写入。
	SettleStatePending SettleState = "pending"
	// SettleStateBalanced 平衡：实耗与预留误差在允许容差范围内，无补偿凭证。
	SettleStateBalanced SettleState = "balanced"
	// SettleStateOver 多退：实耗 < 预留，差额须退还用户。
	SettleStateOver SettleState = "over"
	// SettleStateShort 少补：实耗 > 预留，差额须追收用户。
	SettleStateShort SettleState = "short"
)

// FeeSettle 单次出金手续费结算记录。

type FeeSettle struct {
	// ID 结算记录唯一标识（UUID）。
	ID string `json:"id"`
	// ReserveID 关联的 FeeReserve ID。
	ReserveID string `json:"reserve_id"`
	// WithdrawID 出金单号（冗余字段，便于快速索引）。
	WithdrawID string `json:"withdraw_id"`
	// VersionID 关联的 QuoteVersion ID（可为空）。
	VersionID string `json:"version_id,omitempty"`
	// UserID 出金用户 ID。
	UserID string `json:"user_id"`
	// Asset 计价资产（用户资产，如 USDT/BTC）。
	Asset string `json:"asset"`
	// Chain 链名。
	Chain string `json:"chain"`
	// ReservedFee 预留费（来自 FeeReserve.Reserved）。
	ReservedFee string `json:"reserved_fee"`
	// ActualFee 链上实际手续费（广播确认后写入）。
	ActualFee string `json:"actual_fee"`
	// DeltaAmt 差额绝对值（|actual - reserved|）。
	DeltaAmt string `json:"delta_amt"`
	// DeltaBPS 差额相对预留费的 BPS（用于判断是否超出容差）。
	DeltaBPS int `json:"delta_bps"`
	// ToleranceBPS 本次结算允许的容差 BPS；来自 SettlePolicy。
	// 若 DeltaBPS ≤ ToleranceBPS，状态为 balanced。
	ToleranceBPS int `json:"tolerance_bps"`
	// State 结算状态。
	State SettleState `json:"state"`
	// JournalNote 账本凭证摘要说明（补偿凭证写入后填充）。
	JournalNote string `json:"journal_note,omitempty"`
	// LedgerCompensation 补偿凭证 ID（over/short 状态下写账本后填充）。
	LedgerCompensation string `json:"ledger_compensation,omitempty"`
	// Actor 操作人（广播确认 worker 的身份标识）。
	Actor string `json:"actor,omitempty"`
	// SettledAt 结算完成时间。
	SettledAt *time.Time `json:"settled_at,omitempty"`
	// CreatedAt 创建时间。
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 最后更新时间。
	UpdatedAt time.Time `json:"updated_at"`
}

// SettlePolicy 结算策略（tolerance + 自动补偿配置）。
type SettlePolicy struct {
	// ToleranceBPS 容差 BPS：DeltaBPS ≤ ToleranceBPS 时视为 balanced，不产生补偿凭证。
	// 典型值：50（0.5%）。
	ToleranceBPS int `json:"tolerance_bps"`
	// AutoCompensate 是否自动写入补偿凭证（over/short）。
	// false 时仅记录 FeeSettle，补偿由人工 ops 触发。
	AutoCompensate bool `json:"auto_compensate"`
}

// DefaultSettlePolicy 默认结算策略：50 BPS 容差，不自动补偿（人工审核）。
var DefaultSettlePolicy = SettlePolicy{
	ToleranceBPS: 50,
	AutoCompensate: false,
}

// computeSettleState 根据预留费、实际费和容差 BPS 判断结算状态。
// 返回（状态、差额绝对值、差额 BPS、error）。
  func computeSettleState(reserved, actual money.Amount, toleranceBPS int) (SettleState, money.Amount, int, error) {
	  cmp := reserved.Cmp(actual)
	   if cmp == 0 {
		  return SettleStateBalanced, money.Zero(), 0, nil
	   }
	  var delta money.Amount
	   var err error
	  if cmp > 0 {
		   delta, err = reserved.Sub(actual)
	   } else {
		  delta, err = actual.Sub(reserved)
	   }
	  if err != nil {
		  return SettleStatePending, money.Zero(), 0, err
	  }
	  // deltaBPS = delta / reserved × 10000
	  deltaBPS := 0
	  if !reserved.IsZero() {
		   bps := delta.Dec().Div(reserved.Dec()).Mul(decimal.NewFromInt(10000))
		   deltaBPS = int(bps.IntPart())
	   }
	  if deltaBPS <= toleranceBPS {
		  return SettleStateBalanced, delta, deltaBPS, nil
	  }
	  if cmp > 0 {
		  return SettleStateOver,
			   delta, deltaBPS, nil
	  }
	  return SettleStateShort,
		   delta, deltaBPS, nil
   }

// SettleStore FeeSettle 存储（SQL + 内存双模）。

type SettleStore struct {
	mu    sync.Mutex
	mem   map[string]*FeeSettle // withdraw_id → settle
	sqlDB *persist.SQLDB
	clock func() time.Time
}

// NewSettleStore 创建 SettleStore。

  func NewSettleStore(db *persist.SQLDB) *SettleStore {
	   return &SettleStore{
		   mem: map[string]*FeeSettle{},
		  sqlDB: db,
		  clock: func() time.Time { return time.Now().UTC() },
	  }
   }

// NewSettleStoreMem 纯内存模式（/嵌入场景）。

   func NewSettleStoreMem() *SettleStore { return NewSettleStore(nil) }

// WithClock 替换时钟函数（用）。
   func (ss *SettleStore) WithClock(c func() time.Time) *SettleStore {
	   if ss != nil && c != nil {
		  ss.clock = c
	   }
	  return ss
  }

  func (ss *SettleStore) now() time.Time {
	   if ss == nil || ss.clock == nil {
		  return time.Now().UTC()
	   }
	  return ss.clock()
   }

  func (ss *SettleStore) sqlOn() bool {
	  return ss != nil && ss.sqlDB != nil && ss.sqlDB.DB != nil
  }

// Create 从已结算的 FeeReserve 创建一条 FeeSettle 记录（状态 pending）。
// 幂等：相同 WithdrawID 已存在则返回已有记录。

  func (ss *SettleStore) Create(ctx context.Context, r FeeReserve, actor string) (FeeSettle, error) {
	  if ss == nil {
		   return FeeSettle{}, ErrUnavailable
	  }
	  if r.State != StateSettled {
		  return FeeSettle{}, fmt.Errorf("%w: reserve must be settled before creating settle record", ErrState)
	   }
	  // 幂等检查
	  if ss.sqlOn() {
		  if existing, err := ss.sqlGetByWithdraw(ctx, r.WithdrawID); err == nil {
			   return existing, nil
		   }
	  } else {
		  ss.mu.Lock()
		  if existing, ok := ss.mem[r.WithdrawID]; ok {
			  ss.mu.Unlock()
			  return *existing, nil
		  }
		  ss.mu.Unlock()
	   }
	  now := ss.now()
	   settle := FeeSettle{
		  ID:         uuid.NewString(),
		  ReserveID:  r.ID,
		   WithdrawID: r.WithdrawID,
		   VersionID:  r.VersionID,
		  UserID: r.UserID,
		   Asset:  r.Asset,
		   Chain:       r.Chain,
		  ReservedFee: r.Reserved,
		   ActualFee: r.Actual,
		  DeltaAmt: r.Delta,
		  State:    SettleStatePending,
		  Actor:     actor,
		  CreatedAt: now,
		  UpdatedAt: now,
	  }
	   if ss.sqlOn() {
		  return settle, ss.sqlCreate(ctx, settle)
	   }
	   ss.mu.Lock()
	   cp := settle
	   ss.mem[r.WithdrawID] = &cp
	  ss.mu.Unlock()
	   return settle, nil
   }

// Finalize 将 pending 的 FeeSettle 推进到最终状态（balanced/over/short）。
// policy 决定容差 BPS 和是否自动补偿。
  func (ss *SettleStore) Finalize(ctx context.Context, withdrawID string, policy SettlePolicy) (FeeSettle, error) {
	  if ss == nil {
		   return FeeSettle{}, ErrUnavailable
	   }
	   wid := strings.TrimSpace(withdrawID)
	  var s FeeSettle
	  if ss.sqlOn() {
		   got, err := ss.sqlGetByWithdraw(ctx, wid)
		  if err != nil {
			   return FeeSettle{}, ErrNotFound
		  }
		   s = got
	   } else {
		  ss.mu.Lock()
		   got, ok := ss.mem[wid]
		   if !ok {
			   ss.mu.Unlock()
			  return FeeSettle{}, ErrNotFound
		  }
		   s = *got
		  ss.mu.Unlock()
	  }
	   if s.State != SettleStatePending {
		  return s, nil // 已终态，幂等返回
	   }
	  reserved, err := money.ParseBook(s.ReservedFee)
	  if err != nil {
		   return FeeSettle{}, fmt.Errorf("%w: reserved_fee parse: %v", ErrAmount, err)
	   }
	  actual, err := money.ParseBook(s.ActualFee)
	  if err != nil {
		  return FeeSettle{}, fmt.Errorf("%w: actual_fee parse: %v", ErrAmount, err)
	  }
	   state, delta, deltaBPS, err := computeSettleState(reserved, actual, policy.ToleranceBPS)
	  if err != nil {
		   return FeeSettle{}, err
	   }
	   now := ss.now()
	   s.State = state
	  s.DeltaAmt = delta.String()
	   s.DeltaBPS = deltaBPS
	  s.ToleranceBPS = policy.ToleranceBPS
	   s.UpdatedAt = now
	   if state != SettleStatePending {
		   s.SettledAt = &now
	   }
	  if ss.sqlOn() {
		   if err := ss.sqlUpdate(ctx, s); err != nil {
			   return FeeSettle{}, err
		   }
	  } else {
		  ss.mu.Lock()
		  cp := s
		  ss.mem[wid] = &cp
		   ss.mu.Unlock()
	   }
	   return s, nil
  }

// GetByWithdraw 按出金单查询结算记录。
   func (ss *SettleStore) GetByWithdraw(ctx context.Context, withdrawID string) (FeeSettle, error) {
	  if ss == nil {
		  return FeeSettle{}, ErrUnavailable
	   }
	  wid := strings.TrimSpace(withdrawID)
	   if ss.sqlOn() {
		   return ss.sqlGetByWithdraw(ctx,
			   wid)
	   }
	  ss.mu.Lock()
	   defer ss.mu.Unlock()
	  s, ok := ss.mem[wid]
	   if !ok {
		  return FeeSettle{}, ErrNotFound
	  }
	   return *s, nil
   }

// ListByState 按状态列出结算记录（ops delta desk 用）。

   func (ss *SettleStore) ListByState(ctx context.Context, state SettleState, limit int) ([]FeeSettle, error) {
	   if ss == nil {
		   return nil, ErrUnavailable
	   }
	   if limit <= 0 {
		  limit = 100
	  }
	   if ss.sqlOn() {
		  return ss.sqlListByState(ctx, state, limit)
	   }
	   ss.mu.Lock()
	  defer ss.mu.Unlock()
	  var out []FeeSettle
	  for _, s := range ss.mem {
		   if s.State == state {
			   out = append(out, *s)
		   }
		  if len(out) >= limit {
			  break
		   }
	   }
	   sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	  return out, nil
  }

// MarkCompensated 记录补偿凭证 ID（over/short 凭证写入账本后调用）。
  func (ss *SettleStore) MarkCompensated(ctx context.Context, withdrawID, ledgerID, note string) (FeeSettle, error) {
	   if ss == nil {
		  return FeeSettle{}, ErrUnavailable
	  }
	  wid := strings.TrimSpace(withdrawID)
	   now := ss.now()
	  if ss.sqlOn() {
		  _, err := ss.sqlDB.Conn(ctx).ExecContext(ctx,
			  `UPDATE fee_settlements SET ledger_compensation=?, journal_note=?, updated_at=? WHERE withdraw_id=?`,
			  ledgerID, note, now, wid,
		  )
		   if err != nil {
			   return FeeSettle{}, err
		  }
		  return ss.sqlGetByWithdraw(ctx,
			  wid)
	   }
	  ss.mu.Lock()
	   defer ss.mu.Unlock()
	   s, ok := ss.mem[wid]
	   if !ok {
		  return FeeSettle{}, ErrNotFound
	  }
	  cp := *s
	  cp.LedgerCompensation = ledgerID
	  cp.JournalNote = note
	  cp.UpdatedAt = now
	   ss.mem[wid] = &cp
	   return cp, nil
   }

// --- SQL 实现 ---

   func (ss *SettleStore) sqlCreate(ctx context.Context, s FeeSettle) error {
	   _, err := ss.sqlDB.Conn(ctx).ExecContext(ctx,
		  `INSERT INTO fee_settlements
			(id, reserve_id, withdraw_id, version_id, user_id, asset, chain,
			 reserved_fee, actual_fee, delta_amt, delta_bps, tolerance_bps,
			 state, journal_note, ledger_compensation, actor, settled_at, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		   s.ID, s.ReserveID, s.WithdrawID, s.VersionID, s.UserID, s.Asset, s.Chain,
		  s.ReservedFee, s.ActualFee, s.DeltaAmt, s.DeltaBPS, s.ToleranceBPS,
		   string(s.State), s.JournalNote, s.LedgerCompensation, s.Actor,
		   s.SettledAt, s.CreatedAt, s.UpdatedAt,
	   )
	   return err
   }

   func (ss *SettleStore) sqlUpdate(ctx context.Context, s FeeSettle) error {
	   _, err := ss.sqlDB.Conn(ctx).ExecContext(ctx,
		   `UPDATE fee_settlements
		 SET state=?, delta_amt=?, delta_bps=?, tolerance_bps=?, settled_at=?, updated_at=?
		 WHERE withdraw_id=? AND state='pending'`,
		  string(s.State), s.DeltaAmt, s.DeltaBPS, s.ToleranceBPS, s.SettledAt, s.UpdatedAt, s.WithdrawID,
	   )
	  return err
   }

   func (ss *SettleStore) sqlGetByWithdraw(ctx context.Context, wid string) (FeeSettle, error) {
	   row := ss.sqlDB.Conn(ctx).QueryRowContext(ctx,
		   `SELECT id, reserve_id, withdraw_id, version_id, user_id, asset, chain, 
		        reserved_fee, actual_fee, delta_amt, delta_bps, tolerance_bps,
		        state, journal_note, ledger_compensation, actor, settled_at, created_at, updated_at
		 FROM fee_settlements WHERE withdraw_id=? ORDER BY created_at DESC LIMIT 1`, wid)
	  return scanSettleImpl(row)
  }

  func (ss *SettleStore) sqlListByState(ctx context.Context, state SettleState, limit int) ([]FeeSettle, error) {
	   rows, err := ss.sqlDB.Conn(ctx).QueryContext(ctx,
		  `SELECT id, reserve_id, withdraw_id, version_id, user_id, asset, chain,
		        reserved_fee, actual_fee, delta_amt, delta_bps, tolerance_bps,
		        state, journal_note, ledger_compensation, actor, settled_at, created_at, updated_at
		 FROM fee_settlements WHERE state=? ORDER BY created_at DESC LIMIT ?`,
		   string(state), limit)
	  if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	   var out []FeeSettle
	   for rows.Next() {
		   s, err := scanSettleImpl(rows)
		  if err != nil {
			  return nil, err
		   }
		   out = append(out, s)
	   }
	  return out, rows.Err()
  }

type settleScanner interface {
	Scan(dest ...any) error
}

   func scanSettleImpl(sc settleScanner) (FeeSettle, error) {
	  var s FeeSettle
	  var vidStr, journalNote, ledgerComp, actor sql.NullString
	   var settledAt sql.NullTime
	  var stateStr string
	   err := sc.Scan(
		  &s.ID, &s.ReserveID, &s.WithdrawID, &vidStr, &s.UserID, &s.Asset, &s.Chain,
		  &s.ReservedFee, &s.ActualFee, &s.DeltaAmt, &s.DeltaBPS, &s.ToleranceBPS,
		   &stateStr, &journalNote, &ledgerComp, &actor, &settledAt, &s.CreatedAt, &s.UpdatedAt,
	  )
	   if errors.Is(err, sql.ErrNoRows) {
		   return FeeSettle{}, ErrNotFound
	  }
	  if err != nil {
		  return FeeSettle{}, err
	   }
	   s.State = SettleState(stateStr)
	   if vidStr.Valid {
		  s.VersionID = vidStr.String
	  }
	  if journalNote.Valid {
		  s.JournalNote = journalNote.String
	   }
	  if ledgerComp.Valid {
		   s.LedgerCompensation = ledgerComp.String
	   }
	   if actor.Valid {
		   s.Actor = actor.String
	   }
	   if settledAt.Valid {
		  t := settledAt.Time
		   s.SettledAt = &t
	  }
	  return s, nil
  }
