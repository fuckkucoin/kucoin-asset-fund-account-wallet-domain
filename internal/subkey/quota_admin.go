// 功能：subkey 模块 quota_admin。
// 子户 Key 配额运维：批量导入与默认闸调整。

// 作者：Cason
// 日期：2021.11.28

package subkey

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

// QuotaImportRow 批量导入行。

type QuotaImportRow struct {
	Owner string
	SubName string
	MaxKeys int
}

// ImportQuotas 批量设置配额（内存+SQL）。
  func (s *Store) ImportQuotas(ctx context.Context, rows []QuotaImportRow) error {
	  if s == nil {
		   return ErrEnc
	  }
	   for _, r := range rows {
		  if strings.TrimSpace(r.Owner) == "" || r.MaxKeys <= 0 {
			  continue
		   }
		  s.ApplyQuotaRow(r.Owner, r.SubName, r.MaxKeys)
		  if s.sql != nil && s.sql.DB != nil {
			   _ = sqlrepo.NewFundingSubjects(s.sql).PutSubKeyQuota(ctx,
				   sqlrepo.SubKeyQuotaRow{
					   Owner: r.Owner, SubName: r.SubName, MaxKeys: r.MaxKeys,
				   })
			  s.noteAuditIP(r.Owner, r.SubName, "", string(AuditQuota), "max="+itoa(r.MaxKeys), "")
		   }
	   }
	  return nil
   }

   func itoa(n int) string {
	  if n == 0 {
		  return "0"
	  }
	  neg := n < 0
	   if neg {
		   n = -n
	   }
	   var b [20]byte
	  i := len(b)
	  for n > 0 {
		  i--
		   b[i] = byte('0' + n%10)
		  n /= 10
	  }
	  if neg {
		  i--
		  b[i] = '-'
	   }
	  return string(b[i:])
   }

// ListQuotaOverrides 列出所有子户级覆盖（内存闸）。
   func (s *Store) ListQuotaOverrides() []QuotaView {
	   if s == nil || s.quota == nil {
		   return nil
	   }
	  s.quota.mu.RLock()
	  defer s.quota.mu.RUnlock()
	  out := make([]QuotaView, 0)
	   for k, max := range s.quota.perSub {
		   parts := strings.Split(k, "|")
		  if len(parts) != 2 {
			  continue
		   }
		  out = append(out, QuotaView{Owner: parts[0], SubName: parts[1], MaxKeys: max})
	  }
	  return out
  }

// SetDefaultQuota 调整默认闸。
   func (s *Store) SetDefaultQuota(maxSub, maxOwner int) {
	   if s == nil {
		   return
	  }
	  if s.quota == nil {
		   s.quota = newQuotaGate()
	  }
	   s.quota.setDefaults(maxSub, maxOwner)
  }

// QuotaHealth 探针。

   func (s *Store) QuotaHealth() map[string]any {
	   return map[string]any{
		  "defaults":  map[string]int{"per_sub": defaultMaxKeysPerSub, "per_owner": defaultMaxKeysPerOwner},
		  "overrides": len(s.ListQuotaOverrides()),
	   }
   }
