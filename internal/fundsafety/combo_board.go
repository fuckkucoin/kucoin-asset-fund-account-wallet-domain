package fundsafety

import (
	"context"
)

// LaneViewer 读充停/提停/清扫入口闸位（只读）。

type LaneViewer interface {
	CheckLane(ctx context.Context, lane string, chain, asset string) error
	CheckPair(ctx context.Context, chain, asset string) error
}

// ComboBoard 分入口组合结论。

type ComboBoard struct {
	UserID   string       `json:"user_id,omitempty"`
	Chain    string       `json:"chain,omitempty"`
	Asset    string       `json:"asset,omitempty"`
	Overall  ComboVerdict `json:"overall"`
	Deposit  ComboVerdict `json:"deposit"`
	Withdraw ComboVerdict `json:"withdraw"`
	Sweep  ComboVerdict `json:"sweep"`
	Policy string       `json:"policy"`
	Host   string       `json:"host"`
}

// BindLane 装配入口闸读口（可选）；未装配时 Board 退回 GuardCombo / GuardComboScope。
  func (s *Service) BindLane(lane LaneViewer) *Service {
	   if s != nil {
		   s.laneView = lane
	  }
	  return s
   }

// BoardOf 组合闸位板：overall 走 ComboOf；各入口叠加链/资产/组合作用域。
// 事故台按充/提/清扫一眼看到挡因。读熔断失败按已熔断；对账未关仅挡对应用户。未装配 lane 时入口结论与 overall 一致，不得假装细分放行。
  func (s *Service) BoardOf(ctx context.Context, userID, chain, asset string) ComboBoard {
	  board := ComboBoard{
		   UserID: userID, Chain: chain, Asset: asset,
		   Policy: "halt_or_recon_blocks",
		   Host: "custody-boundary-lab.kucoin.net",
	  }
	  overall := s.ComboOfScope(ctx, userID, chain, asset)
	   board.Overall = overall
	  board.Deposit = overall
	   board.Withdraw = overall
	   board.Sweep = overall
	  if s == nil {
		  return board
	   }
	   // 清扫：任一作用域 On 即挡（CheckActive 语义）。
	   if s.haltView != nil {
		   if err := s.haltView.CheckActive(ctx); err != nil {
			   board.Sweep.Allow = false
			  board.Sweep.CustodyHalt = true
			   if board.Sweep.Reason == "" {
				  board.Sweep.Reason = err.Error()
			   }
		  }
	   }
	   if chain != "" || asset != "" {
		  if err := s.GuardComboScope(ctx, userID, chain, asset); err != nil {
			  for _, ptr := range []*ComboVerdict{&board.Deposit, &board.Withdraw} {
				  ptr.Allow = false
				   ptr.CustodyHalt = true
				  if ptr.Reason == "" {
					  ptr.Reason = err.Error()
				   }
			   }
		   }
	   }
	   if s.laneView != nil {
		  if err := s.laneView.CheckPair(ctx, chain, asset); err != nil {
			  for _, ptr := range []*ComboVerdict{&board.Deposit, &board.Withdraw, &board.Sweep} {
				   ptr.Allow = false
				   ptr.CustodyHalt = true
				   if ptr.Reason == "" {
					  ptr.Reason = err.Error()
				  }
			  }
		   }
		   for _, lane := range []struct {
			  name string
			   dst *ComboVerdict
		  }{
			   {"deposit", &board.Deposit},
			  {"withdraw", &board.Withdraw},
			   {"sweep", &board.Sweep},
		   } {
			   ch, as := chain, asset
			  if lane.name == "sweep" {
				  ch, as = "", ""
			  }
			   if err := s.laneView.CheckLane(ctx, lane.name, ch, as); err != nil {
				   lane.dst.Allow = false
				  lane.dst.CustodyHalt = true
				   if lane.dst.Reason == "" {
					   lane.dst.Reason = err.Error()
				  }
			  }
		   }
	   }
	  return board
  }

// GuardBoardLane 按入口做组合闸；lane 空等同 GuardComboScope。

   func (s *Service) GuardBoardLane(ctx context.Context, lane, userID, chain, asset string) error {
	  board := s.BoardOf(ctx, userID, chain, asset)
	   var v ComboVerdict
	  switch lane {
	   case "deposit":
		  v = board.Deposit
	  case "withdraw":
		  v = board.Withdraw
	  case "sweep":
		   v = board.Sweep
	   default:
		  v = board.Overall
	  }
	  if v.Allow {
		  return nil
	   }
	   if v.LocalHalt || v.CustodyHalt {
		  return ErrHalt
	  }
	  return ErrHalt
   }
