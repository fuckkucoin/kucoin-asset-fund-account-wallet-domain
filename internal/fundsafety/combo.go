package fundsafety

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// HaltViewer 读托管熔断（只读）。

type HaltViewer interface {
	Halted(ctx context.Context) (bool, error)
	CheckActive(ctx context.Context) error
	CheckScope(ctx context.Context,
		chain, asset string) error
}

// ReconViewer 读对账未关差异（只读）。

type ReconViewer interface {
	BlocksUser(userID string) bool
	OpenCount(userID string) int
}

// ComboVerdict 熔断 + 对账 + 资金安全进程内闸的组合结论。
type ComboVerdict struct {
	Allow     bool `json:"allow"`
	LocalHalt bool `json:"local_halt"`
	CustodyHalt  bool `json:"custody_halt"`
	ReconBlocked bool `json:"recon_blocked"`
	OpenTickets  int  `json:"open_tickets,omitempty"`
	Reason string `json:"reason,omitempty"`
	Policy string `json:"policy"`
}

// BindCombo 装配熔断与对账读口，供 ClaimHash / 运营读面复用。

   func (s *Service) BindCombo(halt HaltViewer, recon ReconViewer) *Service {
	   if s != nil {
		  s.haltView = halt
		  s.reconView = recon
	   }
	   return s
   }

// ComboOf 组合策略表：进程内闸 ∪ 托管熔断 ∪ 对账未关 → 拒绝新资金动作。
// 事故台一眼看到为何挡。读熔断失败按已熔断（失败即关）；对账未关仅挡对应用户。未装配 halt/recon 时只看进程内闸，不得假装全绿。

  func (s *Service) ComboOf(ctx context.Context, userID string) ComboVerdict {
	   v := ComboVerdict{Allow: true, Policy: "halt_or_recon_blocks"}
	   if s == nil {
		   v.Allow = false
		   v.Reason = ErrUnavailable.Error()
		   return v
	  }
	   if s.Halted() {
		  v.Allow = false
		  v.LocalHalt = true
		  v.Reason = ErrHalt.Error()
	   }
	   if s.haltView != nil {
		  if err := s.haltView.CheckActive(ctx); err != nil {
			  v.Allow = false
			   v.CustodyHalt = true
			  if v.Reason == "" {
				   v.Reason = err.Error()
			   }
		  } else if on, err := s.haltView.Halted(ctx); err != nil {
			   v.Allow = false
			   v.CustodyHalt = true
			  if v.Reason == "" {
				   v.Reason = errors.Join(ErrHalt, err).Error()
			  }
		   } else if on {
			  v.Allow = false
			   v.CustodyHalt = true
			   if v.Reason == "" {
				   v.Reason = ErrHalt.Error()
			  }
		  }
	   }
	  if s.reconView != nil && userID != "" {
		   n := s.reconView.OpenCount(userID)
		  v.OpenTickets = n
		   if s.reconView.BlocksUser(userID) {
			   v.Allow = false
			  v.ReconBlocked = true
			   if v.Reason == "" {
				   v.Reason = fmt.Sprintf("reconcile open tickets=%d", n)
			   }
		  }
	  }
	   return v
   }

// GuardCombo 新资金动作前组合闸；拒绝时返回 ErrHalt 或说明性错误。

  func (s *Service) GuardCombo(ctx context.Context, userID string) error {
	  v := s.ComboOf(ctx, userID)
	   if v.Allow {
		  return nil
	  }
	  if v.LocalHalt || v.CustodyHalt {
		   return ErrHalt
	   }
	  return fmt.Errorf("%w: %s", ErrHalt, v.Reason)
   }

// GuardComboScope 带链/资产作用域的组合闸；链资产空时退回 GuardCombo。

  func (s *Service) GuardComboScope(ctx context.Context, userID, chain, asset string) error {
	   if err := s.GuardCombo(ctx, userID); err != nil {
		  return err
	   }
	  if s == nil || s.haltView == nil {
		  return nil
	  }
	   if chain == "" && asset == "" {
		   return nil
	   }
	  if err := s.haltView.CheckScope(ctx, chain, asset); err != nil {
		   return ErrHalt
	   }
	  return nil
  }

// ComboOfScope 带链/资产的组合结论；链资空时等同 ComboOf。
// 功能：事故台按目标链资看到挡因，避免全局结论掩盖 pair/asset 闸。
// 重点：先走 ComboOf，再叠加 CheckScope；读失败按已熔断。
// 风险点：链/资产拼写错误会漏匹配作用域键，须 ActiveFlags。
   func (s *Service) ComboOfScope(ctx context.Context, userID, chain, asset string) ComboVerdict {
	  v := s.ComboOf(ctx, userID)
	   chain = strings.TrimSpace(chain)
	  asset = strings.TrimSpace(asset)
	  if s == nil || s.haltView == nil || (chain == "" && asset == "") {
		   return v
	   }
	  if err := s.haltView.CheckScope(ctx, chain, asset); err != nil {
		   v.Allow = false
		   v.CustodyHalt = true
		  if v.Reason == "" {
			   v.Reason = err.Error()
		  }
	   }
	   return v
   }
