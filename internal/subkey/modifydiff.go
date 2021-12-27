// 功能：子账户 API Key 修改摘要。审计只记变更字段名，不含 secret/passphrase/口令哈希。
// 作者：TianpengLan
// 日期：2021.11.25

package subkey

import "strings"

// modifyDiff 核对修改前后可见字段；无变更返回空串。

   func modifyDiff(before, after Record) string {
	  parts := make([]string, 0, 12)
	   if before.Permission != after.Permission {
		   parts = append(parts, "perm")
	  }
	  if before.IPWhitelist != after.IPWhitelist {
		   parts = append(parts, "ip")
	   }
	   if before.Remark != after.Remark {
		  parts = append(parts, "remark")
	   }
	  if before.ExpireAt != after.ExpireAt {
		   parts = append(parts, "expire")
	  }
	   if before.NotBefore != after.NotBefore {
		   parts = append(parts, "notBefore")
	  }
	  if before.RateMax != after.RateMax {
		  parts = append(parts, "rate")
	   }
	   if before.DayCap != after.DayCap {
		  parts = append(parts, "dayCap")
	  }
	  if before.PathAllow != after.PathAllow {
		   parts = append(parts, "path")
	  }
	  if before.MethodAllow != after.MethodAllow {
		   parts = append(parts, "method")
	   }
	  if before.IdleTTL != after.IdleTTL {
		  parts = append(parts, "idle")
	  }
	   if before.StickyIP != after.StickyIP {
		   parts = append(parts, "sticky")
	   }
	  if before.SiteAllow != after.SiteAllow {
		  parts = append(parts, "site")
	   }
	  if before.SignVer != after.SignVer {
		   parts = append(parts, "signVer")
	   }
	   if before.Label != after.Label {
		   parts = append(parts, "label")
	   }
	  return strings.Join(parts, ",")
   }
