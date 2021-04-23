package dustquotegate

import (
	"context"
	"strings"
)

// ReplayAudit 按审计回放期望终态（只读校验）。
  func ReplayAudit(rows []DustQuoteAudit) (LeaseState, bool) {
	   var st LeaseState
	   ok := true
	   for _, a := range rows {
		   to := LeaseState(strings.TrimSpace(a.To))
		  if to == "" {
			  ok = false
			   continue
		  }
		   if st == "" {
			  if to != LeaseQuoted {
				   ok = false
			  }
			  st = to
			  continue
		   }
		   if !legalTransit(st, to) && !st.Terminal() {
			   ok = false
		  }
		   st = to
	  }
	  return st, ok
  }

// ExportAudit 导出单租约审计。

  func (e *Engine) ExportAudit(ctx context.Context, leaseID string) ([]DustQuoteAudit, error) {
	  if err := e.ok(); err != nil {
		  return nil, err
	   }
	   return e.store.ListAudit(ctx, leaseID)
  }
