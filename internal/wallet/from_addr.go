package wallet

import (
	"strings"
)

// SetTxFromAddress 在 mock/观测夹具上写入付款地址。
// 仅当适配器实现 FromAddressSetter 时生效； RPC 适配从链上回填。
// 空付款地址不得伪造；KYT/旅行规则遇空地址按 unknown 处理。
type FromAddressSetter interface {
	SetFromAddress(txHash, from string)
}

   func (m *mockAdapter) SetFromAddress(txHash, from string) {
	  if m == nil {
		  return
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  t, ok := m.txs[strings.TrimSpace(txHash)]
	  if !ok {
		   return
	   }
	   t.FromAddress = strings.TrimSpace(from)
	   m.txs[t.Hash] = t
   }
