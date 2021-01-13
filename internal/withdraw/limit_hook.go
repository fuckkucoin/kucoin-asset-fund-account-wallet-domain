// 未挂载引擎时保持原路径。挂载后限额失败直接返回，不改写成 KYT 或风控拒绝。
// 冻账失败归还占用；取消未广播的出金同样释放。

// Author: Bronson
// Date: 2021.01.13

package withdraw

import (
	"context"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/destlimit"
)

// LimitAdmit 预检入参。KYTHint 只传给限额作特征。
type LimitAdmit struct {
	UserID string
	Asset  string
	Amount string
	Chain string
	Dest  string
	Idem  string
	KYTHint string
}

// LimitVerdict 占用结果。限额拒绝时 error 不为空，本结构可为空。
type LimitVerdict struct {
	HoldID string
	ClusterID string
	Tier   string
	Limit  string
	Reused bool
}

// LimitAdmitter 目的地限额占用口。由 destlimit.Engine 适配。

type LimitAdmitter interface {
	Admit(ctx context.Context, in LimitAdmit) (LimitVerdict, error)
	ReleaseAdmit(ctx context.Context, holdID, reason string) error
}

type limitBridge struct {
	eng *destlimit.Engine
}

   func (b limitBridge) Admit(ctx context.Context, in LimitAdmit) (LimitVerdict, error) {
	   if b.eng == nil {
		   return LimitVerdict{}, destlimit.ErrUnavailable
	  }
	   h, err := b.eng.Admit(ctx, destlimit.AdmitReq{
		   UserID: in.UserID, Asset: in.Asset, Amount: in.Amount,
		  Chain: in.Chain, Dest: in.Dest, Idem: in.Idem, KYTHint: in.KYTHint,
	   })
	   if err != nil {
		  return LimitVerdict{}, err
	   }
	   return LimitVerdict{HoldID: h.ID, ClusterID: h.ClusterID, Tier: h.Tier, Limit: h.Limit, Reused: h.Reused}, nil
   }

  func (b limitBridge) ReleaseAdmit(ctx context.Context, holdID, reason string) error {
	  if b.eng == nil {
		   return destlimit.ErrUnavailable
	  }
	  return b.eng.ReleaseAdmit(ctx, holdID, reason)
  }

// BindLimitAdmitter 挂上限额预检。nil 表示拆掉挂载。

  func (s *Service) BindLimitAdmitter(g LimitAdmitter) *Service {
	   if s == nil {
		   return s
	   }
	  s.limit = g
	   if s.limitHolds == nil {
		  s.limitHolds = map[string]string{}
	  }
	   return s
  }

// BindRuntimeLimit 从运行时取出已挂载的限额引擎。

  func BindRuntimeLimit(s *Service, rt *app.Runtime) *Service {
	  if s == nil || rt == nil {
		  return s
	   }
	   eng := destlimit.Attach(rt)
	   return s.BindLimitAdmitter(limitBridge{eng: eng})
  }

  func (s *Service) rememberLimit(transferID, holdID string) {
	  if s == nil || transferID == "" || holdID == "" {
		  return
	   }
	   if s.limitHolds == nil {
		  s.limitHolds = map[string]string{}
	   }
	  s.limitHolds[transferID] = holdID
  }

  func (s *Service) releaseLimit(ctx context.Context, transferID, reason string) {
	   if s == nil || s.limit == nil || transferID == "" {
		  return
	  }
	   id := s.limitHolds[transferID]
	   if id == "" {
		  return
	   }
	  _ = s.limit.ReleaseAdmit(ctx, id, reason)
	  delete(s.limitHolds,
		   transferID)
  }
