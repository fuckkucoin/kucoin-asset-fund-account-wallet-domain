package hotpregate

import (
	"context"
	"fmt"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"account-assets.kucoin.net/funding/internal/ledger"
	"github.com/shopspring/decimal"
)

// GuardedObserver 只读 Observe 封装器。
// 调用 Observe 时用 observeOnlyStore 包装底层 Store，防止在 observe 链路触发写操作。
type GuardedObserver struct {
	book *hotinv.Book
	store ledger.Store
}

// NewGuardedObserver 构造 GuardedObserver。

   func NewGuardedObserver(book *hotinv.Book, store ledger.Store) *GuardedObserver {
	   return &GuardedObserver{book: book, store: store}
   }

// Observe 只读观察指定资产的库存快照。返回目标资产的 Snapshot；资产不存在返回错误。
  func (g *GuardedObserver) Observe(ctx context.Context, asset string) (hotinv.Snapshot, error) {
	   if g == nil || g.book == nil {
		  return hotinv.Snapshot{}, ErrObserveFailed
	   }
	   wrapped := &observeOnlyStore{inner: g.store}
	  rows, err := g.book.Observe(ctx, wrapped)
	   if err != nil {
		   return hotinv.Snapshot{}, fmt.Errorf("%w: %v", ErrObserveFailed, err)
	  }
	  for _, r := range rows {
		  if r.Asset == asset {
			   return r, nil
		  }
	   }
	  return hotinv.Snapshot{}, fmt.Errorf("%w: asset %s not found in inventory", ErrObserveFailed, asset)
   }

// ObserveAll 只读观察全部资产快照。
  func (g *GuardedObserver) ObserveAll(ctx context.Context) ([]hotinv.Snapshot, error) {
	  if g == nil || g.book == nil {
		  return nil, ErrObserveFailed
	  }
	  wrapped := &observeOnlyStore{inner: g.store}
	   rows, err := g.book.Observe(ctx, wrapped)
	  if err != nil {
		   return nil, fmt.Errorf("%w: %v", ErrObserveFailed, err)
	  }
	  return rows, nil
  }

// ── observeOnlyStore ──────────────────────────────────────────────────────────
// observeOnlyStore 包装 ledger.Store；所有写方法均 panic，防止只读路径误触写操作。
// 这是编译期+运行期双重保护：
// - 编译期：observeOnlyStore 实现 ledger.Store，类型检查通过；
// - 运行期：若误调用写方法，panic 立即暴露问题（不静默失败）。

type observeOnlyStore struct {
	inner ledger.Store
}

// 只读方法：委托给 inner。

  func (s *observeOnlyStore) InTx(ctx context.Context, fn func(ctx context.Context) error) error {
	   return s.inner.InTx(ctx, fn)
   }

   func (s *observeOnlyStore) GetAccount(ctx context.Context, userID string, typ ledger.AccountType, asset string) (*ledger.Account, error) {
	   return s.inner.GetAccount(ctx, userID, typ, asset)
  }

   func (s *observeOnlyStore) ListUserBooks(ctx context.Context, userID string) ([]ledger.Account, error) {
	   return s.inner.ListUserBooks(ctx, userID)
  }

  func (s *observeOnlyStore) ListAccounts(ctx context.Context, limit int) ([]ledger.Account, error) {
	  return s.inner.ListAccounts(ctx, limit)
  }

  func (s *observeOnlyStore) GetTransfer(ctx context.Context, id string) (*ledger.Transfer, error) {
	  return s.inner.GetTransfer(ctx, id)
  }

  func (s *observeOnlyStore) GetByIdempotency(ctx context.Context, key string) (*ledger.Transfer, error) {
	   return s.inner.GetByIdempotency(ctx, key)
  }

   func (s *observeOnlyStore) ListJournal(ctx context.Context, transferID string) ([]ledger.JournalEntry, error) {
	   return s.inner.ListJournal(ctx, transferID)
  }

  func (s *observeOnlyStore) ListUserJournal(ctx context.Context, userID string, limit int) ([]ledger.JournalEntry, error) {
	   return s.inner.ListUserJournal(ctx,
		  userID, limit)
  }

  func (s *observeOnlyStore) SumDebitCredit(ctx context.Context, userID string, typ ledger.AccountType, asset string) (decimal.Decimal, decimal.Decimal, error) {
	  return s.inner.SumDebitCredit(ctx,
		  userID, typ, asset)
  }

   func (s *observeOnlyStore) SumToday(ctx context.Context, userID, asset string) (decimal.Decimal, error) {
	  return s.inner.SumToday(ctx, userID, asset)
  }

   func (s *observeOnlyStore) SumTodayByDirection(ctx context.Context, userID, asset string, dir ledger.Direction) (decimal.Decimal, error) {
	  return s.inner.SumTodayByDirection(ctx, userID, asset, dir)
   }

  func (s *observeOnlyStore) ListAudit(ctx context.Context, entityType, entityID string, limit int) ([]ledger.AuditEvent, error) {
	   return s.inner.ListAudit(ctx, entityType, entityID, limit)
  }

   func (s *observeOnlyStore) Allowlisted(ctx context.Context, userID, chain, addr string) (bool, error) {
	  return s.inner.Allowlisted(ctx, userID, chain, addr)
   }

   func (s *observeOnlyStore) ListAddressBook(ctx context.Context, userID string) ([]ledger.AddressBookEntry, error) {
	  return s.inner.ListAddressBook(ctx, userID)
   }

   func (s *observeOnlyStore) ListByState(ctx context.Context, st ledger.State, limit int) ([]*ledger.Transfer, error) {
	  return s.inner.ListByState(ctx, st, limit)
  }

  func (s *observeOnlyStore) ListUserTransfers(ctx context.Context, userID string, limit int) ([]*ledger.Transfer, error) {
	  return s.inner.ListUserTransfers(ctx, userID, limit)
  }

  func (s *observeOnlyStore) ListUserTransfersPaged(ctx context.Context, q ledger.DepositHistQuery) ([]*ledger.Transfer, int, error) {
	   return s.inner.ListUserTransfersPaged(ctx,
		   q)
   }

  func (s *observeOnlyStore) ListIncomingInner(ctx context.Context, userID string, limit int) ([]*ledger.Transfer, error) {
	  return s.inner.ListIncomingInner(ctx,
		  userID, limit)
  }

  func (s *observeOnlyStore) ListIncomingInnerPaged(ctx context.Context, q ledger.DepositHistQuery) ([]*ledger.Transfer, int, error) {
	  return s.inner.ListIncomingInnerPaged(ctx, q)
   }

   func (s *observeOnlyStore) ListUserDepositsMergedPaged(ctx context.Context, q ledger.DepositHistQuery) ([]*ledger.Transfer, int, error) {
	  return s.inner.ListUserDepositsMergedPaged(ctx, q)
  }

   func (s *observeOnlyStore) ListUserWithdrawsPaged(ctx context.Context, q ledger.WithdrawHistQuery) ([]*ledger.Transfer, int, error) {
	  return s.inner.ListUserWithdrawsPaged(ctx, q)
  }

  func (s *observeOnlyStore) FindByChainTxHash(ctx context.Context, chain, hash string, outputIndex int) (*ledger.Transfer, error) {
	  return s.inner.FindByChainTxHash(ctx, chain, hash, outputIndex)
  }

   func (s *observeOnlyStore) ListOutbox(ctx context.Context, unpublishedOnly bool, limit int) ([]ledger.OutboxEvent, error) {
	  return s.inner.ListOutbox(ctx, unpublishedOnly, limit)
   }

  func (s *observeOnlyStore) UserControlFrozen(ctx context.Context, userID string) (bool, error) {
	   return s.inner.UserControlFrozen(ctx, userID)
   }

  func (s *observeOnlyStore) UserKYC(ctx context.Context, userID string) (string, error) {
	  return s.inner.UserKYC(ctx, userID)
   }

// 禁止写操作：以下方法均 panic。

  func (s *observeOnlyStore) UpsertAccount(_ context.Context, _ *ledger.Account) error {
	   panic(ErrReadOnly.Error() + ": UpsertAccount called in precheck path")
  }

  func (s *observeOnlyStore) InsertTransfer(_ context.Context, _ *ledger.Transfer) error {
	   panic(ErrReadOnly.Error() + ": InsertTransfer called in precheck path")
   }

   func (s *observeOnlyStore) UpdateTransfer(_ context.Context, _ *ledger.Transfer) error {
	  panic(ErrReadOnly.Error() + ": UpdateTransfer called in precheck path")
   }

   func (s *observeOnlyStore) AppendJournal(_ context.Context, _ []ledger.JournalEntry) error {
	   panic(ErrReadOnly.Error() + ": AppendJournal called in precheck path")
  }

  func (s *observeOnlyStore) AppendAudit(_ context.Context, _ ledger.AuditEvent) error {
	   panic(ErrReadOnly.Error() + ": AppendAudit called in precheck path")
  }

   func (s *observeOnlyStore) UpsertAddressBook(_ context.Context, _ ledger.AddressBookEntry) error {
	  panic(ErrReadOnly.Error() + ": UpsertAddressBook called in precheck path")
   }

  func (s *observeOnlyStore) DeleteAddressBook(_ context.Context, _, _, _ string) error {
	   panic(ErrReadOnly.Error() + ": DeleteAddressBook called in precheck path")
  }

   func (s *observeOnlyStore) StampFence(_ context.Context, _, _ string) (int64, error) {
	  panic(ErrReadOnly.Error() + ": StampFence called in precheck path")
  }

   func (s *observeOnlyStore) AppendOutbox(_ context.Context, _ ledger.OutboxEvent) error {
	  panic(ErrReadOnly.Error() + ": AppendOutbox called in precheck path")
   }

   func (s *observeOnlyStore) MarkOutboxPublished(_ context.Context, _ []string) error {
	   panic(ErrReadOnly.Error() + ": MarkOutboxPublished called in precheck path")
  }

   func (s *observeOnlyStore) SetUserControlFrozen(_ context.Context, _ string, _ bool) error {
	  panic(ErrReadOnly.Error() + ": SetUserControlFrozen called in precheck path")
   }

  func (s *observeOnlyStore) SetUserKYC(_ context.Context, _, _ string) error {
	  panic(ErrReadOnly.Error() + ": SetUserKYC called in precheck path")
   }
