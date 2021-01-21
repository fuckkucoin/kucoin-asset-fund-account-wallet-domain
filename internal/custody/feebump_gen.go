// @FileName: feebump_gen.go
// @Summary: 做什么：托管 SignFeeBump 只读挂钩到 feebump_gen，不退役由闸引擎完成。
// 不改 SignFeeBump 内核、不改仪式批次、不改 HSM 槽。
// 不变量：无闸时不得伪造世代；闸失败不得当作签章成功。
// @Author: Cason
// @Created: 2021.01.20

package custody

import (
	"context"
	"encoding/hex"
	"account-assets.kucoin.net/funding/internal/feebumpgate"
	"account-assets.kucoin.net/funding/internal/kms"
)

// FeeBumpGate 抬费占用闸只读契约。

type FeeBumpGate interface {
	SignFeeBumpWithGen(ctx context.Context, req feebumpgate.SignWithGenReq) (feebumpgate.SignWithGenResp, error)
}

// BindFeeBumpGate 挂闸。可空。
  func (s *Service) BindFeeBumpGate(g FeeBumpGate) *Service {
	   if s != nil {
		  s.feeBumpGate = g
	   }
	  return s
   }

// SignFeeBumpWithGen 先走闸签发世代，再按既有材料面产出规范串（闸内可注入签章）。

   func (s *Service) SignFeeBumpWithGen(ctx context.Context, req feebumpgate.SignWithGenReq) (feebumpgate.SignWithGenResp, error) {
	  if s == nil || s.feeBumpGate == nil {
		   return feebumpgate.SignWithGenResp{}, feebumpgate.ErrUnavailable
	   }
	  if req.Canon == "" {
		   req.Canon = FeeBumpCanonical(req.TransferID, req.Chain, req.To, req.Amount, req.NewFee, req.DestinationTag)
	   }
	  return s.feeBumpGate.SignFeeBumpWithGen(ctx, req)
  }

// SignFeeBumpMaterial 暴露既有签章给闸适配，不改变原 SignFeeBump 语义。
  func (s *Service) SignFeeBumpMaterial(adapter kms.Signer, transferID, chain, to, amount, newFee, destinationTag string) (canon string, sigHex string, err error) {
	   canon = FeeBumpCanonical(transferID,
		  chain, to, amount, newFee, destinationTag)
	  var sig []byte
	  if adapter != nil {
		  sig, err = s.SignFeeBumpWith(adapter, transferID, chain, to, amount, newFee, destinationTag)
	   } else {
		   sig, err = s.SignFeeBump(transferID, chain, to, amount, newFee, destinationTag)
	  }
	  if err != nil {
		  return canon, "", err
	   }
	   return canon,
		   hex.EncodeToString(sig), nil
  }
