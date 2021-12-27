package wdpath

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/destallow"
	"account-assets.kucoin.net/funding/internal/kyt"
	"account-assets.kucoin.net/funding/internal/kytsvc"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/travelvasp"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/internal/wdguard"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/google/uuid"
)

var (
	ErrUnavailable = fmt.Errorf("withdraw path unavailable")
	ErrUser        = fmt.Errorf("withdraw path user invalid")
	ErrAllow       = fmt.Errorf("withdraw path destination not allowlisted")
	ErrTravel = fmt.Errorf("withdraw path travel refused")
	ErrKYT       = fmt.Errorf("withdraw path kyt refused")
	ErrPreflight = fmt.Errorf("withdraw path preflight refused")
	ErrLimit     = fmt.Errorf("withdraw path dest limit refused")
	ErrAmount    = fmt.Errorf("withdraw path amount invalid")
	ErrIdem = fmt.Errorf("withdraw path idempotency conflict")
	ErrFence    = fmt.Errorf("withdraw path fence stale")
	ErrVelocity = fmt.Errorf("withdraw path velocity exceeded")
	ErrRelease = fmt.Errorf("withdraw path release invalid")
)

type Intent struct {
	ID string `json:"id"`
	Idem string `json:"idempotency_key"`
	UserID string `json:"user_id"`
	Chain  string `json:"chain"`
	Dest     string `json:"dest"`
	Asset    string `json:"asset"`
	Amount   string `json:"amount"`
	TravelID string `json:"travel_id,omitempty"`
	Allow     bool      `json:"allow"`
	Reason    string    `json:"reason,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type OpenReq struct {
	UserID string `json:"user_id"`
	Chain string `json:"chain"`
	Dest  string `json:"dest"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	VASP   string `json:"vasp"`
	Actor string `json:"actor"`
	IdempotencyKey string `json:"idempotency_key"`
	TicketID       string `json:"ticket_id,omitempty"`
	DestClass string `json:"dest_class,omitempty"`
	Risk int `json:"risk,omitempty"`
}

// pathRelease 出金路径在放行之前只核对额度，不占用冻结。
type pathRelease interface {
	AdmitPath(ctx context.Context, userID, asset, amount, chain, dest, idem, ticketID string, risk int, destClass string) error
}

type Service struct {
	mu sync.Mutex
	rows map[string]*Intent
	byIdem map[string]string
	hits   map[string]int
	allow *destallow.Book
	kyt    *kytsvc.Service
	travel *travelvasp.Service
	wd *wdguard.Service
	kv   persist.KV
	maxH int
	originVASP string
	peerVASP string
	clock    func() time.Time
	release  pathRelease
	blockNew func(user, asset string) error
	limit    LimitGate
}

  func New() *Service {
	  allow := destallow.New()
	   return &Service{
		  rows:   map[string]*Intent{},
		   byIdem: map[string]string{}, hits: map[string]int{},
		   allow: allow, kyt: kytsvc.New(kyt.Static{Mode: kyt.Allow}),
		   travel: travelvasp.New(), wd: wdguard.New().BindAllow(allow), kv: persist.NewMemKV(),
		   maxH: 0, originVASP: travelvasp.OriginCode(), peerVASP: travelvasp.PeerCode(),
		  clock: func() time.Time { return time.Now().UTC() },
	  }
  }

   func (s *Service) BindKV(kv persist.KV) *Service {
	   if s != nil && kv != nil {
		   s.kv = kv
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
	  return s
  }

   func (s *Service) SetMaxHits(n int) *Service {
	   if s != nil && n > 0 {
		  s.maxH = n
	  }
	  return s
  }

   func (s *Service) Allow() *destallow.Book { return s.allow }

   func (s *Service) BindAllow(b *destallow.Book) *Service {
	   if s != nil && b != nil {
		  s.allow = b
		  if s.wd != nil {
			   s.wd.BindAllow(b)
		   }
	   }
	   return s
  }

// BindRelease 挂放行预检。未挂时路径保持原样。

  func (s *Service) BindRelease(g pathRelease) *Service {
	  if s != nil && g != nil {
		  s.release = g
	  }
	  return s
   }

   func (s *Service) BindKYT(k *kytsvc.Service) *Service {
	  if s != nil && k != nil {
		   s.kyt = k
	  }
	  return s
  }

   func (s *Service) Open(ctx context.Context, req OpenReq) (Intent, error) {
	   if s == nil {
		  return Intent{}, ErrUnavailable
	  }
	  uid,
		   err := validate.UserID(req.UserID)
	   if err != nil {
		  return Intent{}, ErrUser
	   }
	   ch, err := validate.Chain(req.Chain)
	  if err != nil {
		   return Intent{}, err
	   }
	   as, err := validate.Asset(req.Asset)
	   if err != nil {
		  return Intent{}, err
	  }
	  if _, err := money.Parse(req.Amount); err != nil {
		  return Intent{}, ErrAmount
	   }
	   dest, err := validate.Address(ch, req.Dest)
	  if err != nil {
		  return Intent{}, err
	  }
	  idem,
		  err := validate.Idem(req.IdempotencyKey)
	  if err != nil {
		   return Intent{}, ErrIdem
	  }
	  s.mu.Lock()
	   if id, ok := s.byIdem[idem]; ok {
		  cur := s.rows[id]
		   s.mu.Unlock()
		  return cur.copy(), nil
	   }
	   if s.maxH > 0 && s.hits[uid] >= s.maxH {
		  s.mu.Unlock()
		  return Intent{}, ErrVelocity
	  }
	  s.mu.Unlock()
	  if s.blockNew != nil {
		   if err := s.blockNew(uid, as); err != nil {
			  return Intent{}, err
		   }
	   }
	   // 放行预检在白名单和后续冻账之前。没有票据或额度不足则失败关闭。
	   if s.release != nil {
		   if err := s.release.AdmitPath(ctx, uid, as, req.Amount, ch, dest, idem, strings.TrimSpace(req.TicketID), req.Risk, req.DestClass); err != nil {
			   return Intent{}, fmt.Errorf("%w: %v", ErrRelease, err)
		  }
	   }
	   if err := s.allow.RequireStrict(uid, ch, dest); err != nil {
		  return Intent{}, ErrAllow
	   }
	   actor := strings.TrimSpace(req.Actor)
	   gate, err := s.travel.GateOutbound(ctx, travelvasp.OutboundReq{
		  UserID:     uid,
		  MovementID: idem, Actor: actor,
		   Asset: as, Amount: req.Amount, Chain: ch, Dest: dest, PeerCode: req.VASP,
		  Originator:  travelvasp.Party{Name: uid, Account: uid},
		   Beneficiary: travelvasp.Party{Name: dest, Account: dest},
	   }, nil)
	   if err != nil {
		   return Intent{}, ErrTravel
	  }
	   if gate.Required && !gate.AllowBroadcast {
		   return Intent{}, ErrTravel
	  }
	  msgID := gate.Case.MsgID
	   if msgID == "" {
		  msgID = gate.Case.ID
	   }
	   kr, err := s.kyt.Screen(ctx, kyt.Request{
		   UserID: uid, MovementID: idem, Chain: ch, ToAddress: dest, Asset: as, Amount: req.Amount,
	   })
	   if err != nil || kr.Decision != kyt.Allow {
		  return Intent{}, ErrKYT
	   }
	   if s.limit != nil {
		  if lerr := s.limit.AdmitPath(ctx, LimitCheck{
			   UserID: uid,
			   Asset:  as, Amount: req.Amount, Chain: ch, Dest: dest,
			  Idem: idem, KYTHint: string(kr.Decision),
		   }); lerr != nil {
			   return Intent{}, fmt.Errorf("%w: %w", ErrLimit, lerr)
		   }
	   }
	  pf, err := s.wd.Preflight(ctx, wdguard.Check{
		  UserID: uid, Chain: ch, Dest: dest, Asset: as, Amount: req.Amount,
		  Actor: actor, MovementID: idem,
		  Pack: "kyc2", HourUTC: 12,
	   })
	   if err != nil || !pf.Allow {
		  return Intent{}, ErrPreflight
	   }
	  row := &Intent{
		  ID: uuid.NewString(), Idem: idem, UserID: uid, Chain: ch, Dest: dest,
		  Asset:  as,
		  Amount: req.Amount, TravelID: msgID, Allow: true, CreatedAt: time.Now().UTC(),
	  }
	  s.mu.Lock()
	   if s.maxH > 0 && s.hits[uid] >= s.maxH {
		  s.mu.Unlock()
		   return Intent{}, ErrVelocity
	  }
	   s.hits[uid]++
	   s.rows[row.ID] = row
	   s.byIdem[idem] = row.ID
	  s.mu.Unlock()
	   return row.copy(), nil
   }

   func (i Intent) copy() Intent { return i }

  func (s *Service) List(userID string, limit int) []Intent {
	  if s == nil {
		   return nil
	   }
	  if limit <= 0 {
		  limit = 50
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  out := make([]Intent, 0, limit)
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

const kindWD = "wdpath"

   func (s *Service) Save(ctx context.Context, in Intent) (Intent, error) {
	  if s == nil || s.kv == nil {
		   return Intent{}, ErrUnavailable
	  }
	   body, err := json.Marshal(in)
	   if err != nil {
		   return Intent{}, err
	   }
	   err = s.kv.InTx(ctx, func(ctx context.Context) error {
		  cur, ferr := s.kv.Get(ctx, kindWD, in.Idem)
		   exp := persist.Fence{}
		   if ferr == nil {
			   exp = cur.Fence
		  } else if !errors.Is(ferr, persist.ErrNotFound) {
			   return ferr
		  }
		   return s.kv.Put(ctx, persist.Row{Kind: kindWD, Key: in.Idem, Body: body}, exp)
	  })
	   if err != nil {
		   if errors.Is(err, persist.ErrStaleFence) {
			  return Intent{}, ErrFence
		  }
		   return Intent{}, err
	  }
	   return in, nil
  }
