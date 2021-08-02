package isodebt

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/isobook"
)

// GenerationManager 管理 PairGeneration 世代递增与 DebtRatio 重算。

type GenerationManager struct {
	store Store
	book  *isobook.Book
	clock func() time.Time
}

// NewGenerationManager 构造世代管理器。
   func NewGenerationManager(store Store, book *isobook.Book) *GenerationManager {
	   return &GenerationManager{
		  store: store,
		   book:  book,
		  clock: func() time.Time { return time.Now().UTC() },
	  }
  }

// WithClock 注入时钟。
   func (m *GenerationManager) WithClock(c func() time.Time) *GenerationManager {
	   if m != nil && c != nil {
		  m.clock = c
	   }
	  return m
   }

  // BumpOnCredit 在 Credit 操作改变 equity 后调用；递增世代并重算 DebtRatio。
   // sourceTxnID 为本次 Credit 交易标识。
  func (m *GenerationManager) BumpOnCredit(ctx context.Context, userID, symbol, sourceTxnID string) (PairGeneration, error) {
	  return m.bump(ctx, userID, symbol, "credit", sourceTxnID)
  }

// BumpOnDebit 在 Debit 操作改变 debt 后调用；递增世代并重算 DebtRatio。
// sourceTxnID 为本次 Debit 交易标识。
  func (m *GenerationManager) BumpOnDebit(ctx context.Context, userID, symbol, sourceTxnID string) (PairGeneration, error) {
	  return m.bump(ctx, userID, symbol, "debit", sourceTxnID)
   }

// Recompute 主动重算 DebtRatio，不递增世代（仅更新比率快照）。
   func (m *GenerationManager) Recompute(ctx context.Context, userID, symbol string) (PairGeneration, error) {
	   current, err := m.store.GetGeneration(ctx, userID, symbol)
	   if err != nil {
		  return PairGeneration{}, fmt.Errorf("isodebt: get generation: %w", err)
	   }
	   debt, equity, ratio := m.computeFromBook(userID, symbol)
	  current.DebtRatio = ratio
	  current.TotalDebt = debt
	   current.TotalEquity = equity
	   current.ComputedAt = m.clock()
	  current.TriggeredBy = "recompute"
	  if err := m.store.SaveGeneration(ctx, current); err != nil {
		  return PairGeneration{}, fmt.Errorf("isodebt: save generation: %w", err)
	  }
	   return current, nil
   }

// CurrentGeneration 返回 userID+symbol 的当前世代号（不存在时为 0）。

   func (m *GenerationManager) CurrentGeneration(ctx context.Context, userID, symbol string) (int64, error) {
	  g, err := m.store.GetGeneration(ctx, userID, symbol)
	  if err != nil {
		   return 0, err
	  }
	   return g.Generation, nil
  }

// bump 递增世代、重算 DebtRatio 并持久化。
  func (m *GenerationManager) bump(ctx context.Context, userID, symbol, trigger, sourceTxnID string) (PairGeneration, error) {
	   userID = strings.TrimSpace(userID)
	  symbol = strings.ToUpper(strings.TrimSpace(symbol))
	  if userID == "" || symbol == "" {
		   return PairGeneration{}, fmt.Errorf("%w: userID and symbol required", ErrInvalidInput)
	  }
	  current, err := m.store.GetGeneration(ctx, userID, symbol)
	   if err != nil {
		   return PairGeneration{}, fmt.Errorf("isodebt: get generation: %w", err)
	   }
	   debt, equity, ratio := m.computeFromBook(userID, symbol)
	  next := PairGeneration{
		  ID: uuid.NewString(),
		  UserID: userID,
		   Symbol:     symbol,
		   Generation: current.Generation + 1,
		   DebtRatio: ratio,
		  TotalDebt:   debt,
		   TotalEquity: equity,
		   ComputedAt:  m.clock(),
		   TriggeredBy: trigger,
		   SourceTxnID: sourceTxnID,
	  }
	  if err := m.store.SaveGeneration(ctx, next); err != nil {
		  return PairGeneration{}, fmt.Errorf("isodebt: save generation: %w", err)
	   }
	   return next, nil
   }

// computeFromBook 从 isobook.Book 读取 Pair 状态，计算 totalDebt / totalEquity / DebtRatio。
// 采用 quote side liability 作为 debt，quote side total 作为 equity 代理值（与 marginhook LTV 计算相互独立）。

  func (m *GenerationManager) computeFromBook(userID, symbol string) (debt, equity, ratio decimal.Decimal) {
	   pairs := m.book.List(userID, symbol)
	  for _, p := range pairs {
		   if p.Symbol != strings.ToUpper(strings.TrimSpace(symbol)) {
			  continue
		   }
		   baseLiab, _ := decimal.NewFromString(p.Base.Liability)
		  quoteLiab,
			   _ := decimal.NewFromString(p.Quote.Liability)
		  baseTotal, _ := decimal.NewFromString(p.Base.Total)
		  quoteTotal, _ := decimal.NewFromString(p.Quote.Total)
		   debt = debt.Add(baseLiab).Add(quoteLiab)
		   equity = equity.Add(baseTotal).Add(quoteTotal)
	   }
	   ratio = ComputeRatio(debt, equity)
	  return
  }
