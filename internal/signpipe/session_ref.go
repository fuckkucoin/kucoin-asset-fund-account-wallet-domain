package signpipe

import (
	"fmt"
	"strings"
	"time"
)

// SweepSignRef 冷腿绑定的签章任务。不含签名字节。

type SweepSignRef struct {
	ID    string `json:"id"`
	State string `json:"state"`
	Chain     string    `json:"chain"`
	Amount    string    `json:"amount"`
	ExpiresAt time.Time `json:"expires_at"`
}

  func (p *Pipe) SweepRef(id string) (SweepSignRef, error) {
	   if p == nil {
		   return SweepSignRef{}, fmt.Errorf("sign pipeline unavailable")
	   }
	  j, err := p.Get(strings.TrimSpace(id))
	  if err != nil {
		   return SweepSignRef{}, err
	  }
	  return SweepSignRef{ID: j.ID,
		   State: string(j.State), Chain: j.Chain, Amount: j.Amount, ExpiresAt: j.ExpiresAt}, nil
   }
