package fundintgate

import (
	"context"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/fundint"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/userdir"
	"github.com/shopspring/decimal"
)

// GateFuncAdapter 将 fundint.GateFunc 适配为 GateEval。

type GateFuncAdapter struct {
	Fn fundint.GateFunc
}

// Allow 调用底层闸。

  func (a GateFuncAdapter) Allow(ctx context.Context, userID string) error {
	  if a.Fn == nil {
		  return nil
	   }
	  return a.Fn(ctx, userID)
  }

// DirectoryResolver 只读消费 userdir.Directory。
type DirectoryResolver struct {
	Dir userdir.Directory
}

// Resolve 复用 fundint 多标识一致规则：多标识必须落到同一用户。

  func (r DirectoryResolver) Resolve(_ context.Context, toUID, toEmail, toPhone string) (ResolveResult, error) {
	  if r.Dir == nil {
		   return ResolveResult{},
			   ErrResolveFail
	  }
	  uid := strings.TrimSpace(toUID)
	   email := strings.TrimSpace(toEmail)
	  phone := strings.TrimSpace(toPhone)
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
		  return ResolveResult{}, userdir.ErrUnknown
	  }
	   hint := ComposeToHint(uid, email, phone)
	   if n > 1 {
		   var got, ch string
		  check := func(kind, val, label string) error {
			   if val == "" {
				   return nil
			  }
			   id, err := r.Dir.Resolve(kind, val)
			   if err != nil {
				  return err
			   }
			  if got == "" {
				   got = id
				   ch = label
				   return nil
			   }
			   if !strings.EqualFold(got, id) {
				  return fundint.ErrAmbiguous
			  }
			   return nil
		  }
		  if err := check("uid", uid, "uid"); err != nil {
			  return ResolveResult{}, err
		  }
		   if err := check("email", email, "email"); err != nil {
			  return ResolveResult{}, err
		  }
		  if err := check("phone", phone, "phone"); err != nil {
			  return ResolveResult{}, err
		   }
		  return ResolveResult{ToUserID: got, Channel: ch, Hint: hint}, nil
	   }
	  switch {
	   case uid != "":
		  id, err := r.Dir.Resolve("uid", uid)
		  return ResolveResult{ToUserID: id, Channel: "uid", Hint: hint}, err
	  case email != "":
		   id, err := r.Dir.Resolve("email", email)
		   return ResolveResult{ToUserID: id, Channel: "email", Hint: hint}, err
	  default:
		  id, err := r.Dir.Resolve("phone", phone)
		   return ResolveResult{ToUserID: id, Channel: "phone", Hint: hint}, err
	   }
  }

// LedgerSender 调用 ledger.Create + SpotSend，不改公式。
type LedgerSender struct {
	Led *ledger.Service
}

// SpotSend 走既有 Create 接口。
  func (s LedgerSender) SpotSend(ctx context.Context, from, to, asset, amount, idem string) (string, error) {
	   if s.Led == nil {
		  return "", ErrUnavailable
	   }
	   d, err := decimal.NewFromString(amount)
	   if err != nil {
		   return "", err
	  }
	  tr, err := s.Led.Create(ctx, ledger.CreateReq{
		  UserID:    from,
		  Direction: ledger.SpotSend,
		   Asset:     asset, Amount: d, ToAddress: to,
		   IdempotencyKey: idem,
	  })
	   if err != nil {
		   return "", err
	   }
	   if tr == nil {
		   return "", fmt.Errorf("%w: empty transfer", ErrUnavailable)
	   }
	  return tr.ID, nil
   }

// StaticResolver 用固定解析器。
type StaticResolver struct {
	ToUserID string
	Channel  string
	Err      error
}

// Resolve 返回固定结果。
   func (s StaticResolver) Resolve(_ context.Context, _, _, _ string) (ResolveResult, error) {
	  if s.Err != nil {
		  return ResolveResult{}, s.Err
	  }
	  return ResolveResult{ToUserID: s.ToUserID, Channel: firstNonEmpty(s.Channel, "uid"), Hint: "static"}, nil
  }

// StaticGate 用闸。
type StaticGate struct {
	Err error
}

// Allow 返回预设错误。

   func (g StaticGate) Allow(_ context.Context,
	   _ string) error {
	   return g.Err
  }

// RecordingSender 记录调用次数， fail-closed。
type RecordingSender struct {
	Calls int
	ID    string
	Err   error
}

// SpotSend 计数。
   func (r *RecordingSender) SpotSend(_ context.Context, _, _, _, _, _ string) (string, error) {
	  r.Calls++
	   if r.Err != nil {
		   return "", r.Err
	  }
	  if r.ID == "" {
		  r.ID = "xfer-test"
	   }
	   return r.ID, nil
  }

// MapResolver 按 ToUID 映射。

type MapResolver struct {
	Users map[string]string
}

// Resolve 仅消费 ToUID。

  func (m MapResolver) Resolve(_ context.Context, toUID, toEmail, toPhone string) (ResolveResult, error) {
	  key := strings.TrimSpace(toUID)
	  ch := "uid"
	  if key == "" {
		  key = strings.TrimSpace(toEmail)
		   ch = "email"
	   }
	  if key == "" {
		   key = strings.TrimSpace(toPhone)
		   ch = "phone"
	  }
	  if m.Users == nil {
		   return ResolveResult{}, ErrResolveFail
	   }
	  id, ok := m.Users[key]
	  if !ok {
		  return ResolveResult{}, ErrResolveFail
	   }
	   return ResolveResult{ToUserID: id, Channel: ch, Hint: ComposeToHint(toUID, toEmail, toPhone)}, nil
  }
