// 出金冻账前的放行闸。闸只决定能不能冻；失败不改冻结态，也不为链上 live hash 自动解冻。

// Author: Talon
// Date: 2021.01.13

package withdraw

import "context"

// ReleaseAsk 冻账前交给放行台的要素。金额是本次想冻结的数量。
type ReleaseAsk struct {
	UserID string
	Asset  string
	Amount string
	Chain     string
	Dest      string
	Idem      string
	TicketID  string
	DestClass string
	Risk int
}

// ReleaseGrant 放行台已经占用的额度。账本没冻成时按幂等键退回占用。

type ReleaseGrant struct {
	TicketID string
	Idem     string
	Amount string
}

type releaseGate interface {
	BeforeFreeze(ctx context.Context, ask ReleaseAsk) (ReleaseGrant, error)
	CommitFreeze(ctx context.Context, grant ReleaseGrant) error
	AbortFreeze(ctx context.Context, grant ReleaseGrant) error
}

// BindRelease 挂放行闸。未挂时出金保持原路径。

  func (s *Service) BindRelease(g releaseGate) *Service {
	   if s != nil && g != nil {
		   s.release = g
	  }
	   return s
   }
