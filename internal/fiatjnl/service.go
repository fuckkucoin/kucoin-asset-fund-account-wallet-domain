// @FileName: service.go
// @Summary: 法币进出金凭证。onramp 桥出用户进；offramp 用户出桥进。KYC 未过失败即关。
// 扩展：集成凭证世代追踪（GenTracker）与不平衡挡写闸（BlockGate）。
// Post 成功后异步同步世代到 fiatbalgate；CheckBlock 钩子在写入前校验是否存在不平衡案。
// @Author: Draven
// @Created: 2021.05.14

package fiatjnl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/fiatrail"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/validate"
)

var (
	ErrUnavailable = fmt.Errorf("fiat journal unavailable")
	ErrUser = fmt.Errorf("fiat journal user invalid")
	ErrAmount = fmt.Errorf("fiat journal amount invalid")
	ErrKind = fmt.Errorf("fiat journal kind invalid")
	ErrKYC   = fmt.Errorf("fiat journal kyc refused")
	ErrIdem  = fmt.Errorf("fiat journal idempotency conflict")
	ErrFence = fmt.Errorf("fiat journal fence stale")
	ErrJournal = fmt.Errorf("fiat journal unbalanced")
	// ErrBlocked Post 路径被不平衡案挡写（由 BlockGate 注入）。
	ErrBlocked = fmt.Errorf("fiat journal blocked by imbalance")
)

// BlockGate 不平衡挡写闸接口；由 fiatbalgate.Engine 实现。
// fiatjnl.Service 通过此接口在 Post 前校验是否存在活跃不平衡案。
// 未注入（nil）时忽略挡写校验（向后兼容）。

type BlockGate interface {
	// IsBlocked 快速判断指定资产是否被挡写。true 时 Post 必须拒绝。
	IsBlocked(ctx context.Context, asset string, genSeq uint64) (bool, error)
	// NotifyGen 将新世代快照通知给 BlockGate（Post 成功后异步调用）。
	NotifyGen(ctx context.Context, gen JournalGen) error
}

type Entry struct {
	ID     string        `json:"id"`
	Idem   string        `json:"idempotency_key"`
	UserID string        `json:"user_id"`
	Kind   fiatrail.Kind `json:"kind"`
	Asset  string        `json:"asset"`
	Amount string        `json:"amount"`
	Currency  string       `json:"currency"`
	KYC       bool         `json:"kyc"`
	Lines     []money.Line `json:"-"`
	CreatedAt time.Time    `json:"created_at"`
}

type PostReq struct {
	UserID string `json:"user_id"`
	Kind string `json:"kind"`
	Asset string `json:"asset"`
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
	KYCTier  string `json:"kyc_tier"`
	IdempotencyKey string `json:"idempotency_key"`
}

type Service struct {
	mu     sync.Mutex
	rows   map[string]*Entry
	byIdem map[string]string
	kyc map[string]bool
	kv    persist.KV
	clock func() time.Time
	// gen 世代追踪器；每次 Post/Adjust 成功后抬升世代号。
	gen *GenTracker
	// gate 不平衡挡写闸；nil 时不做挡写校验（向后兼容）。
	gate BlockGate
	led  *ledger.Service
}

// ServiceOption Service 配置选项。

type ServiceOption func(*Service)

func (s *Service) BindLedger(led *ledger.Service) *Service {
	if s != nil {
		s.led = led
	}
	return s
}

func kycTierOK(tier string) bool {
	tier = strings.TrimSpace(tier)
	if tier == "" || tier == "0" || strings.EqualFold(tier, "none") {
		return false
	}
	if n, err := strconv.Atoi(tier); err == nil {
		return n >= 1
	}
	return strings.EqualFold(tier, "pass") || strings.EqualFold(tier, "verified")
}

// WithBlockGate 注入不平衡挡写闸（由 fiatbalgate.Engine 适配实现）。

   func WithBlockGate(g BlockGate) ServiceOption {
	  return func(s *Service) { s.gate = g }
  }

// WithServiceClock 注入时钟。

  func WithServiceClock(fn func() time.Time) ServiceOption {
	  return func(s *Service) {
		  if fn != nil {
			   s.clock = fn
		  }
	  }
}

  func New(opts ...ServiceOption) *Service {
	  s := &Service{
		   rows:   map[string]*Entry{},
		   byIdem: map[string]string{}, kyc: map[string]bool{},
		   kv: persist.NewMemKV(), clock: func() time.Time { return time.Now().UTC() },
		  gen: newGenTracker(),
	  }
	  for _, o := range opts {
		  o(s)
	  }
	  return s
   }

// ActiveGenSeq 返回当前最新世代号（只读，无锁）。供 fiatbalgate 只读引用。
  func (s *Service) ActiveGenSeq() GenSeq {
	   if s == nil {
		  return 0
	   }
	   return s.gen.ActiveSeq()
  }

// GenHistory 返回最近 n 条世代快照副本。n<=0 返回全部缓存。
   func (s *Service) GenHistory(n int) []JournalGen {
	   if s == nil {
		  return nil
	   }
	  return s.gen.History(n)
   }

// PollGens 返回 seq > sinceSeq 的世代快照（供 fiatbalgate.GenProducer 使用）。

  func (s *Service) PollGens(_ context.Context, sinceSeq uint64, limit int) ([]JournalGen, error) {
	  if s == nil {
		  return nil, nil
	   }
	   all := s.gen.History(0)
	  var out []JournalGen
	  for _, g := range all {
		  if uint64(g.Seq) > sinceSeq {
			   out = append(out, g)
			   if limit > 0 && len(out) >= limit {
				   break
			   }
		  }
	   }
	   return out, nil
  }

  func (s *Service) MarkKYC(userID string, ok bool) error {
	  if s == nil {
		  return ErrUnavailable
	  }
	   uid, err := validate.UserID(userID)
	   if err != nil {
		   return ErrUser
	   }
	  s.mu.Lock()
	   s.kyc[uid] = ok
	   s.mu.Unlock()
	   return nil
   }

   func (s *Service) Post(ctx context.Context, req PostReq) (Entry, error) {
	  if s == nil {
		  return Entry{}, ErrUnavailable
	   }
	  uid, err := validate.UserID(req.UserID)
	   if err != nil {
		  return Entry{}, ErrUser
	   }
	   kind, err := fiatrail.ParseKind(req.Kind)
	   if err != nil {
		   return Entry{}, ErrKind
	   }
	   as, err := validate.Asset(req.Asset)
	  if err != nil {
		  return Entry{}, err
	  }
	  amt,
		  err := money.Parse(req.Amount)
	   if err != nil {
		   return Entry{}, ErrAmount
	   }
	   idem, err := validate.Idem(req.IdempotencyKey)
	   if err != nil {
		  return Entry{}, ErrIdem
	  }
	   s.mu.Lock()
	   if id, ok := s.byIdem[idem]; ok {
		  cur := s.rows[id]
		  s.mu.Unlock()
		  if cur.UserID != uid || cur.Amount != amt.String() || cur.Kind != kind {
			  return Entry{}, ErrIdem
		  }
		   return cur.copy(), nil
	   }
	   if !s.kyc[uid] && !kycTierOK(req.KYCTier) {
		  s.mu.Unlock()
		  return Entry{}, ErrKYC
	  }
	  s.mu.Unlock()
	  if s.led != nil {
		  dir := ledger.FiatOnramp
		  if kind == fiatrail.KindOfframp {
			  dir = ledger.FiatOfframp
		  }
		  dec, err := ledger.ParseAmount(amt.String())
		  if err != nil {
			  return Entry{}, err
		  }
		  if _, err := s.led.Create(ctx, ledger.CreateReq{
			  UserID: uid, Direction: dir, Asset: as, Amount: dec, IdempotencyKey: idem,
		  }); err != nil {
			  return Entry{}, err
		  }
	  }
	   var lines []money.Line
	   if kind == fiatrail.KindOnramp {
		  lines = money.CreditPair(as, amt)
	  } else {
		   lines = money.DebitPair(as, amt)
	   }
	   if err := money.Balanced(lines); err != nil {
		   return Entry{}, ErrJournal
	   }
	  left, _ := money.ParseBook("0")
	   right, _ := money.ParseBook("0")
	  for _, ln := range lines {
		   left, _ = left.Add(ln.Debit)
		  right, _ = right.Add(ln.Credit)
	   }
	   if err := money.Conserve(left, right); err != nil {
		  return Entry{}, ErrJournal
	   }
	   // 挡写校验：存在未清不平衡案时拒绝新 Post（fail-closed）。
	   if s.gate != nil {
		  genSeq := uint64(s.gen.ActiveSeq())
		   blocked, gateErr := s.gate.IsBlocked(ctx, as, genSeq)
		  if gateErr != nil && blocked {
			  return Entry{}, fmt.Errorf("%w: %s", ErrBlocked, gateErr.Error())
		  }
		   if blocked {
			   return Entry{}, ErrBlocked
		  }
	  }
	   row := &Entry{
		  ID: uuid.NewString(), Idem: idem, UserID: uid, Kind: kind, Asset: as,
		  Amount: amt.String(), Currency: strings.ToUpper(strings.TrimSpace(req.Currency)),
		   KYC:   true,
		   Lines: lines, CreatedAt: s.clock(),
	   }
	  s.mu.Lock()
	   s.rows[row.ID] = row
	  s.byIdem[idem] = row.ID
	  s.mu.Unlock()
	  // 世代追踪：Post 成功后抬升世代号。
	   gen := s.gen.BumpGen(row.ID, "post", as, s.clock)
	   // 异步通知 fiatbalgate 世代快照（失败不影响主路径）。
	  if s.gate != nil {
		   go func() {
			   _ = s.gate.NotifyGen(context.Background(), gen)
		  }()
	  }
	  _ = ctx
	  return row.copy(), nil
  }

   func (e Entry) copy() Entry { return e }

  func (s *Service) List(userID string, limit int) []Entry {
	   if s == nil {
		   return nil
	  }
	   if limit <= 0 {
		   limit = 50
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  out := make([]Entry, 0, limit)
	  for _, r := range s.rows {
		  if userID != "" && r.UserID != userID {
			   continue
		   }
		   out = append(out, r.copy())
		   if len(out) >= limit {
			  break
		  }
	  }
	  return out
  }

const kindFiat = "fiatjnl"

   func (s *Service) Save(ctx context.Context, e Entry) (Entry, error) {
	   if s == nil || s.kv == nil {
		  return Entry{}, ErrUnavailable
	  }
	  body, err := json.Marshal(e)
	   if err != nil {
		   return Entry{}, err
	  }
	  err = s.kv.InTx(ctx, func(ctx context.Context) error {
		   cur, ferr := s.kv.Get(ctx, kindFiat, e.Idem)
		  exp := persist.Fence{}
		  if ferr == nil {
			  exp = cur.Fence
		  } else if !errors.Is(ferr, persist.ErrNotFound) {
			  return ferr
		  }
		   return s.kv.Put(ctx, persist.Row{Kind: kindFiat, Key: e.Idem, Body: body}, exp)
	  })
	   if err != nil {
		  if errors.Is(err, persist.ErrStaleFence) {
			  return Entry{}, ErrFence
		   }
		  return Entry{}, err
	  }
	   return e, nil
   }
