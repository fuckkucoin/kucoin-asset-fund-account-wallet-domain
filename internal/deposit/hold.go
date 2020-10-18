// 功能：充值入账域之冻结占用。
// 作者：Kian
// 日期：2020.10.17

package deposit

import (
	"fmt"
	"strings"
	"time"
)

var (
	ErrCreditHold      = fmt.Errorf("deposit credit held pending exception")
	ErrCreditHoldChain = fmt.Errorf("deposit credit held for chain exception")
)

// BlocksCredit 未结异常单时不得入账。观察额与单据不一致必须先结案。

   func (d *Desk) BlocksCredit(userID string) error {
	   if d == nil {
		   return ErrTicketMiss
	  }
	  if d.OpenCount(userID) > 0 {
		  return ErrCreditHold
	   }
	  return nil
   }

// RequireClear 入账前闸。空用户按全台未结单计。
  func (d *Desk) RequireClear(userID string) error {
	   return d.BlocksCredit(userID)
  }

// BlocksCreditScoped 按链/资产收窄入金闸；空 Asset 旧单按链收窄。
// 带 Asset 的他链/他币开单仍全局挡；空 Asset 旧单仅挡同链，不再纯用户全局。
// 空 chain 请求遇空 Asset 单仍挡（无法判链）；结案前不得入账。
  func (d *Desk) BlocksCreditScoped(userID, chain, asset string) error {
	  if d == nil {
		   return ErrTicketMiss
	   }
	   rows := d.OpenRows(userID)
	   if len(rows) == 0 {
		  return nil
	   }
	   chain = strings.ToUpper(strings.TrimSpace(chain))
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	  blocking := false
	   chainScoped := false
	  for _, r := range rows {
		   rAsset := strings.ToUpper(strings.TrimSpace(r.Asset))
		  rChain := strings.ToUpper(strings.TrimSpace(r.Chain))
		   if rAsset == "" {
			   // 空 Asset 旧单：仅按链收窄，他链不挡。
			  if chain == "" || strings.EqualFold(rChain, chain) {
				   blocking = true
				  if chain != "" && strings.EqualFold(rChain, chain) {
					  chainScoped = true
				  }
			  }
			  continue
		  }
		  // 带 Asset：任一未结单仍挡本路径（防错网错币绕过）。
		   blocking = true
		  chainHit := chain != "" && strings.EqualFold(rChain, chain)
		   assetHit := asset != "" && strings.EqualFold(rAsset, asset)
		  if chainHit || assetHit {
			   chainScoped = true
		  }
	   }
	   if !blocking {
		  return nil
	  }
	   if chainScoped {
		   return ErrCreditHoldChain
	   }
	   return ErrCreditHold
  }

// OpenRows 列出用户未结异常单（入金闸观测）。

  func (d *Desk) OpenRows(userID string) []Exception {
	  if d == nil {
		   return nil
	  }
	  d.mu.Lock()
	  defer d.mu.Unlock()
	  userID = strings.TrimSpace(userID)
	  out := make([]Exception, 0)
	  for _, row := range d.rows {
		   if row.State == TicketClosed {
			  continue
		  }
		   if userID != "" && !strings.EqualFold(row.UserID, userID) {
			  continue
		   }
		  out = append(out, row.copy())
	   }
	   return out
   }

// OldestOpenAge 最老未结单年龄，供 SLA/告警。

  func (d *Desk) OldestOpenAge(userID string, now time.Time) (time.Duration, bool) {
	  rows := d.OpenRows(userID)
	  if len(rows) == 0 {
		   return 0, false
	  }
	   if now.IsZero() {
		  now = time.Now().UTC()
	   }
	   var oldest time.Time
	   for _, r := range rows {
		   if oldest.IsZero() || r.CreatedAt.Before(oldest) {
			   oldest = r.CreatedAt
		   }
	   }
	   if oldest.IsZero() {
		  return 0, false
	  }
	   return now.Sub(oldest), true
   }
