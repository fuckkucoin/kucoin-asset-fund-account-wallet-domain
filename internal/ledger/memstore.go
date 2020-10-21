// 内存账本，供 OpenMem 使用。
// 作者: Knox
// 日期: 2020.10.06
package ledger

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/wallet"
	"github.com/shopspring/decimal"
)

// Store 划转、流水、账本的持久化。AppendJournal 必须先 ValidateJournal；UpsertAccount 必须拒负数。
type Store interface {
	InTx(ctx context.Context, fn func(ctx context.Context) error) error
	   GetAccount(ctx context.Context, userID string, typ AccountType, asset string) (*Account, error)
	  UpsertAccount(ctx context.Context, a *Account) error
	   InsertTransfer(ctx context.Context, t *Transfer) error
	   GetTransfer(ctx context.Context, id string) (*Transfer, error)
	   GetByIdempotency(ctx context.Context,
		key string) (*Transfer, error)
	UpdateTransfer(ctx context.Context, t *Transfer) error
	AppendJournal(ctx context.Context, entries []JournalEntry) error
	ListJournal(ctx context.Context, transferID string) ([]JournalEntry, error)
	SumDebitCredit(ctx context.Context, userID string, typ AccountType, asset string) (debit, credit decimal.Decimal, err error)
	SumToday(ctx context.Context,
		userID, asset string) (decimal.Decimal, error)
	SumTodayByDirection(ctx context.Context, userID, asset string, dir Direction) (decimal.Decimal, error)
	AppendAudit(ctx context.Context,
		e AuditEvent) error
	ListAudit(ctx context.Context, entityType, entityID string, limit int) ([]AuditEvent, error)
	Allowlisted(ctx context.Context, userID, chain, addr string) (bool, error)
	ListAddressBook(ctx context.Context, userID string) ([]AddressBookEntry, error)
	UpsertAddressBook(ctx context.Context, e AddressBookEntry) error
	DeleteAddressBook(ctx context.Context, userID, chain, addr string) error
	ListUserBooks(ctx context.Context, userID string) ([]Account, error)
	ListByState(ctx context.Context, st State, limit int) ([]*Transfer, error)
	ListUserTransfers(ctx context.Context, userID string, limit int) ([]*Transfer, error)
	ListUserTransfersPaged(ctx context.Context, q DepositHistQuery) ([]*Transfer, int, error)
	ListIncomingInner(ctx context.Context, userID string, limit int) ([]*Transfer, error)
	ListIncomingInnerPaged(ctx context.Context, q DepositHistQuery) ([]*Transfer, int, error)
	ListUserDepositsMergedPaged(ctx context.Context,
		q DepositHistQuery) ([]*Transfer, int, error)
	ListUserWithdrawsPaged(ctx context.Context, q WithdrawHistQuery) ([]*Transfer, int, error)
	FindByChainTxHash(ctx context.Context, chain, hash string, outputIndex int) (*Transfer, error)
	StampFence(ctx context.Context, id, token string) (int64, error)
	AppendOutbox(ctx context.Context, e OutboxEvent) error
	ListOutbox(ctx context.Context, unpublishedOnly bool, limit int) ([]OutboxEvent, error)
	MarkOutboxPublished(ctx context.Context, ids []string) error
	ListUserJournal(ctx context.Context,
		userID string, limit int) ([]JournalEntry, error)
	UserControlFrozen(ctx context.Context, userID string) (bool, error)
	SetUserControlFrozen(ctx context.Context, userID string, frozen bool) error
	UserKYC(ctx context.Context, userID string) (string, error)
	SetUserKYC(ctx context.Context, userID, level string) error
	// ListAccounts 扫描科目余额，供热桥用户三方对账汇总负债。limit<=0 时由实现给上限。
	ListAccounts(ctx context.Context, limit int) ([]Account, error)
}

type MemStore struct {
	mu sync.Mutex
	txMu sync.Mutex
	accounts  map[string]*Account
	transfers map[string]*Transfer
	byIdem map[string]string
	journal []JournalEntry
	audit  []AuditEvent
	allow  map[string]AddressBookEntry
	outbox []OutboxEvent
	seed   map[string]decimal.Decimal // Seed 写入的无流水水位，对账允许账面=轧差+水位
	nextAccID int64
	frozenU   map[string]bool
	kycU map[string]string
}

// NewMemStore 构造进程内账本；事务失败时 snapshot/restore 回滚。

   func NewMemStore() *MemStore {
	  return &MemStore{
		   accounts:  map[string]*Account{},
		   transfers: map[string]*Transfer{},
		  byIdem:    map[string]string{},
		  allow:     map[string]AddressBookEntry{},
		   seed:    map[string]decimal.Decimal{},
		  frozenU: map[string]bool{},
		  kycU:    map[string]string{},
	   }
   }

   func accKey(userID string, typ AccountType, asset string) string {
	   return userID + "|" + string(typ) + "|" + asset
   }

   func (m *MemStore) Seed(userID string, typ AccountType, asset string, available string) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  if m.seed == nil {
		  m.seed = map[string]decimal.Decimal{}
	  }
	  amt := decimal.RequireFromString(available)
	  k := accKey(userID, typ, asset)
	   m.seed[k] = amt
	  m.nextAccID++
	  m.accounts[k] = &Account{
		   ID: m.nextAccID, UserID: userID, AccountType: typ, Asset: asset,
		  Available: amt, Frozen: decimal.Zero,
	   }
   }

// SeedAllowance 夹具写入的无流水水位。 Store 不实现则对账按 0。
   func (m *MemStore) SeedAllowance(userID string, typ AccountType, asset string) decimal.Decimal {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   if m.seed == nil {
		  return decimal.Zero
	  }
	   return m.seed[accKey(userID,
		  typ, asset)]
   }

  func (m *MemStore) Allow(userID, chain, addr string) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   m.allow[userID+"|"+chain+"|"+addr] = AddressBookEntry{
		   UserID: userID, Chain: chain, Address: addr, Label: "seed", Whitelist: true,
	   }
   }

// InTx 内存事务：先快照，fn 失败则 restore，和 SQL 同一套调用约定。
   func (m *MemStore) InTx(ctx context.Context, fn func(ctx context.Context) error) error {
	   if ctx.Value(memTxKey{}) == m {
		return fn(ctx)
	}
	m.txMu.Lock()
	defer m.txMu.Unlock()
	snap := m.snapshot()
	ctx = context.WithValue(ctx, memTxKey{}, m)
	if err := fn(ctx); err != nil {
		m.restore(snap)
		return err
	}
	return nil
}

type memTxKey struct{}

type memSnap struct {
	accounts map[string]*Account
	transfers map[string]*Transfer
	byIdem  map[string]string
	journal []JournalEntry
	audit []AuditEvent
	allow map[string]AddressBookEntry
	outbox []OutboxEvent
	seed      map[string]decimal.Decimal
	nextAccID int64
	frozenU map[string]bool
	kycU    map[string]string
}

   func (m *MemStore) snapshot() memSnap {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  acc := make(map[string]*Account, len(m.accounts))
	   for k, a := range m.accounts {
		  cp := *a
		  acc[k] = &cp
	   }
	  trs := make(map[string]*Transfer, len(m.transfers))
	   for k, t := range m.transfers {
		  cp := *t
		  if t.RiskFlags != nil {
			  cp.RiskFlags = append([]string{}, t.RiskFlags...)
		  }
		   trs[k] = &cp
	   }
	  idem := make(map[string]string, len(m.byIdem))
	  for k, v := range m.byIdem {
		   idem[k] = v
	   }
	   allow := make(map[string]AddressBookEntry,
		  len(m.allow))
	  for k, v := range m.allow {
		   allow[k] = v
	  }
	   j := append([]JournalEntry{}, m.journal...)
	   ob := append([]OutboxEvent{}, m.outbox...)
	  au := make([]AuditEvent, len(m.audit))
	  for i, e := range m.audit {
		  au[i] = e
		  if e.Detail != nil {
			   d := make(map[string]any, len(e.Detail))
			  for dk, dv := range e.Detail {
				  d[dk] = dv
			   }
			   au[i].Detail = d
		   }
	  }
	  fz := make(map[string]bool, len(m.frozenU))
	  for k, v := range m.frozenU {
		  fz[k] = v
	   }
	  kyc := make(map[string]string, len(m.kycU))
	  for k, v := range m.kycU {
		   kyc[k] = v
	   }
	  seed := make(map[string]decimal.Decimal, len(m.seed))
	   for k, v := range m.seed {
		  seed[k] = v
	   }
	   return memSnap{accounts: acc, transfers: trs, byIdem: idem, journal: j, audit: au, allow: allow, outbox: ob, seed: seed, nextAccID: m.nextAccID, frozenU: fz, kycU: kyc}
  }

   func (m *MemStore) restore(s memSnap) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  m.accounts = s.accounts
	  m.transfers = s.transfers
	  m.byIdem = s.byIdem
	   m.journal = s.journal
	  m.audit = s.audit
	  m.allow = s.allow
	   m.outbox = s.outbox
	   m.seed = s.seed
	   m.nextAccID = s.nextAccID
	   m.frozenU = s.frozenU
	  m.kycU = s.kycU
  }

// GetAccount 取科目余额；没有则造零账，和 SQLStore 行为一致。

   func (m *MemStore) GetAccount(_ context.Context, userID string, typ AccountType, asset string) (*Account, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   a,
		   ok := m.accounts[accKey(userID, typ, asset)]
	   if !ok {
		  m.nextAccID++
		   a = &Account{ID: m.nextAccID,
			  UserID: userID, AccountType: typ, Asset: asset, Available: decimal.Zero, Frozen: decimal.Zero}
		   m.accounts[accKey(userID, typ, asset)] = a
	   }
	  cp := *a
	  cp.stampBook()
	  return &cp, nil
   }

// UpsertAccount 写入可用与冻结。负数直接拒绝。
// 带快照时，内存中的当前余额必须与快照一致，否则拒绝覆盖。
// 没有快照时按绝对值写入，只用于建账和夹具。资金路径必须先 GetAccount。

   func (m *MemStore) UpsertAccount(_ context.Context, a *Account) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   if a == nil {
		   return ErrStaleBook
	   }
	   if a.Available.IsNegative() || a.Frozen.IsNegative() {
		  return ErrNegative
	  }
	   cur, ok := m.accounts[accKey(a.UserID, a.AccountType, a.Asset)]
	  if a.snapOK {
		   if !ok || !cur.Available.Equal(a.snapAvail) || !cur.Frozen.Equal(a.snapFrozen) {
			   return ErrStaleBook
		   }
		   cur.Available = a.Available
		   cur.Frozen = a.Frozen
		   a.stampBook()
		   return nil
	   }
	  if !ok {
		  m.nextAccID++
		   a.ID = m.nextAccID
		   cp := *a
		   m.accounts[accKey(a.UserID, a.AccountType, a.Asset)] = &cp
		  return nil
	  }
	  cur.Available = a.Available
	   cur.Frozen = a.Frozen
	   return nil
   }

// InsertTransfer 建单；幂等键已占用则冲突。
  func (m *MemStore) InsertTransfer(_ context.Context, t *Transfer) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  if id, ok := m.byIdem[t.IdempotencyKey]; ok {
		   if id != t.ID {
			   return ErrIdempotency
		  }
		  return nil
	  }
	   if t.TxHash != "" && !IsInternalTxHash(t.TxHash) {
		   for _, cur := range m.transfers {
			   if cur.Chain == t.Chain && cur.TxHash == t.TxHash && cur.OutputIndex == t.OutputIndex && cur.ID != t.ID {
				  return fmt.Errorf("%w: %s", ErrDuplicateHash, cur.ID)
			  }
		  }
	  }
	  cp := *t
	   m.transfers[t.ID] = &cp
	   m.byIdem[t.IdempotencyKey] = t.ID
	   return nil
   }

   func (m *MemStore) GetTransfer(_ context.Context, id string) (*Transfer, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   t, ok := m.transfers[id]
	   if !ok {
		  return nil, ErrNotFound
	   }
	   cp := *t
	   return &cp, nil
  }

// GetByIdempotency 按幂等键找回已有单，Create 据此 resume。

  func (m *MemStore) GetByIdempotency(_ context.Context, key string) (*Transfer, error) {
	   m.mu.Lock()
	  id, ok := m.byIdem[key]
	  m.mu.Unlock()
	   if !ok {
		   return nil, ErrNotFound
	   }
	  return m.GetTransfer(context.Background(), id)
   }

   func (m *MemStore) UpdateTransfer(_ context.Context, t *Transfer) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   cur, ok := m.transfers[t.ID]
	  if !ok {
		  return ErrNotFound
	  }
	   if cur.FenceEpoch != t.FenceEpoch || cur.FenceToken != t.FenceToken {
		  return ErrStaleFence
	   }
	  cp := *t
	  if t.RiskFlags != nil {
		   cp.RiskFlags = append([]string{},
			   t.RiskFlags...)
	   }
	  *cur = cp
	  return nil
  }

// StampFence 盖 fence epoch，丢租约后过期 worker 写不进。
  func (m *MemStore) StampFence(_ context.Context, id, token string) (int64, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   cur, ok := m.transfers[id]
	   if !ok {
		   return 0, ErrNotFound
	  }
	  cur.FenceEpoch++
	  cur.FenceToken = token
	   return cur.FenceEpoch, nil
  }

// AppendJournal 先 ValidateJournal 再追加；不平的批次整批拒绝。

   func (m *MemStore) AppendJournal(_ context.Context, entries []JournalEntry) error {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   if err := ValidateJournal(entries); err != nil {
		   return err
	  }
	  seen := map[string]struct{}{}
	   for _, e := range m.journal {
		  seen[e.TransferID+"|"+strconv.Itoa(e.Seq)] = struct{}{}
	   }
	   for _, e := range entries {
		   k := e.TransferID + "|" + strconv.Itoa(e.Seq)
		  if _, ok := seen[k]; ok {
			  return fmt.Errorf("duplicate journal seq %s", k)
		   }
		  seen[k] = struct{}{}
	  }
	  m.journal = append(m.journal, entries...)
	  return nil
   }

   func (m *MemStore) ListJournal(_ context.Context, transferID string) ([]JournalEntry, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  var out []JournalEntry
	   for _, e := range m.journal {
		   if e.TransferID == transferID {
			   out = append(out, e)
		  }
	  }
	  return out, nil
   }

   func (m *MemStore) ListUserJournal(_ context.Context, userID string, limit int) ([]JournalEntry, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if limit <= 0 {
		  limit = 50
	  }
	   var matched []JournalEntry
	   for _, e := range m.journal {
		  if e.UserID == userID {
			   matched = append(matched, e)
		  }
	  }
	  if len(matched) > limit {
		  matched = matched[len(matched)-limit:]
	   }
	  out := make([]JournalEntry, 0, len(matched))
	   for i := len(matched) - 1; i >= 0; i-- {
		  out = append(out, matched[i])
	   }
	  return out, nil
  }

   func (m *MemStore) UserControlFrozen(_ context.Context, userID string) (bool, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  return m.frozenU[userID], nil
  }

   func (m *MemStore) SetUserControlFrozen(_ context.Context, userID string, frozen bool) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   if m.frozenU == nil {
		   m.frozenU = map[string]bool{}
	   }
	   if frozen {
		  m.frozenU[userID] = true
	  } else {
		   delete(m.frozenU, userID)
	   }
	   return nil
  }

   func (m *MemStore) UserKYC(_ context.Context, userID string) (string, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   if m.kycU == nil {
		  return KYCBasic, nil
	  }
	   if v, ok := m.kycU[userID]; ok && v != "" {
		   return v, nil
	   }
	   return KYCBasic, nil
  }

   func (m *MemStore) SetUserKYC(_ context.Context, userID, level string) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if m.kycU == nil {
		  m.kycU = map[string]string{}
	   }
	  m.kycU[userID] = level
	  return nil
  }

   func (m *MemStore) SumDebitCredit(_ context.Context, userID string, typ AccountType, asset string) (decimal.Decimal, decimal.Decimal, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   d, c := decimal.Zero, decimal.Zero
	   for _, e := range m.journal {
		  if e.UserID == userID && e.AccountType == typ && e.Asset == asset {
			  d = d.Add(e.Debit)
			  c = c.Add(e.Credit)
		   }
	  }
	   return d, c, nil
   }

  func (m *MemStore) SumToday(_ context.Context, userID, asset string) (decimal.Decimal, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   sum := decimal.Zero
	  day := time.Now().UTC().Truncate(24 * time.Hour)
	   for _, t := range m.transfers {
		   if t.UserID == userID && t.Asset == asset && !t.CreatedAt.Before(day) {
			   if t.State == StateFailed || t.State == StateRolledBack {
				   continue
			  }
			  sum = sum.Add(t.Amount)
		   }
	  }
	   return sum, nil
  }

// CountSince 统计 since 之后、同方向池、未失败的单据数。频率闸用它，避免把整表载入内存。
func (m *MemStore) CountSince(_ context.Context, userID string, dir Direction, since time.Time) (int, error) {
	if m == nil {
		return 0, fmt.Errorf("risk store unavailable")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	pool := directionPool(dir)
	for _, t := range m.transfers {
		if t.UserID != userID || !t.CreatedAt.After(since) {
			continue
		}
		if t.State == StateFailed || t.State == StateRolledBack {
			continue
		}
		if directionPool(t.Direction) != pool {
			continue
		}
		n++
	}
	return n, nil
}

// SumTodayByDirection 分方向日占用；充提互不挤占对方额度。
   func (m *MemStore) SumTodayByDirection(_ context.Context, userID, asset string, dir Direction) (decimal.Decimal, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  sum := decimal.Zero
	   day := time.Now().UTC().Truncate(24 * time.Hour)
	  pool := directionPool(dir)
	   for _, t := range m.transfers {
		   if t.UserID != userID || t.Asset != asset || t.CreatedAt.Before(day) {
			  continue
		  }
		  if t.State == StateFailed || t.State == StateRolledBack {
			  continue
		   }
		  if directionPool(t.Direction) != pool {
			  continue
		  }
		   sum = sum.Add(t.Amount)
	  }
	   return sum, nil
  }

   func (m *MemStore) AppendAudit(_ context.Context, e AuditEvent) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   if e.CreatedAt.IsZero() {
		   e.CreatedAt = time.Now().UTC()
	   }
	   m.audit = append(m.audit, e)
	   return nil
  }

   func (m *MemStore) ListAudit(_ context.Context, entityType, entityID string, limit int) ([]AuditEvent, error) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   if limit <= 0 {
		   limit = 50
	  }
	   var out []AuditEvent
	   for i := len(m.audit) - 1; i >= 0; i-- {
		   e := m.audit[i]
		  if entityType != "" && e.EntityType != entityType {
			   continue
		   }
		   if entityID != "" && e.EntityID != entityID {
			   continue
		  }
		   out = append(out, e)
		   if len(out) >= limit {
			  break
		  }
	   }
	  return out, nil
   }

   func (m *MemStore) Allowlisted(_ context.Context, userID, chain, addr string) (bool, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   for _, c := range allowlistChains(chain) {
		  if e, ok := m.allow[userID+"|"+c+"|"+addr]; ok && !e.Disabled {
			   return true, nil
		  }
	   }
	  return false, nil
   }

func (m *MemStore) AllowlistCount(_ context.Context, userID, chain string) (int, error) {
	if m == nil {
		return 0, fmt.Errorf("risk store unavailable")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, c := range allowlistChains(chain) {
		prefix := userID + "|" + c + "|"
		for k, e := range m.allow {
			if strings.HasPrefix(k, prefix) && !e.Disabled {
				n++
			}
		}
	}
	return n, nil
}

   func (m *MemStore) ListAddressBook(_ context.Context, userID string) ([]AddressBookEntry, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   out := make([]AddressBookEntry, 0)
	  for _, e := range m.allow {
		  if e.UserID == userID {
			  cp := e
			   cp.Whitelist = !cp.Disabled
			   out = append(out, cp)
		  }
	   }
	  return out, nil
  }

  func (m *MemStore) UpsertAddressBook(_ context.Context, e AddressBookEntry) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  e.Whitelist = !e.Disabled
	  m.allow[e.UserID+"|"+e.Chain+"|"+e.Address] = e
	  return nil
   }

  func (m *MemStore) DeleteAddressBook(_ context.Context, userID, chain, addr string) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  k := userID + "|" + chain + "|" + addr
	   if _, ok := m.allow[k]; !ok {
		   return ErrNotFound
	  }
	   delete(m.allow, k)
	   return nil
  }

   func (m *MemStore) ListAccounts(_ context.Context, limit int) ([]Account, error) {
	   if limit <= 0 {
		   limit = 5000
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   out := make([]Account, 0, len(m.accounts))
	  for _, a := range m.accounts {
		   if a == nil {
			   continue
		  }
		   out = append(out, *a)
		  if len(out) >= limit {
			   break
		  }
	  }
	   return out, nil
  }

   func (m *MemStore) ListUserBooks(_ context.Context, userID string) ([]Account, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  var out []Account
	  for _, a := range m.accounts {
		   if a.UserID == userID {
			   out = append(out, *a)
		  }
	   }
	  return out, nil
   }

  func (m *MemStore) ListByState(_ context.Context, st State, limit int) ([]*Transfer, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   var out []*Transfer
	  for _, t := range m.transfers {
		  if t.State == st {
			   cp := *t
			  out = append(out, &cp)
			  if limit > 0 && len(out) >= limit {
				  break
			   }
		   }
	   }
	   return out, nil
  }

  func (m *MemStore) ListUserTransfers(_ context.Context, userID string, limit int) ([]*Transfer, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  var out []*Transfer
	  for _, t := range m.transfers {
		   if t.UserID == userID {
			   cp := *t
			  out = append(out, &cp)
			  if limit > 0 && len(out) >= limit {
				  break
			   }
		  }
	   }
	   return out, nil
  }

// ListIncomingInner 对手方为本人的内部转入。SpotSend 明文落 ToAddress/Hint。

   func (m *MemStore) ListIncomingInner(_ context.Context, userID string, limit int) ([]*Transfer, error) {
	   userID = strings.TrimSpace(userID)
	  if userID == "" {
		  return nil, nil
	  }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  var out []*Transfer
	  for _, t := range m.transfers {
		  if t == nil || t.Direction != SpotSend {
			  continue
		   }
		  if t.UserID == userID {
			   continue
		  }
		   if strings.TrimSpace(t.ToAddress) != userID && strings.TrimSpace(t.ToAddressHint) != userID {
			   continue
		   }
		  cp := *t
		   out = append(out, &cp)
		   if limit > 0 && len(out) >= limit {
			   break
		  }
	   }
	  return out, nil
  }

// FindByChainTxHash 按 (chain, hash, output_index) 查单。

   func (m *MemStore) FindByChainTxHash(_ context.Context, chain, hash string, outputIndex int) (*Transfer, error) {
	  if hash == "" || IsInternalTxHash(hash) {
		  return nil, ErrNotFound
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  for _, t := range m.transfers {
		  if t.Chain == chain && t.TxHash == hash && t.OutputIndex == outputIndex {
			  cp := *t
			  if t.RiskFlags != nil {
				  cp.RiskFlags = append([]string{}, t.RiskFlags...)
			   }
			  return &cp, nil
		   }
	  }
	   return nil, ErrNotFound
  }

// AppendOutbox 与改账同事务入出箱（内存里就是同一把锁）。
  func (m *MemStore) AppendOutbox(_ context.Context, e OutboxEvent) error {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  if e.ID == "" {
		  e.ID = strconv.FormatInt(time.Now().UTC().UnixNano(), 10) + "-" + e.AggregateID
	   }
	  if e.CreatedAt.IsZero() {
		   e.CreatedAt = time.Now().UTC()
	   }
	  cp := e
	   if e.Payload != nil {
		   d := make(map[string]any, len(e.Payload))
		  for k, v := range e.Payload {
			   d[k] = v
		  }
		   cp.Payload = d
	   }
	   m.outbox = append(m.outbox, cp)
	  return nil
  }

  func (m *MemStore) ListOutbox(_ context.Context, unpublishedOnly bool, limit int) ([]OutboxEvent, error) {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   if limit <= 0 {
		   limit = 50
	  }
	  out := make([]OutboxEvent, 0, limit)
	   for i := len(m.outbox) - 1; i >= 0; i-- {
		   e := m.outbox[i]
		   if unpublishedOnly && e.PublishedAt != nil {
			  continue
		   }
		   cp := e
		  if e.Payload != nil {
			   d := make(map[string]any,
				   len(e.Payload))
			  for k, v := range e.Payload {
				  d[k] = v
			   }
			   cp.Payload = d
		  }
		  out = append(out, cp)
		  if len(out) >= limit {
			  break
		  }
	  }
	  return out, nil
   }

  func (m *MemStore) MarkOutboxPublished(_ context.Context, ids []string) error {
	  if len(ids) == 0 {
		  return nil
	   }
	   set := map[string]struct{}{}
	   for _, id := range ids {
		   set[id] = struct{}{}
	  }
	   now := time.Now().UTC()
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   for i := range m.outbox {
		  if _, ok := set[m.outbox[i].ID]; ok && m.outbox[i].PublishedAt == nil {
			   t := now
			  m.outbox[i].PublishedAt = &t
		   }
	   }
	   return nil
   }

   func (m *MemStore) SetFrozen(userID string, typ AccountType, asset, frozen string) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   a,
		   ok := m.accounts[accKey(userID, typ, asset)]
	   if !ok {
		  return
	  }
	   a.Frozen = decimal.RequireFromString(frozen)
   }

   func (m *MemStore) LastAuditAction() string {
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  if len(m.audit) == 0 {
		  return ""
	   }
	   return m.audit[len(m.audit)-1].Action
   }

   func (m *MemStore) Ping(context.Context) error { return nil }

// SeedBooks 写入种子用户账本与提现白名单。
  func SeedBooks(m *MemStore) {
	   uid := config.DefaultSeedUserID
	   if cfg, err := config.Load(); err == nil && cfg.SeedUserID != "" {
		  uid = cfg.SeedUserID
	   }
	   m.Seed(uid, AccountSpot, "USDT", "10000")
	  m.Seed(uid, AccountSpot, "BTC", "0.5")
	   m.Seed(uid, AccountSpot, "ETH", "8")
	   m.Seed(uid, AccountWallet, "USDT", "2000")
	  m.Seed(uid, AccountWallet, "ETH", "2")
	   m.Seed(uid, AccountWallet, "BTC", "0.1")
	   m.Seed(uid, AccountWallet, "SOL", "50")
	  m.Seed(uid, AccountWallet, "TRX", "10000")
	   m.Seed(uid, AccountWallet, "TON", "100")
	  m.Seed(uid, AccountTrading, "USDT", "0")
	  m.Seed(uid, AccountMargin, "USDT", "0")
	   m.Seed(uid, AccountIsolated, "USDT", "0")
	   m.Seed(uid, AccountIsolated, "BTC", "0")
	   m.Seed(uid, AccountFutures, "USDT", "0")
	  m.Seed(uid, AccountEarn, "USDT", "0")
	   m.Seed(config.PeerSeedUserID, AccountSpot, "USDT", "100")
	   m.Seed("system", AccountBridge, "USDT", "1000000")
	  m.Seed("system", AccountBridge, "BTC", "100")
	  m.Seed("system", AccountBridge, "ETH", "1000")
	   m.Seed("system", AccountBridge, "SOL", "10000")
	  m.Seed("system", AccountBridge, "TRX", "1000000")
	   m.Seed("system", AccountBridge, "TON", "10000")
	  m.Seed("system", AccountFee, "USDT", "0")
	   m.Seed("system", AccountFee, "ETH", "0")
	   m.Seed("system", AccountFee, "BTC", "0")
	   m.Seed("system", AccountFee, "SOL", "0")
	   m.Seed("system", AccountCold, "USDT", "0")
	   m.Seed("system", AccountCold, "ETH", "0")
	   m.Seed("system", AccountCold, "BTC", "0")
	   m.Seed("system", AccountWarm, "USDT", "0")
	   m.Seed("system", AccountWarm, "ETH", "0")
	   m.Seed("system", AccountWarm, "BTC", "0")
	  for _, c := range wallet.SupportedChains() {
		   m.Allow(uid,
			  c, wallet.DeriveAddress(uid, c))
		   m.Allow(uid, c, wallet.DeriveAddress(ExtWithdrawUser, c))
	  }
	   m.Allow(uid, "EVM", wallet.DeriveAddress(uid, "ETH"))
	   m.Allow(uid, "EVM", wallet.DeriveAddress(ExtWithdrawUser, "ETH"))
  }

// ExtWithdrawUser 流程出金对手。不用用户自己的入站地址当提现目标。

const ExtWithdrawUser = "ext-withdraw-peer"

// ApplyJournalToAccount 只认 memo 改可用/冻结。未知 memo 直接失败；改完变负则拒，坏流水不得落账。

   func ApplyJournalToAccount(a *Account, e JournalEntry) error {
	   switch e.Memo {
	  case "freeze_available":
		   a.Available = a.Available.Sub(e.Debit)
	   case "freeze_frozen":
		  a.Frozen = a.Frozen.Add(e.Credit)
	  case "consume_frozen":
		   a.Frozen = a.Frozen.Sub(e.Debit)
	   case "credit_available":
		   a.Available = a.Available.Add(e.Credit)
	   case "unfreeze_frozen":
		  a.Frozen = a.Frozen.Sub(e.Debit)
	   case "restore_available":
		  a.Available = a.Available.Add(e.Credit)
	   case "bridge_in":
		   a.Available = a.Available.Add(e.Credit)
	   case "bridge_out":
		   a.Available = a.Available.Sub(e.Debit)
	   case "reorg_debit_available":
		   a.Available = a.Available.Sub(e.Debit)
	  case "reorg_restore_frozen":
		  a.Frozen = a.Frozen.Add(e.Credit)
	   case "fee_debit":
		   a.Available = a.Available.Sub(e.Debit)
	  case "fee_credit":
		  a.Available = a.Available.Add(e.Credit)
	  case "swap_debit":
		   a.Available = a.Available.Sub(e.Debit)
	   case "swap_credit":
		   a.Available = a.Available.Add(e.Credit)
	  case "interest_accrue":
		  // 应计利息占用：记入保证金科目冻结，不碰可用。
		   a.Frozen = a.Frozen.Add(e.Debit)
	  case "interest_offset":
		   // 还款冲应计：释放保证金冻结。
		   a.Frozen = a.Frozen.Sub(e.Credit)
	  default:
		   return fmt.Errorf("unknown journal memo %s", e.Memo)
	   }
	  if a.Available.IsNegative() || a.Frozen.IsNegative() {
		  return ErrNegative
	  }
	   return nil
   }

// ListUserTransfersPaged 按保留窗与分页列出用户划转（充值历史用）。
  func (m *MemStore) ListUserTransfersPaged(_ context.Context, q DepositHistQuery) ([]*Transfer, int, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  matched := make([]*Transfer, 0)
	  for _, t := range m.transfers {
		   if t == nil || t.UserID != q.UserID {
			  continue
		   }
		   if t.Direction != WalletDeposit {
			  continue
		  }
		   if !q.Start.IsZero() && t.CreatedAt.Before(q.Start) {
			  continue
		  }
		   if !q.End.IsZero() && t.CreatedAt.After(q.End) {
			  continue
		  }
		  if q.Currency != "" && !strings.EqualFold(t.Asset, q.Currency) {
			  continue
		   }
		   if !MatchStateFilter(t, q.Status) {
			  continue
		   }
		   cp := *t
		  matched = append(matched, &cp)
	   }
	   // 近到远
	   for i := 0; i < len(matched); i++ {
		   for j := i + 1; j < len(matched); j++ {
			   if matched[j].CreatedAt.After(matched[i].CreatedAt) {
				  matched[i],
					  matched[j] = matched[j], matched[i]
			  }
		  }
	   }
	   total := len(matched)
	   off := q.Offset()
	  if off >= total {
		   return []*Transfer{}, total, nil
	  }
	   end := off + q.PageSize
	   if end > total {
		  end = total
	  }
	  return matched[off:end], total, nil
   }

// ListIncomingInnerPaged 内部转入 SQL 同口径分页 + 保留窗。

  func (m *MemStore) ListIncomingInnerPaged(_ context.Context, q DepositHistQuery) ([]*Transfer, int, error) {
	   userID := strings.TrimSpace(q.UserID)
	  if userID == "" {
		   return nil, 0, nil
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   matched := make([]*Transfer, 0)
	   for _, t := range m.transfers {
		  if t == nil || t.Direction != SpotSend {
			  continue
		   }
		  if t.UserID == userID {
			  continue
		  }
		   if strings.TrimSpace(t.ToAddress) != userID && strings.TrimSpace(t.ToAddressHint) != userID {
			  continue
		  }
		  if !q.Start.IsZero() && t.CreatedAt.Before(q.Start) {
			   continue
		   }
		   if !q.End.IsZero() && t.CreatedAt.After(q.End) {
			   continue
		   }
		   if q.Currency != "" && !strings.EqualFold(t.Asset, q.Currency) {
			  continue
		   }
		  if !MatchStateFilter(t, q.Status) {
			   continue
		   }
		  cp := *t
		  matched = append(matched, &cp)
	   }
	  for i := 0; i < len(matched); i++ {
		   for j := i + 1; j < len(matched); j++ {
			  if matched[j].CreatedAt.After(matched[i].CreatedAt) {
				  matched[i], matched[j] = matched[j], matched[i]
			   }
		   }
	   }
	   total := len(matched)
	   off := q.Offset()
	  if off >= total {
		  return []*Transfer{}, total, nil
	  }
	  end := off + q.PageSize
	   if end > total {
		   end = total
	  }
	  return matched[off:end], total, nil
   }

// ListUserWithdrawsPaged 提现历史分页（WalletWithdraw + SpotSend 本人转出）。

  func (m *MemStore) ListUserWithdrawsPaged(_ context.Context, q WithdrawHistQuery) ([]*Transfer, int, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   matched := make([]*Transfer, 0)
	  for _, t := range m.transfers {
		   if t == nil || t.UserID != q.UserID {
			   continue
		  }
		   switch strings.ToLower(strings.TrimSpace(q.InnerFilter)) {
		  case "true", "1", "yes", "inner":
			  if t.Direction != SpotSend {
				   continue
			   }
		  case "false", "0", "no", "chain":
			   if t.Direction != WalletWithdraw {
				  continue
			   }
		  default:
			  if t.Direction != WalletWithdraw && t.Direction != SpotSend {
				  continue
			   }
		   }
		   if !q.Start.IsZero() && t.CreatedAt.Before(q.Start) {
			   continue
		  }
		  if !q.End.IsZero() && t.CreatedAt.After(q.End) {
			  continue
		  }
		  if q.Currency != "" && !strings.EqualFold(t.Asset, q.Currency) {
			  continue
		   }
		   if !MatchStateFilter(t, q.Status) {
			  continue
		  }
		  cp := *t
		  matched = append(matched, &cp)
	  }
	  for i := 0; i < len(matched); i++ {
		  for j := i + 1; j < len(matched); j++ {
			   if matched[j].CreatedAt.After(matched[i].CreatedAt) {
				  matched[i], matched[j] = matched[j], matched[i]
			   }
		   }
	  }
	   total := len(matched)
	   off := q.Offset()
	  if off >= total {
		  return []*Transfer{}, total, nil
	  }
	   end := off + q.PageSize
	   if end > total {
		  end = total
	   }
	  return matched[off:end], total, nil
   }

// ListUserDepositsMergedPaged 链上+内部转入按 created_at 降序合并分页（内存同口径）。

   func (m *MemStore) ListUserDepositsMergedPaged(_ context.Context, q DepositHistQuery) ([]*Transfer, int, error) {
	   userID := strings.TrimSpace(q.UserID)
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   matched := make([]*Transfer, 0)
	   for _, t := range m.transfers {
		   if t == nil {
			  continue
		   }
		   ok := false
		   switch q.InnerMode {
		   case DepositInnerOnly:
			  if t.Direction == SpotSend && t.UserID != userID &&
				  (strings.TrimSpace(t.ToAddress) == userID || strings.TrimSpace(t.ToAddressHint) == userID) {
				  ok = true
			  }
		  case DepositChainOnly:
			   if t.Direction == WalletDeposit && t.UserID == userID {
				  ok = true
			   }
		  default:
			   if t.Direction == WalletDeposit && t.UserID == userID {
				  ok = true
			   }
			   if t.Direction == SpotSend && t.UserID != userID &&
				   (strings.TrimSpace(t.ToAddress) == userID || strings.TrimSpace(t.ToAddressHint) == userID) {
				   ok = true
			  }
		  }
		   if !ok {
			   continue
		   }
		   if !q.Start.IsZero() && t.CreatedAt.Before(q.Start) {
			  continue
		  }
		   if !q.End.IsZero() && t.CreatedAt.After(q.End) {
			  continue
		  }
		   if q.Currency != "" && !strings.EqualFold(t.Asset, q.Currency) {
			   continue
		   }
		   if !MatchStateFilter(t, q.Status) {
			  continue
		   }
		  cp := *t
		   matched = append(matched, &cp)
	  }
	   for i := 0; i < len(matched); i++ {
		  for j := i + 1; j < len(matched); j++ {
			  if matched[j].CreatedAt.After(matched[i].CreatedAt) {
				   matched[i], matched[j] = matched[j], matched[i]
			   }
		   }
	  }
	   total := len(matched)
	   off := q.Offset()
	   if off >= total {
		   return []*Transfer{}, total, nil
	  }
	  end := off + q.PageSize
	   if end > total {
		  end = total
	  }
	  return matched[off:end], total, nil
  }
