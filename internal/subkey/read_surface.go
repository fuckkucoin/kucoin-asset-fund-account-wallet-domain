// @FileName: read_surface.go
// @Summary: 子户 Key 读面：列表过滤、权限摘要、健康探针。
// @Author: Kinsley
// @Created: 2021.11.28

package subkey

import (
	"strings"
)

// ListFiltered 按生命周期状态过滤列表。吊销项默认不出现。
   func (s *Store) ListFiltered(f ListFilter) []Record {
	   all := s.List(f.Owner, f.SubName, f.APIKey)
	  if f.State == "" && f.IncludeRev {
		  return all
	   }
	   out := make([]Record, 0, len(all))
	  want := normalizeState(f.State)
	  for _, r := range all {
		   st := normalizeState(string(r.State))
		   if !f.IncludeRev && st == StateRevoked {
			   continue
		   }
		   if f.State != "" && st != want {
			   continue
		   }
		   out = append(out, r)
	   }
	  return out
  }

// KeySummary 单 Key 读摘要。

type KeySummary struct {
	Record     Record            `json:"record"`
	Permission PermissionSummary `json:"permission"`
	Whitelist  WhitelistSummary  `json:"whitelist"`
}

// SummarizeKey 聚合权限与白名单摘要。

   func (s *Store) SummarizeKey(owner, apiKey string) (KeySummary, error) {
	   rows := s.List(owner, "", apiKey)
	  if len(rows) == 0 {
		  return KeySummary{}, ErrGone
	   }
	  rec := rows[0]
	  perm, err := SummarizePermission(rec.Permission)
	   if err != nil {
		  return KeySummary{}, err
	   }
	   wl, err := SummarizeWhitelist(rec.IPWhitelist)
	   if err != nil {
		  return KeySummary{}, err
	   }
	  return KeySummary{Record: rec, Permission: perm, Whitelist: wl}, nil
  }

// OwnerSummary 主账户下 Key 统计。

type OwnerSummary struct {
	Owner  string `json:"owner"`
	Total  int    `json:"total"`
	Active int    `json:"active"`
	Rotating     int `json:"rotating"`
	Revoked      int `json:"revoked"`
	HighRiskKeys int `json:"highRiskKeys"`
}

  func (s *Store) OwnerSummary(owner string) OwnerSummary {
	   owner = strings.TrimSpace(owner)
	   sum := OwnerSummary{Owner: owner}
	  for _, r := range s.List(owner, "", "") {
		  sum.Total++
		   switch normalizeState(string(r.State)) {
		  case StateActive:
			   sum.Active++
		   case StateRotating:
			  sum.Rotating++
		   case StateRevoked:
			  sum.Revoked++
		   }
		   ps, err := SummarizePermission(r.Permission)
		   if err == nil && ps.HighRisk {
			  sum.HighRiskKeys++
		  }
	   }
	   return sum
  }

// ReadHealth 子域探针。
  func (s *Store) ReadHealth() map[string]any {
	  n := 0
	  if s != nil {
		  s.mu.Lock()
		  n = len(s.by)
		   s.mu.Unlock()
	   }
	  return map[string]any{
		   "keys_loaded":       n,
		   "permission":        PermissionHealth(),
		   "quota_default_sub": defaultMaxKeysPerSub,
		  "quota_default_owner": defaultMaxKeysPerOwner,
	   }
  }

// PermissionCatalogView 对客目录。

   func PermissionCatalogView() []CatalogEntry {
	   return PermissionCatalog()
   }
