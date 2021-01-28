// 功能：把归因引擎接到观察服务上。默认不打开钩子。
// 环境变量 ATTRIB_REQUIRE=1 时，本进程的 Observe 才要求入账意图。
// 速率键由引擎写成 attrib:，这里不碰 dep:。

// 作者：Likeo
// 日期：2021.01.07

package deposit

import (
	"context"
	"os"
	"strings"
	"account-assets.kucoin.net/funding/internal/addrintel"
	"account-assets.kucoin.net/funding/internal/addrpool"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/attribrt"
	"account-assets.kucoin.net/funding/internal/fundsurf"
	"account-assets.kucoin.net/funding/internal/wallet"
)

type attribBox struct {
	eng *attribrt.Engine
	svc *Service
}

  func (b *attribBox) Gate(ctx context.Context, req ObserveReq, seen wallet.Observation) error {
	  if b == nil || b.eng == nil {
		  return ErrAttribHold
	  }
	   _ = ctx
	   to := strings.TrimSpace(seen.ToAddress)
	  d, err := b.eng.Resolve(attribrt.Sight{
		  Chain: req.Chain, TxHash: req.TxHash, OutputIndex: req.OutputIndex,
		  Asset: req.Asset, Amount: seen.Amount.String(),
		  From: strings.TrimSpace(seen.FromAddress),
		  To:   to,
		   Memo: req.Memo, Tag: req.Tag, Network: req.Network,
		   UserHint: req.UserID, Confirmations: seen.Confirmations,
	   })
	  if err != nil {
		   return err
	   }
	  if !d.CreditIntent {
		   return &attribrt.DecisionError{Codes: d.Reasons}
	   }
	  if req.UserID != "" && d.UserID != "" && req.UserID != d.UserID {
		   return ErrAttribHold
	  }
	   return nil
  }

   func mountAttribution(s *Service, rt *app.Runtime) AttributionGate {
	  eng, err := attribrt.New()
	  if err != nil {
		   return brokenGate{err: err}
	  }
	   eng.SetBinder(liveBinder{svc: s})
	   if rt != nil {
		   if p, ok := rt.SharedMemo.(*addrpool.SharedMemoPool); ok {
			  eng.SetShared(func(chain string) (string, bool) {
				  return addrpool.SharedNamespace(p, chain)
			   })
		  }
		  if !rt.Mem && rt.SQL != nil {
			  eng.BindSQL(rt.SQL)
		   }
		  intel := addrintel.New()
		   eng.SetIntel(func(chain, addr string) (bool, bool) {
			  return addrintel.SourceFlags(intel, chain, addr)
		  })
		   if rt.FundSurf != nil {
			  rt.FundSurf.BindAttribReader(func(chain, hash string, output int) (fundsurf.AttribSnap, bool) {
				  d, ok := eng.Find(chain, hash, output)
				   if !ok {
					   return fundsurf.AttribSnap{}, false
				   }
				   return fundsurf.AttribSnap{
					  ID: d.ID, Chain: d.Chain, TxHash: d.TxHash, Output: d.OutputIndex,
					   Disposition: d.Disposition, Credit: d.CreditIntent,
					  Reasons: joinReason(d.Reasons), UserID: d.UserID,
					  Policy: d.PolicyVersion, Family: d.Family,
				  }, true
			  })
		   }
	  }
	   return &attribBox{eng: eng,
		  svc: s}
  }

  func joinReason(xs []string) string {
	  out := ""
	   for i, s := range xs {
		   if i > 0 {
			  out += ","
		   }
		  out += s
	   }
	  return out
  }

type brokenGate struct{ err error }

   func (g brokenGate) Gate(context.Context, ObserveReq, wallet.Observation) error {
	   if g.err != nil {
		   return g.err
	  }
	   return ErrAttribHold
  }

type liveBinder struct{ svc *Service }

   func (b liveBinder) Lookup(chain, kind, key string) (string, bool) {
	   if b.svc == nil || key == "" {
		  return "", false
	  }
	   got, ok := b.svc.Attributor().Lookup(chain, AttribKind(kind), key)
	  if !ok {
		   return "", false
	  }
	  return got.UserID, true
  }

// Engine 取出已装配的运行时。没有装配过就先装，但不打开钩子。

   func Engine(s *Service) *attribrt.Engine {
	  if s == nil {
		   return nil
	  }
	   g := s.prepareAttribution(nil)
	  box, ok := g.(*attribBox)
	   if !ok {
		   return nil
	   }
	   return box.eng
  }

// MaybeEnableAttribution 仅当 ATTRIB_REQUIRE=1。未设置时钩子保持关闭。

  func MaybeEnableAttribution(s *Service, rt *app.Runtime) {
	   if os.Getenv("ATTRIB_REQUIRE") != "1" {
		   return
	   }
	   EnableAttribution(s, rt)
   }
