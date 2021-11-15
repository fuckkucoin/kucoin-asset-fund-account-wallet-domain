package solvency

import "account-assets.kucoin.net/funding/internal/auditchain"

// ChainAudit 把偿付发布记到只追加审计链。参与者不使用 system，避免审计链拒绝入账。
type ChainAudit struct {
	Chain *auditchain.Service
}

   func (c ChainAudit) Append(kind, actor, body string) (string, error) {
	  if c.Chain == nil {
		   return "", ErrAudit
	  }
	  e, err := c.Chain.Append(kind, actor, body)
	  if err != nil {
		  return "", err
	  }
	  return e.ID, nil
   }
