// @FileName: chain.go
// @Summary: deposit 模块 chain。
// 充值入账域之链交互适配。
// @Author: Blaise
// @Created: 2021.01.07

package deposit

import (
	"account-assets.kucoin.net/funding/internal/chainad"
	"account-assets.kucoin.net/funding/internal/ledger"
)

// NewWithChain 用各族内存适配器当观察面。金额仍只认 Inspect 结果。

  func NewWithChain(led *ledger.Service, reg *chainad.Registry) *Service {
	  if reg == nil {
		   reg = chainad.NewRegistry()
	   }
	  return New(led, reg)
  }
