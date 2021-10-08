package quotemultigate

import "strings"

// OrPanel 空面板回落到默认策略，供显式采集束判定。

  func OrPanel(p *Panel) *Panel {
	   if p == nil {
		   return NewPanel(DefaultPolicy(), NewJournal())
	  }
	   return p
   }

// Coalesce 取首个非空白串，用于幂等键回填。
  func Coalesce(a, b string) string {
	  if strings.TrimSpace(a) != "" {
		   return a
	  }
	   return b
  }
