package fundpath

import (
	"context"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/account"
	"account-assets.kucoin.net/funding/internal/addrpool"
	"account-assets.kucoin.net/funding/internal/deposit"
	"account-assets.kucoin.net/funding/internal/destallow"
	"account-assets.kucoin.net/funding/internal/hopcompgate"
	"account-assets.kucoin.net/funding/internal/hotinv"
	"account-assets.kucoin.net/funding/internal/kyt"
	"account-assets.kucoin.net/funding/internal/kytsvc"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/travelvasp"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/internal/wdguard"
	"account-assets.kucoin.net/funding/internal/xferstore"
	"github.com/google/uuid"
)

type Service struct {
	mu sync.Mutex
	halt   bool
	paths  map[string]*Path
	byIdem map[string]string
	pool   *addrpool.Pool
	inv *addrpool.Inventory
	attr *deposit.Attributor
	dep  *deposit.Service
	acc *account.Service
	xfer *xferstore.Store
	shard shardPlanner
	allow *destallow.Book
	kyt   *kytsvc.Service
	travel       *travelvasp.Service
	travelThresh *travelvasp.ThresholdBook
	wd    *wdguard.Service
	hot   *hotinv.Book
	store ledger.Store
	kv persist.KV
	// compGate 同片 hop 补偿闸；nil 表示未绑定（补偿仍执行，但不落仓）。
	compGate   *hopcompgate.Gate
	originVASP string
	peerVASP   string
	fcGate     FundCircuitGate
	clock func() time.Time
}

   func New() *Service {
	  allow := destallow.New()
	   return &Service{
		   paths: map[string]*Path{}, byIdem: map[string]string{},
		   pool: addrpool.New(nil, 24*time.Hour), inv: addrpool.NewInventory(nil, 24*time.Hour),
		  attr: deposit.NewAttributor(), allow: allow,
		   kyt: kytsvc.New(kyt.Static{Mode: kyt.Allow}), travel: travelvasp.New(),
		   wd: wdguard.New().BindAllow(allow), hot: hotinv.New(nil), xfer: xferstore.NewMem(),
		   kv: persist.NewMemKV(), originVASP: travelvasp.OriginCode(), peerVASP: travelvasp.PeerCode(),
		  clock: func() time.Time { return time.Now().UTC() },
	   }
  }

   func (s *Service) BindDeposit(dep *deposit.Service) *Service {
	   if s != nil && dep != nil {
		   s.dep = dep
		   s.attr = dep.Attributor()
		   // 入金 Observe 复用本装配的旅行规则服务，避免旁路建单。
		  if s.travel != nil {
			  dep.BindTravel(s.travel, s.originVASP, s.peerVASP)
		   }
	   }
	  return s
   }

   func (s *Service) BindAccount(acc *account.Service) *Service {
	   if s != nil {
		   s.acc = acc
	   }
	   return s
  }

   func (s *Service) BindStore(st ledger.Store) *Service {
	  if s != nil {
		   s.store = st
	  }
	   return s
   }

  func (s *Service) BindAllow(b *destallow.Book) *Service {
	  if s != nil && b != nil {
		  s.allow = b
		  if s.wd != nil {
			  s.wd.BindAllow(b)
		   }
	   }
	  return s
   }

  func (s *Service) BindXfer(x *xferstore.Store) *Service {
	   if s != nil && x != nil {
		   s.xfer = x
	  }
	  return s
   }

// BindShard 挂上分片计划。同用户同分片仍走原划转；跨分片才进入分片协议。

   func (s *Service) BindShard(p shardPlanner) *Service {
	  if s != nil {
		   s.shard = p
	   }
	  return s
   }

  func (s *Service) BindKV(kv persist.KV) *Service {
	   if s != nil && kv != nil {
		   s.kv = kv
		  s.xfer = xferstore.New(kv)
	   }
	  return s
   }

  func (s *Service) BindTravel(tr *travelvasp.Service) *Service {
	   if s != nil && tr != nil {
		  s.travel = tr
		  if s.dep != nil {
			   s.dep.BindTravel(tr, s.originVASP, s.peerVASP)
		  }
	   }
	   return s
   }

  func (s *Service) BindTravelCodes(origin, peer string) *Service {
	   if s == nil {
		  return s
	   }
	   if v := strings.ToUpper(strings.TrimSpace(origin)); v != "" {
		   s.originVASP = v
	  }
	   if v := strings.ToUpper(strings.TrimSpace(peer)); v != "" {
		  s.peerVASP = v
	  }
	  if s.dep != nil && s.travel != nil {
		  s.dep.BindTravel(s.travel, s.originVASP, s.peerVASP)
	   }
	   return s
   }

// BindTravelThresholds 把走廊旅行规则门槛挂到入金 Observe。
// 配置热更新后可再次调用；未挂门槛时金额>0 且已绑定旅行服务则一律开报文。
// 门槛表不得在留空大额放行。
  func (s *Service) BindTravelThresholds(book *travelvasp.ThresholdBook) *Service {
	  if s == nil {
		  return s
	   }
	  s.travelThresh = book
	  if s.dep != nil {
		  s.dep.BindTravelThresholds(book)
	   }
	   return s
   }

  func (s *Service) BindKYT(k *kytsvc.Service) *Service {
	   if s != nil && k != nil {
		  s.kyt = k
	  }
	  return s
  }

  func (s *Service) BindHot(h *hotinv.Book) *Service {
	   if s != nil && h != nil {
		   s.hot = h
	   }
	   return s
  }

// BindCompGate 挂载同片 hop 补偿闸。BindTransfer 失败时自动建 HopCompCase。
  func (s *Service) BindCompGate(g *hopcompgate.Gate) *Service {
	   if s != nil && g != nil {
		  s.compGate = g
	   }
	   return s
  }

  func (s *Service) BindWD(w *wdguard.Service) *Service {
	   if s != nil && w != nil {
		   s.wd = w
		  if s.allow != nil {
			   s.wd.BindAllow(s.allow)
		  }
	   }
	   return s
  }

   func (s *Service) SetHalt(on bool) {
	   if s == nil {
		   return
	  }
	  s.mu.Lock()
	   s.halt = on
	  s.mu.Unlock()
  }

  func (s *Service) Halted() bool {
	   if s == nil {
		  return true
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  return s.halt
  }

   func (s *Service) now() time.Time {
	   if s == nil || s.clock == nil {
		   return time.Now().UTC()
	  }
	  return s.clock()
   }

  func (s *Service) Pool() *addrpool.Pool {
	   if s == nil {
		  return addrpool.New(nil,
			   24*time.Hour)
	  }
	  return s.pool
   }

   func (s *Service) Inv() *addrpool.Inventory {
	  if s == nil {
		  return addrpool.NewInventory(nil, 24*time.Hour)
	  }
	  return s.inv
  }

  func (s *Service) Allow() *destallow.Book {
	  if s == nil {
		  return destallow.New()
	   }
	   return s.allow
   }

   func (s *Service) Attributor() *deposit.Attributor {
	  if s == nil {
		   return deposit.NewAttributor()
	   }
	  return s.attr
  }

  func (s *Service) Travel() *travelvasp.Service {
	  if s == nil {
		   return travelvasp.New()
	   }
	  return s.travel
   }

// Run 顺序过闸。任一步失败即关，已完成的入金不回滚。
   func (s *Service) Run(ctx context.Context, req RunReq) (Path, error) {
	   if s == nil {
		  return Path{}, ErrUnavailable
	   }
	   if s.Halted() {
		  return Path{}, ErrHalt
	   }
	  uid, err := validate.UserID(req.UserID)
	  if err != nil {
		  return Path{}, ErrUser
	  }
	  ch,
		  err := validate.Chain(req.Chain)
	  if err != nil {
		  return Path{}, ErrChain
	  }
	   as, err := validate.Asset(req.Asset)
	  if err != nil {
		   return Path{}, ErrAsset
	  }
	  idem,
		   err := validate.Idem(req.IdempotencyKey)
	  if err != nil {
		  return Path{}, ErrIdem
	  }
	   s.mu.Lock()
	  if id, ok := s.byIdem[idem]; ok {
		  cur := s.paths[id]
		   s.mu.Unlock()
		   if cur.UserID != uid || cur.Chain != ch || cur.Asset != as {
			  return Path{}, ErrIdem
		   }
		   return cur.Copy(), nil
	  }
	   s.mu.Unlock()
	  p := Path{
		   ID: uuid.NewString(), Idem: idem, UserID: uid, Chain: ch, Asset: as,
		   Amount: req.Amount, TxHash: strings.TrimSpace(req.TxHash), Dest: strings.TrimSpace(req.Dest),
		  ToBook: strings.TrimSpace(req.ToBook), State: StateOpen, Host: "deposit-withdraw.kucoin.net",
		   Region:    persist.RegionSG,
		   CreatedAt: s.now(), UpdatedAt: s.now(),
	  }
	   bind, err := s.Allocate(uid, ch)
	   if err != nil {
		  return s.fail(p, StepAlloc, ErrAlloc, err)
	   }
	  p.Address = bind
	   p.Steps = append(p.Steps, Step{Name: StepAlloc, Allow: true, Ref: bind.Address})
	   kind := deposit.AttribMemo
	  key := strings.TrimSpace(req.Memo)
	  if key == "" {
		  key = strings.TrimSpace(req.Tag)
		   kind = deposit.AttribTag
	  }
	  if key == "" {
		  key = strings.TrimSpace(req.UTXO)
		  kind = deposit.AttribUTXO
	  }
	  if key == "" {
		  key = bind.Address
		  kind = deposit.AttribAddr
	   }
	   ab, err := s.attr.BindUnique(uid, ch, kind, key)
	   if err != nil {
		  return s.fail(p,
			  StepAttrib, ErrAttrib, err)
	   }
	  p.Attrib = ab
	  p.Steps = append(p.Steps, Step{Name: StepAttrib, Allow: true, Ref: string(ab.Kind)})
	  if !req.SkipCredit {
		  tr, err := s.Credit(ctx, req, uid, ch, as)
		   if err != nil {
			  return s.fail(p, StepCredit, ErrCredit, err)
		   }
		  p.CreditID = tr
		   p.State = StateCredited
		  p.Steps = append(p.Steps,
			  Step{Name: StepCredit, Allow: true, Ref: tr})
	   }
	  if req.ToBook != "" && req.ToBook != "funding" && req.Amount != "" {
		   hopID, err := s.Hop(ctx, HopReq{
			  UserID: uid, FromBook: "funding", ToBook: req.ToBook,
			  Asset: as, Amount: req.Amount, IdempotencyKey: idem + "-hop",
		   })
		   if err != nil {
			   return s.fail(p, StepHop, ErrHop, err)
		   }
		   p.HopID = hopID
		   p.State = StateHopped
		   p.Steps = append(p.Steps, Step{Name: StepHop, Allow: true, Ref: hopID})
	   }
	   if !req.SkipEgress && req.Dest != "" {
		  eg, err := s.Egress(ctx, EgressReq{
			  UserID: uid,
			   Chain:  ch, Dest: req.Dest, Asset: as, Amount: req.Amount,
			  VASP: req.VASP, Actor: req.Actor, MovementID: p.ID,
		  })
		  if err != nil {
			  p.Steps = append(p.Steps, eg...)
			   return s.fail(p, StepWD, ErrWithdraw, err)
		   }
		   p.Steps = append(p.Steps, eg...)
		   p.State = StateCleared
	   }
	  p.UpdatedAt = s.now()
	  s.mu.Lock()
	   s.paths[p.ID] = &p
	   s.byIdem[idem] = p.ID
	  s.mu.Unlock()
	   return p.Copy(), nil
   }

   func (s *Service) fail(p Path, name StepName, wrap, err error) (Path, error) {
	  p.Steps = append(p.Steps, Step{Name: name, Allow: false, Reason: err.Error()})
	   p.State = StateBlocked
	   p.UpdatedAt = s.now()
	   s.mu.Lock()
	  s.paths[p.ID] = &p
	   if p.Idem != "" {
		  s.byIdem[p.Idem] = p.ID
	   }
	   s.mu.Unlock()
	  return p.Copy(), wrap
   }

  func (s *Service) Get(id string) (Path, error) {
	  if s == nil {
		  return Path{}, ErrUnavailable
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   p,
		  ok := s.paths[strings.TrimSpace(id)]
	   if !ok {
		  return Path{}, ErrStep
	   }
	  return p.Copy(), nil
   }

  func (s *Service) List(userID string, limit int) []Path {
	  if s == nil {
		  return nil
	  }
	  if limit <= 0 {
		  limit = 50
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   out := make([]Path, 0, limit)
	  for _, p := range s.paths {
		   if userID != "" && p.UserID != userID {
			  continue
		  }
		  out = append(out, p.Copy())
		  if len(out) >= limit {
			   break
		   }
	   }
	   return out
  }
