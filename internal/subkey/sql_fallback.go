// 子户 Key SQL 加载。列缺失或查询失败时不灌入内存，避免把未知行当成启用。

// Author: Cason
// Date: 2021.11.28

package subkey

import (
	"context"
	"encoding/hex"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

// loadSQLWithFallback 主加载入口。ListSubKeys 失败则不灌入 Key。

  func (s *Store) loadSQLWithFallback() {
	   if s == nil || s.sql == nil || s.sql.DB == nil {
		  return
	   }
	  repo := sqlrepo.NewFundingSubjects(s.sql)
	   rows, err := repo.ListSubKeys(context.Background())
	   if err != nil {
		  return
	  }
	   s.hydrateKeys(rows)
	   aud, err := repo.ListSubKeyAudit(context.Background(), "", 500)
	   if err == nil {
		  for _, a := range aud {
			   s.AppendAuditRow(AuditRow{
				  ID: a.ID, Owner: a.Owner, SubName: a.SubName, APIKey: a.APIKey,
				  Action: AuditAction(a.Action), Detail: a.Detail, ClientIP: a.ClientIP, CreatedAt: a.CreatedAt,
			   })
		   }
	   }
	  quotas,
		  err := repo.ListSubKeyQuota(context.Background())
	   if err != nil {
		   if s.quota == nil {
			   s.quota = newQuotaGate()
		   }
		  s.quota.markCorrupt()
		   return
	   }
	  for _, q := range quotas {
		  s.ApplyQuotaRow(q.Owner, q.SubName, q.MaxKeys)
	  }
   }

// decodePassHash 解析 pass_hash 列。

  func decodePassHash(raw string) ([32]byte, bool) {
	  var sum [32]byte
	  b, err := hex.DecodeString(raw)
	   if err != nil || len(b) != 32 {
		   return sum, false
	   }
	  copy(sum[:], b)
	   return sum, true
  }
