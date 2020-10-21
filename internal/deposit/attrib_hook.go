// @FileName: attrib_hook.go
// @Summary: 观察前的归因钩子。默认关闭。
// 关闭时 Observe 不进入本文件的判定，原有备注、错网、巡检和 dep: 熔断保持原样。
// 打开后，结论没有入账意图就停在账本 Create 之前。
// @Author: Bronson
// @Created: 2020.10.12

package deposit

import (
	"context"
	"errors"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/wallet"
)

// ErrAttribHold 归因拒绝入账。不是账本错误，也不是巡检差额。

var ErrAttribHold = errors.New("attribution refused credit")

// AttributionGate 观察线程上的可选闸。实现不得调用 ledger.Apply。

type AttributionGate interface {
	Gate(ctx context.Context, req ObserveReq, seen wallet.Observation) error
}

   func (s *Service) prepareAttribution(rt *app.Runtime) AttributionGate {
	  if s == nil {
		   return nil
	  }
	  s.attribOnce.Do(func() {
		  s.attribEng = mountAttribution(s, rt)
	  })
	  return s.attribEng
   }

// EnableAttribution 在本进程打开钩子。别的进程不受影响。
  func EnableAttribution(s *Service, rt *app.Runtime) {
	  if s == nil {
		  return
	   }
	   _ = s.prepareAttribution(rt)
	  s.attribOn.Store(true)
   }

// SetAttributionRequired 运营开关。关掉之后观察线程回到原路径。
   func (s *Service) SetAttributionRequired(on bool, rt *app.Runtime) {
	  if s == nil {
		   return
	  }
	  if on {
		  _ = s.prepareAttribution(rt)
	   }
	   s.attribOn.Store(on)
   }

// AttributionOn 钩子是否挡住没有入账意图的观察。

   func (s *Service) AttributionOn() bool {
	  return s != nil && s.attribOn.Load()
  }

// gateAttribution 钩子关闭或未装配时直接返回，不改写请求。
  func (s *Service) gateAttribution(ctx context.Context, req ObserveReq, seen wallet.Observation) error {
	  if s == nil || !s.attribOn.Load() {
		  return nil
	  }
	   if s.attribEng == nil {
		   return ErrAttribHold
	  }
	  return s.attribEng.Gate(ctx, req, seen)
  }
