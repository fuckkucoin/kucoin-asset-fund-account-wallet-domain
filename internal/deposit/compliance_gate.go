// 充值入账域之核验闸门。

// Author: Jensen
// Date: 2021.01.08

package deposit

import (
	"context"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/kyt"
	"account-assets.kucoin.net/funding/internal/travelvasp"
)

// BindKYT 挂入金 KYT 筛查口。
// Observe 建单前筛查；Deny/Review/筛子错误一律失败关闭。
// 未绑定筛子时保持放行以兼容存量装配；须在 Open 时注入厂商筛子。
  func (s *Service) BindKYT(sc kyt.Screener) *Service {
	   if s != nil {
		  s.kytScreen = sc
	   }
	  return s
  }

// BindTravel 挂入金旅行规则报文服务与本对端 VASP 代码。
// 走廊门槛以上入金须 OpenIn 并推进至 screened；缺报文不得入账。
// 旅行规则为核验闸，不是账本分录；筛查拒绝不得 Create。

  func (s *Service) BindTravel(tr *travelvasp.Service, origin, peer string) *Service {
	  if s != nil {
		   s.travel = tr
		   s.originVASP = strings.ToUpper(strings.TrimSpace(origin))
		  s.peerVASP = strings.ToUpper(strings.TrimSpace(peer))
	  }
	  return s
  }

// BindTravelThresholds 挂按走廊的旅行规则金额门槛。
// Observe 仅在达到门槛时开入站报文；门槛表可热更新。
// 门槛损坏时按须开报文失败关闭，不得静默跳过。

  func (s *Service) BindTravelThresholds(book *travelvasp.ThresholdBook) *Service {
	  if s != nil {
		   s.travelThresh = book
	  }
	   return s
  }

  func (s *Service) screenInboundKYT(ctx context.Context, userID, chain, asset, amount, txHash, fromAddr string) error {
	  if s == nil || s.kytScreen == nil {
		  return nil
	  }
	  dec, reason, err := s.kytScreen.Screen(ctx, kyt.Request{
		   UserID: userID, MovementID: "dep:" + txHash, Chain: chain,
		  ToAddress: fromAddr,
		  Asset:     asset, Amount: amount,
	  })
	   if err != nil {
		  return fmt.Errorf("%w: %v", kyt.ErrUnavailable, err)
	   }
	  switch dec {
	   case kyt.Allow:
		  return nil
	  case kyt.Review:
		   return fmt.Errorf("%w: %s", kyt.ErrReview, reason)
	  default:
		  return fmt.Errorf("%w: %s", kyt.ErrDenied, reason)
	   }
  }

   func (s *Service) travelCorridor() string {
	   origin := s.originVASP
	  peer := s.peerVASP
	  if origin == "" {
		  origin = travelvasp.OriginCode()
	   }
	   if peer == "" {
		   peer = travelvasp.PeerCode()
	  }
	  originCountry, peerCountry := "XX", "XX"
	   if s.travel != nil {
		  if v, err := s.travel.GetVASP(origin); err == nil && v.Country != "" {
			  originCountry = v.Country
		   }
		  if v, err := s.travel.GetVASP(peer); err == nil && v.Country != "" {
			   peerCountry = v.Country
		  }
	  }
	  // 先国家走廊；同时保留 VASP 对作为次级键由 Resolve 回落 DEFAULT
	  return travelvasp.CorridorKey(originCountry, peerCountry)
  }

  func (s *Service) screenInboundTravel(userID, asset, amount, txHash, fromAddr string) error {
	  if s == nil || s.travel == nil {
		   return nil
	  }
	   corridor := s.travelCorridor()
	   need, minStr, err := s.travelThresh.RequiresTravel(corridor, asset, amount)
	  if err != nil {
		  return fmt.Errorf("travel threshold: %w", err)
	   }
	  if !need {
		  return nil
	  }
	   _ = minStr
	   origin := s.originVASP
	  peer := s.peerVASP
	   if origin == "" {
		   origin = travelvasp.OriginCode()
	  }
	  if peer == "" {
		   peer = travelvasp.PeerCode()
	   }
	  if fromAddr == "" {
		   fromAddr = "unknown"
	   }
	   if userID == "" {
		  userID = "unknown"
	   }
	   env, err := travelvasp.Shape(asset, amount, travelvasp.Party{
		  Name: userID, Account: fromAddr, VASP: peer,
	  }, travelvasp.Party{
		  Name: userID, Account: fromAddr, VASP: origin,
	   })
	  if err != nil {
		  return err
	   }
	   msg, err := s.travel.OpenIn(peer, "deposit-observe", "dep-trv:"+txHash, env)
	  if err != nil {
		  return err
	   }
	   if _, err := s.travel.AdvanceIn(msg.ID, "deposit-observe", travelvasp.InScreened); err != nil {
		   return err
	   }
	   return nil
   }
