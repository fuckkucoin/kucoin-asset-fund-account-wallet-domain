// @FileName: edge_sla.go
// @Summary: 出金提现域之边缘 SLA。
// @Author: Talon
// @Created: 2021.01.13

package withdraw

import (
	"fmt"
	"strings"
	"time"
)

// OpenPastSLA 列出超 SLA 仍未结的出金异常单。
// 禁止 AutoClose；本列表供运营页/告警拉取。
// 空 desk 返回空；不得因超 SLA 自动解冻出金。

  func (d *Desk) OpenPastSLA(now time.Time, sla time.Duration) []Exception {
	   if d == nil {
		   return nil
	   }
	  if sla <= 0 {
		  sla = 4 * time.Hour
	  }
	   if now.IsZero() {
		   now = time.Now().UTC()
	   }
	   d.mu.Lock()
	  defer d.mu.Unlock()
	  out := make([]Exception, 0)
	   for _, row := range d.rows {
		   if row == nil || row.State == TicketClosed {
			  continue
		   }
		  if now.Sub(row.CreatedAt) >= sla {
			  cp := *row
			  out = append(out, cp)
		   }
	  }
	  return out
   }

// RequireAssignDistinct 结案人不得与开单人相同（四眼）。

  func RequireAssignDistinct(openedBy, actor string) error {
	   if strings.TrimSpace(actor) == "" {
		   return ErrTicketActor
	  }
	  if strings.EqualFold(strings.TrimSpace(openedBy), strings.TrimSpace(actor)) {
		   return fmt.Errorf("%w: opener", ErrTicketAssign)
	  }
	  return nil
  }
