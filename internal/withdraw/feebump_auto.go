// 功能：出金提现域之自动费用抬升。
// 作者：Dax
// 日期：2020.10.29

package withdraw

import (
	"context"
	"encoding/hex"
	"fmt"
	"account-assets.kucoin.net/funding/internal/chainad"
)

// FeeBumpSigner 抬价规范串签章回调（通常挂 cryptoenc.SignCanonical）。
type FeeBumpSigner func(canonical string) ([]byte, error)

// FeeBumpAutoResult encode→custody 联签→Registry 广播一体回执。

type FeeBumpAutoResult struct {
	FeeBump FeeBump `json:"fee_bump"`
	Outbound chainad.Outbound `json:"outbound"`
	DestinationTag string           `json:"destination_tag"`
	Canonical      string           `json:"canonical"`
	SignatureHex   string           `json:"signature_hex,omitempty"`
	Receipt        *chainad.Receipt `json:"receipt,omitempty"`
	Path string `json:"path"`
}

// BindRegistry 装配 chainad.Registry，供 SubmitFeeBump / 自动广播。
// OpenMem/均可绑；未绑则自动广播失败关闭。
// 缺适配器时 Submit 返回 ErrUnavailable，不得假装已上链。

   func (s *Service) BindRegistry(reg *chainad.Registry) *Service {
	  if s != nil {
		   s.reg = reg
	   }
	   return s
   }

// Registry 已装配的链适配注册表。
   func (s *Service) Registry() *chainad.Registry {
	  if s == nil {
		  return nil
	  }
	   return s.reg
   }

// BindFeeBumpSigner 挂抬价联签回调（custody/HSM）。
   func (s *Service) BindFeeBumpSigner(fn FeeBumpSigner) *Service {
	  if s != nil {
		   s.feeBumpSign = fn
	  }
	   return s
  }

// EncodeSignBroadcastFeeBump 抬价 encode→custody 联签→Registry 自动广播。
// 先 BuildResubmitOutbound；autoSign 须已 BindFeeBumpSigner；autoBroadcast 须已 BindRegistry。
// 仅签不广播时不改 new_hash；广播失败保留已落库抬价行与签名面，须人工重提。

  func (s *Service) EncodeSignBroadcastFeeBump(ctx context.Context, trID, from, to, asset, amount string, hints BumpHints, autoSign, autoBroadcast bool) (FeeBumpAutoResult, error) {
	   out := FeeBumpAutoResult{Path: "fee-bump-encode"}
	   if s == nil {
		  return out, fmt.Errorf("withdraw service unavailable")
	   }
	  if autoSign {
		   if err := s.admitReserve(ctx, PhaseSign, asset, amount, trID); err != nil {
			  return out, err
		  }
	   }
	   if autoBroadcast {
		   if err := s.admitReserve(ctx, PhaseSubmit, asset, amount, trID); err != nil {
			  return out, err
		  }
	  }
	   outbound, row, err := s.Bumper().BuildResubmitOutbound(trID, from, to, asset, amount, hints)
	   if err != nil {
		  return out, err
	   }
	  dest := chainad.FeeBumpDestinationTag(outbound)
	  canon := chainad.CanonicalFeeBump(trID, outbound.Chain, outbound.To, outbound.Amount, row.NewFee, dest)
	  out.FeeBump = row
	  out.Outbound = outbound
	  out.DestinationTag = dest
	   out.Canonical = canon
	  if autoSign {
		   if s.feeBumpSign == nil {
			   return out, fmt.Errorf("fee bump signer unavailable")
		   }
		  sig, err := s.feeBumpSign(canon)
		   if err != nil {
			   return out, err
		  }
		  out.SignatureHex = hex.EncodeToString(sig)
		   out.Path = "fee-bump-encode-sign"
	  }
	  if autoBroadcast {
		   reg := s.reg
		   if reg == nil {
			  return out, chainad.ErrUnavailable
		  }
		   updated, rec, err := s.SubmitFeeBump(ctx, reg, trID, from, to, asset, amount, hints)
		   if err != nil {
			   return out, err
		  }
		  out.FeeBump = updated
		   cp := rec
		   out.Receipt = &cp
		   if autoSign {
			  out.Path = "fee-bump-encode-sign-broadcast"
		   } else {
			  out.Path = "fee-bump-encode-broadcast"
		   }
	   }
	   return out, nil
  }

// SubmitFeeBumpBound 用已装配 Registry 广播抬价。

  func (s *Service) SubmitFeeBumpBound(ctx context.Context, trID, from, to, asset, amount string, hints BumpHints) (FeeBump, chainad.Receipt, error) {
	   if s == nil || s.reg == nil {
		   return FeeBump{}, chainad.Receipt{}, chainad.ErrUnavailable
	   }
	  return s.SubmitFeeBump(ctx,
		   s.reg, trID, from, to, asset, amount, hints)
  }
