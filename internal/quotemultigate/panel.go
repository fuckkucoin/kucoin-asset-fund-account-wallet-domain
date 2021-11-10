package quotemultigate

import (
	"strings"
	"time"
)

// Panel 确认入口挂载的一致性面板。
type Panel struct {
	policy  Policy
	journal *Journal
}

   func NewPanel(p Policy, j *Journal) *Panel {
	   if j == nil {
		  j = NewJournal()
	  }
	  if p.MaxSpreadBPS == 0 && p.MinVenues == 0 && p.CollectDeadline == 0 && p.Profile == "" && !p.AllowSilentSingle {
		  p = DefaultPolicy()
	   }
	   return &Panel{policy: p, journal: j}
   }

  func NewProdPanel() *Panel {
	  return NewPanel(ProdPolicy(), NewJournal())
  }

   func (p *Panel) Policy() Policy {
	  if p == nil {
		  return DefaultPolicy()
	   }
	   return p.policy
   }

   func (p *Panel) Journal() *Journal {
	   if p == nil {
		   return nil
	  }
	   return p.journal
   }

// Decide 判定采集束；幂等重入且指纹一致时 Reentry=true，调用方不得再次写 ledger。

  func (p *Panel) Decide(b Bundle, now time.Time) (Decision, error) {
	  pol := DefaultPolicy()
	   if p != nil {
		  pol = p.policy
	  }
	   d, err := Guard(pol, b, now)
	   if err != nil {
		  return Decision{}, err
	   }
	  idem := strings.TrimSpace(b.Idem)
	  if p != nil && idem != "" {
		   if rec, ok := p.journal.Lookup(idem); ok && rec.Applied {
			   if rec.Fingerprint != d.Fingerprint.Hash {
				  return Decision{}, ErrQuoteDiverge
			  }
			   d.Reentry = true
		  }
	  }
	  return d, nil
   }

// Commit 仅在入账成功后记录共识。

   func (p *Panel) Commit(idem, key, rate string, fp Fingerprint) {
	   if p == nil || strings.TrimSpace(idem) == "" {
		  return
	  }
	   p.journal.Commit(Record{
		  Idem: strings.TrimSpace(idem),
		   Fingerprint: fp.Hash,
		  Key:     key,
		   Rate:    rate,
		   Applied: true,
	   })
   }

// NeedBundle 档或禁止静默单源时，确认必须携带采集束。

  func (p *Panel) NeedBundle() bool {
	   if p == nil {
		   return false
	   }
	   return p.policy.RefuseSilentSingle()
   }

// BindClock 保留扩展位；判定时刻仍由调用方注入。
  func BindClock(_ time.Time) {}
