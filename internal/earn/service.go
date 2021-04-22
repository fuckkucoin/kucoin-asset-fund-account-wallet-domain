package earn

import (
	"context"
	"fmt"
	"time"
	"account-assets.kucoin.net/funding/internal/account"
	"account-assets.kucoin.net/funding/internal/command"
	"account-assets.kucoin.net/funding/internal/holdcat"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/google/uuid"
)

type Service struct {
	led  *ledger.Service
	acct *account.Service
	cat  *Catalog
	book *Book
	cmds  *command.Log
	holds *holdcat.Book
	clock func() time.Time
}

  func New(led *ledger.Service) *Service {
	  acct := account.New(led)
	  return &Service{
		   led:  led,
		   acct: acct, cat: DefaultCatalog(), book: NewBook(),
		   cmds: command.NewLog(), clock: func() time.Time { return time.Now().UTC() },
	  }
  }

   func (s *Service) BindKV(kv persist.KV) *Service {
	  if s != nil && s.book != nil {
		   s.book.BindKV(kv)
	  }
	  return s
   }

// BindSQL 仓位与产品目录挂 SQL 主存；OpenMem 不绑。
// 须随后 Hydrate；Put/运营 PUT 写库。
// 未绑定时多实例仓位/EarlyBPS 漂移。

   func (s *Service) BindSQL(db *persist.SQLDB) *Service {
	   if s == nil {
		  return s
	   }
	   if s.book != nil {
		  s.book.BindSQL(db)
	   }
	  if s.cat != nil {
		   s.cat.BindSQL(db)
	   }
	  return s
  }

// Hydrate 启动装回仓位与产品目录。
// 目录无行保留 Default；仓位全量覆盖内存。
// Hydrate 失败不得假装无仓/无目录。
   func (s *Service) Hydrate(ctx context.Context) error {
	  if s == nil {
		  return ErrUnavailable
	  }
	  if ctx == nil {
		  ctx = context.Background()
	  }
	   if s.cat != nil {
		   if err := s.cat.Hydrate(ctx); err != nil {
			   return err
		  }
	   }
	   if s.book != nil {
		  if err := s.book.Hydrate(ctx); err != nil {
			  return err
		   }
	  }
	  return nil
   }

// Health 仓位与目录 Hydrate 健康面。
  func (s *Service) Health() map[string]any {
	   if s == nil {
		  return map[string]any{}
	  }
	   out := map[string]any{}
	  if s.book != nil {
		  out["positions"] = s.book.Health()
	  }
	  if s.cat != nil {
		   out["catalog"] = s.cat.Health()
	  }
	   return out
  }

// BindHolds 挂冻结分类账。申购成功写 earn_hold；赎回按仓位 id ReleaseByRef。

  func (s *Service) BindHolds(h *holdcat.Book) *Service {
	  if s != nil {
		   s.holds = h
	  }
	  return s
  }

  func (s *Service) ProductsBy(cat string) []Product {
	  if s == nil || s.cat == nil {
		   return nil
	   }
	  return s.cat.ListCategory(cat, true)
   }

   func (s *Service) Catalog() *Catalog { return s.cat }

  func (s *Service) Positions() *Book { return s.book }

   func (s *Service) WithClock(c func() time.Time) *Service {
	  if s != nil && c != nil {
		   s.clock = c
	   }
	  return s
  }

  type SubscribeReq struct {
	   UserID string
	   ProductID      string
	  Asset          string
	   Amount         string
	  IdempotencyKey string
	   AutoSubscribe bool
  }

  // Subscribe 资金账 → 理财。先过产品闸，再走 ledger.EarnSubscribe。
   // 同一幂等键回放原仓，不二次冻。
   func (s *Service) Subscribe(ctx context.Context, req SubscribeReq) (Position, error) {
	  if s == nil {
		  return Position{},
			   ErrUnavailable
	   }
	   uid, err := validate.UserID(req.UserID)
	  if err != nil {
		  return Position{}, err
	  }
	   asset,
		   err := validate.Asset(req.Asset)
	   if err != nil {
		   return Position{}, err
	  }
	  amtStr, err := validate.Amount(req.Amount)
	   if err != nil {
		  return Position{}, err
	   }
	  key,
		  err := validate.Idem(req.IdempotencyKey)
	   if err != nil {
		   return Position{}, err
	   }
	   if prev, ok := s.book.ByIdem(key); ok {
		  return prev, nil
	   }
	   prod, err := s.cat.Require(req.ProductID, asset)
	  if err != nil {
		   return Position{}, err
	   }
	   amt, err := money.Parse(amtStr)
	  if err != nil {
		   return Position{}, err
	   }
	  if prod.MinSubscribe != "" {
		   min, _ := money.Parse(prod.MinSubscribe)
		   if amt.Cmp(min) < 0 {
			   return Position{}, ErrMin
		   }
	  }
	   if prod.MaxPosition != "" {
		   cur,
			   err := s.book.SumPrincipal(uid, prod.ID)
		   if err != nil {
			   return Position{}, err
		   }
		   have, err := money.ParseBook(cur)
		  if err != nil {
			  return Position{}, err
		  }
		   next, err := have.Add(amt)
		  if err != nil {
			   return Position{}, err
		   }
		  max, _ := money.Parse(prod.MaxPosition)
		   if next.Cmp(max) > 0 {
			   return Position{}, ErrMax
		  }
	   }
	  now := s.clock()
	  st := StatusOpen
	  unlock := time.Time{}
	   if prod.LockDays > 0 {
		   st = StatusLocked
		   unlock = now.Add(time.Duration(prod.LockDays) * 24 * time.Hour)
	   }
	  payload := command.HashPayload("earn_sub", uid, prod.ID, asset, amtStr)
	  rec, existed, err := s.cmds.Begin(key, "earn_subscribe", payload)
	  if err != nil {
		  return Position{}, err
	   }
	   if existed && rec.Status == command.StatusSucceeded && rec.ResultID != "" {
		  if p, ok := s.book.Get(rec.ResultID); ok {
			   return p, nil
		  }
	  }
	  if s.acct != nil {
		   if _, err := s.acct.Open(ctx, uid, account.BookEarn); err != nil {
			  s.cmds.Fail(key, err)
			   return Position{}, err
		  }
	   }
	   pos := Position{
		  ID: uuid.NewString(), UserID: uid, ProductID: prod.ID, Asset: asset,
		   Principal: amtStr, Accrued: "0", Claimed: "0",
		  OpenedAt: now,
		   UnlockAt: unlock, Status: st, Idempotency: key,
		  AutoSubscribe: req.AutoSubscribe || prod.AutoSubscribe,
	   }
	   if s.led != nil {
		  dec,
			   err := ledger.ParseAmount(amtStr)
		  if err != nil {
			   s.cmds.Fail(key, err)
			  return Position{}, err
		   }
		  tr, err := s.led.Create(ctx, ledger.CreateReq{
			  UserID: uid,
			  Direction: ledger.EarnSubscribe, Asset: asset,
			   Amount: dec, IdempotencyKey: key,
		  })
		  if err != nil {
			  s.cmds.Fail(key, err)
			  return Position{}, err
		   }
		  pos.TransferID = tr.ID
	  }
	   if err := s.book.Put(pos); err != nil {
		   s.cmds.Fail(key, err)
		   return Position{}, err
	   }
	   s.cmds.Succeed(key, pos.ID)
	   if s.holds != nil {
		   if _, err := s.holds.PlaceEarnHold(ctx, uid, asset, "funding", amtStr, pos.ID); err != nil {
			   return Position{}, err
		   }
	  }
	   return pos, nil
   }

type RedeemReq struct {
	UserID         string
	PositionID     string
	IdempotencyKey string
}

// Redeem 理财 → 资金账。锁定期未满拒；本金走 EarnRedeem，应付利息另记 claimed。

  func (s *Service) Redeem(ctx context.Context, req RedeemReq) (Position, error) {
	  if s == nil {
		  return Position{}, ErrUnavailable
	  }
	   uid, err := validate.UserID(req.UserID)
	  if err != nil {
		  return Position{}, err
	   }
	  key, err := validate.Idem(req.IdempotencyKey)
	  if err != nil {
		  return Position{}, err
	   }
	  p, ok := s.book.Get(req.PositionID)
	  if !ok {
		  return Position{}, ErrNotFound
	  }
	  if p.UserID != uid {
		  return Position{}, ErrNotFound
	  }
	   now := s.clock()
	  if err := RedeemCheck(p, now); err != nil {
		   return Position{}, err
	  }
	   payload := command.HashPayload("earn_red", uid, p.ID, p.Principal)
	   rec, existed, err := s.cmds.Begin(key, "earn_redeem", payload)
	  if err != nil {
		   return Position{}, err
	  }
	   if existed && rec.Status == command.StatusSucceeded && rec.ResultID != "" {
		  if got, ok := s.book.Get(rec.ResultID); ok {
			  return got, nil
		  }
	   }
	   if un, err := p.Unclaimed(); err == nil && un.IsPositive() {
		   if _, err := s.PostInterest(ctx, uid, p.ID, key+":int"); err != nil {
			   s.cmds.Fail(key, err)
			   return Position{}, err
		  }
		   if fresh, ok := s.book.Get(p.ID); ok {
			  p = fresh
		   }
	   }
	  if s.led != nil {
		   dec, err := ledger.ParseAmount(p.Principal)
		  if err != nil {
			   s.cmds.Fail(key, err)
			  return Position{}, err
		   }
		  _, err = s.led.Create(ctx, ledger.CreateReq{
			  UserID: uid, Direction: ledger.EarnRedeem, Asset: p.Asset,
			   Amount: dec, IdempotencyKey: key,
		  })
		   if err != nil {
			   s.cmds.Fail(key, err)
			  return Position{}, err
		  }
	   }
	  p.Status = StatusRedeemed
	   if err := s.book.Put(p); err != nil {
		   s.cmds.Fail(key, err)
		   return Position{}, err
	   }
	   s.cmds.Succeed(key, p.ID)
	   if s.holds != nil {
		   if _, err := s.holds.ReleaseByRef(ctx, uid, holdcat.ReasonEarnHold, p.ID); err != nil {
			   return Position{}, err
		   }
	  }
	   return p, nil
   }

  func (s *Service) AccrueUser(userID string) ([]Accrual, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	  }
	   now := s.clock()
	  out := make([]Accrual, 0)
	  for _, p := range s.book.Open(userID, "") {
		   prod,
			  err := s.cat.Get(p.ProductID)
		   if err != nil {
			  return out, err
		  }
		  next, row, err := AccrueOnce(p, prod, now)
		   if err != nil {
			   return out, err
		   }
		  if err := s.book.Put(next); err != nil {
			  return out, err
		  }
		   if row.Days > 0 {
			  out = append(out, row)
		   }
	  }
	  return out, nil
   }

  func (s *Service) Claim(positionID string) (Claim, error) {
	   p, ok := s.book.Get(positionID)
	   if !ok {
		   return Claim{}, ErrNotFound
	   }
	   if s.led != nil {
		  post, err := s.PostInterest(context.Background(), p.UserID, positionID, "earn-claim:"+p.ID)
		   if err != nil {
			   return Claim{}, err
		   }
		  return Claim{PositionID: p.ID, Amount: post.Amount, At: s.clock()}, nil
	  }
	  next, cl, err := ClaimAccrued(p, s.clock())
	  if err != nil {
		  return Claim{}, err
	  }
	   if err := s.book.Put(next); err != nil {
		   return Claim{}, err
	   }
	   return cl, nil
   }

  func (s *Service) Get(id string) (Position, error) {
	  p, ok := s.book.Get(id)
	   if !ok {
		  return Position{}, ErrNotFound
	   }
	  return p, nil
  }

  func (s *Service) SetAutoSubscribe(userID, positionID string, on bool) (Position, error) {
	   p, ok := s.book.Get(positionID)
	  if !ok || p.UserID != userID {
		   return Position{}, ErrNotFound
	  }
	   p.AutoSubscribe = on
	  if err := s.book.Put(p); err != nil {
		  return Position{}, err
	  }
	   return p, nil
   }

  func (s *Service) List(userID string) []Position { return s.book.List(userID) }

// ListPaged 仓位 SQL 分页/status 过滤对外入口。
   func (s *Service) ListPaged(ctx context.Context, userID, status string, page, pageSize int) (items []Position, total, pageOut, pageSizeOut, totalPage int, err error) {
	  if s == nil || s.book == nil {
		  return nil, 0, 1, 50, 1, ErrUnavailable
	   }
	  return s.book.ListPaged(ctx, userID, status, page, pageSize)
  }

   func (s *Service) Products() []Product { return s.cat.List(true) }

   func (s *Service) OpenBooks(ctx context.Context, userID string) error {
	   if s.acct == nil {
		  return nil
	   }
	  _, err := s.acct.OpenAll(ctx, userID)
	  return err
   }

  func (s *Service) String() string {
	  if s == nil {
		   return "earn<nil>"
	  }
	  return fmt.Sprintf("earn products=%d", len(s.cat.List(false)))
  }
