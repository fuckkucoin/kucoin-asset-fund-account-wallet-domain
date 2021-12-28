package wdpath

import (
	"context"
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/destlimit"
	"account-assets.kucoin.net/funding/internal/travelvasp"
)

  func init() {
	  apierr.Register(ErrLimit,
		   http.StatusForbidden)
   }

// LimitCheck 路径预检入参。KYTHint 已是放行结论，只作图谱特征。

type LimitCheck struct {
	UserID string
	Asset  string
	Amount string
	Chain  string
	Dest   string
	Idem    string
	KYTHint string
}

// LimitGate 限额占用口。失败即关，不得当成预检通过。
type LimitGate interface {
	AdmitPath(ctx context.Context,
		in LimitCheck) error
}

type pathLimit struct {
	eng *destlimit.Engine
}

   func (p pathLimit) AdmitPath(ctx context.Context, in LimitCheck) error {
	   if p.eng == nil {
		   return destlimit.ErrUnavailable
	   }
	  _, err := p.eng.Admit(ctx, destlimit.AdmitReq{
		   UserID: in.UserID, Asset: in.Asset, Amount: in.Amount,
		  Chain: in.Chain, Dest: in.Dest, Idem: in.Idem, KYTHint: in.KYTHint,
	   })
	   return err
   }

  func (s *Service) BindLimit(g LimitGate) *Service {
	  if s != nil {
		   s.limit = g
	  }
	  return s
  }

  func (s *Service) BindRuntimeLimit(rt *app.Runtime) *Service {
	   if s == nil || rt == nil {
		   return s
	   }
	   return s.BindLimit(pathLimit{eng: destlimit.Attach(rt)})
   }

// BindTravelBook 调整旅行规则门槛，供路径把未达门槛的金额放行到后续闸。

  func (s *Service) BindTravelBook(book *travelvasp.ThresholdBook) *Service {
	   if s != nil && s.travel != nil && book != nil {
		  s.travel.BindThresholds(book)
	   }
	  return s
  }
