package fundint

import (
	"context"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/userdir"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/shopspring/decimal"
)

var (
	ErrUnavailable = fmt.Errorf("internal transfer unavailable")
	ErrAmbiguous = fmt.Errorf("internal transfer recipient ambiguous")
	ErrMinAmount = fmt.Errorf("internal transfer below min amount")
	ErrAsset = fmt.Errorf("internal transfer asset invalid")
	ErrIdem  = fmt.Errorf("internal transfer idempotency required")
	ErrGate = fmt.Errorf("internal transfer gated")
)

// GateFunc 出金冷却/禁提/熔断闸（与链上出金同口径）。
type GateFunc func(ctx context.Context, userID string) error

type Service struct {
	led *ledger.Service
	dir userdir.Directory
	gate   GateFunc
	minAmt decimal.Decimal
	minSet bool
	lease LeaseEngine
}

   func New(led *ledger.Service, dir userdir.Directory) *Service {
	   if dir == nil {
		   dir = userdir.NewMem()
	  }
	  return &Service{led: led, dir: dir}
  }

// BindGate 挂 RequireWithdraw 等同闸。
  func (s *Service) BindGate(g GateFunc) *Service {
	   if s != nil {
		  s.gate = g
	  }
	   return s
   }

// BindMinAmount 内部转最小额（一致 risk.min_amount）。
   func (s *Service) BindMinAmount(raw string) *Service {
	   if s == nil {
		  return s
	   }
	   raw = strings.TrimSpace(raw)
	   if raw == "" {
		   s.minSet = false
		  return s
	   }
	  d, err := decimal.NewFromString(raw)
	  if err != nil || !d.IsPositive() {
		   return s
	   }
	   s.minAmt = d
	  s.minSet = true
	  return s
   }

type Req struct {
	UserID string
	ToUID  string
	ToEmail string
	ToPhone string
	Asset   string
	Amount         decimal.Decimal
	IdempotencyKey string
	Remark         string
	IntGateLeaseID string // 未过期租约；缺省且已绑引擎时 CheckPass 签发
}

type Result struct {
	Transfer *ledger.Transfer
	ToUserID string
	Channel  string // uid|email|phone
	IntGateLeaseID string // 只读产出，供他包引用
}

   func (s *Service) Send(ctx context.Context, req Req) (*ledger.Transfer, error) {
	   res, err := s.SendResolved(ctx, req)
	   if err != nil {
		  return nil, err
	  }
	  return res.Transfer, nil
  }

// SendResolved 内部转并回传解析后的收款用户与渠道。
   func (s *Service) SendResolved(ctx context.Context, req Req) (Result, error) {
	   out := Result{}
	   if s == nil || s.led == nil {
		   return out, ErrUnavailable
	   }
	   if ctx == nil {
		  ctx = context.Background()
	   }
	   uid := strings.TrimSpace(req.UserID)
	   if uid == "" {
		  return out, fmt.Errorf("user_id required")
	   }
	  idem := strings.TrimSpace(req.IdempotencyKey)
	  if idem == "" {
		  return out, ErrIdem
	  }
	  asset, err := validate.Asset(req.Asset)
	   if err != nil {
		  return out, ErrAsset
	  }
	  if !req.Amount.IsPositive() {
		  return out, ledger.ErrInvalidAmount
	  }
	   if s.minSet && req.Amount.LessThan(s.minAmt) {
		   return out, ErrMinAmount
	  }
	   // 租约引擎绑定后：闸通过必须先签发 int_gate_lease，Send 复验失败不得 SpotSend。
	  // 未绑定引擎时保持历史 GateFunc 即时判定，避免破坏既有调用面。
	  if s.lease != nil {
		   leaseID := strings.TrimSpace(req.IntGateLeaseID)
		   if leaseID == "" {
			  id, err := s.lease.CheckPass(ctx, req)
			   if err != nil {
				   return out, err
			   }
			  leaseID = id
		  }
		   if err := s.lease.AssertSend(ctx, leaseID, req); err != nil {
			  return out, err
		  }
		  out.IntGateLeaseID = leaseID
	   } else if s.gate != nil {
		   if err := s.gate(ctx, uid); err != nil {
			   return out, fmt.Errorf("%w: %v", ErrGate, err)
		   }
	  }
	   to, channel, err := s.resolve(req)
	  if err != nil {
		   return out, err
	  }
	  if strings.EqualFold(to, uid) {
		   return out,
			  ledger.ErrSelfTransfer
	  }
	   tr, err := s.led.Create(ctx, ledger.CreateReq{
		  UserID: uid, Direction: ledger.SpotSend,
		  Asset: asset, Amount: req.Amount, ToAddress: to,
		  IdempotencyKey: idem,
	   })
	   if err != nil {
		   return out, err
	   }
	   out.Transfer = tr
	   out.ToUserID = to
	  out.Channel = channel
	   if s.lease != nil && out.IntGateLeaseID != "" && tr != nil {
		  _ = s.lease.CloseSend(ctx, out.IntGateLeaseID, tr.ID)
	   }
	  return out, nil
   }

  func (s *Service) resolve(req Req) (string, string, error) {
	   uid := strings.TrimSpace(req.ToUID)
	   email := strings.TrimSpace(req.ToEmail)
	   phone := strings.TrimSpace(req.ToPhone)
	   n := 0
	   if uid != "" {
		  n++
	  }
	   if email != "" {
		  n++
	   }
	  if phone != "" {
		  n++
	  }
	   if n == 0 {
		  return "", "", userdir.ErrUnknown
	  }
	   if n > 1 {
		   // 多标识：逐项解析，必须落到同一用户。
		   var got string
		   var ch string
		  check := func(kind, val, label string) error {
			   if val == "" {
				   return nil
			  }
			   id, err := s.dir.Resolve(kind, val)
			  if err != nil {
				  return err
			   }
			  if got == "" {
				  got = id
				  ch = label
				  return nil
			   }
			   if !strings.EqualFold(got, id) {
				   return ErrAmbiguous
			  }
			   return nil
		  }
		  if err := check("uid", uid, "uid"); err != nil {
			   return "", "", err
		   }
		  if err := check("email", email, "email"); err != nil {
			  return "", "", err
		  }
		   if err := check("phone", phone, "phone"); err != nil {
			   return "", "", err
		   }
		   return got, ch, nil
	   }
	  switch {
	   case uid != "":
		  id, err := s.dir.Resolve("uid", uid)
		  return id, "uid", err
	   case email != "":
		  id, err := s.dir.Resolve("email", email)
		   return id, "email", err
	   default:
		  id, err := s.dir.Resolve("phone", phone)
		   return id, "phone", err
	   }
  }
