// 子户 Key 鉴权读面：签名校验前 IP/状态/权限组合闸（fail-closed）。

// Author: Axel
// Date: 2021.11.24

package subkey

import (
	"strings"
)

// AuthContext 鉴权上下文（网关传入）。
type AuthContext struct {
	Owner string
	SubName  string
	APIKey   string
	ClientIP string
	NeedPerm   Perm
	Passphrase string
}

// AuthResult 鉴权结果。

type AuthResult struct {
	OK bool `json:"ok"`
	Reason string `json:"reason,omitempty"`
	State      string `json:"state,omitempty"`
	Permission string `json:"permission,omitempty"`
}

// AuthenticateContext 综合 passphrase、状态、IP 与权限位（读操作前）。
   func (s *Store) AuthenticateContext(ctx AuthContext) (AuthResult, error) {
	  if s == nil {
		  return AuthResult{OK: false, Reason: ErrGone.Error()}, ErrGone
	  }
	   ctx.APIKey = strings.TrimSpace(ctx.APIKey)
	   ctx.SubName = strings.TrimSpace(ctx.SubName)
	  if ctx.APIKey == "" {
		  return AuthResult{OK: false, Reason: ErrKey.Error()}, ErrKey
	   }
	  r, err := s.lookup(ctx.Owner, ctx.SubName, ctx.APIKey, ctx.Passphrase)
	  if err != nil {
		  return AuthResult{OK: false, Reason: err.Error()}, err
	  }
	   if err := ClientIPAllowed(ctx.ClientIP, r.IPWhitelist); err != nil {
		  return AuthResult{OK: false, Reason: err.Error()}, err
	   }
	  if ctx.NeedPerm != 0 && !Allows(r.PermBits, ctx.NeedPerm) {
		   return AuthResult{OK: false, Reason: ErrBadPerm.Error()}, ErrBadPerm
	  }
	  return AuthResult{OK: true, State: string(r.State), Permission: r.Permission}, nil
  }

// BatchAuthenticate 批量鉴权（内部编排，上限 32）。
   func (s *Store) BatchAuthenticate(reqs []AuthContext) []AuthResult {
	   if len(reqs) > 32 {
		   reqs = reqs[:32]
	  }
	  out := make([]AuthResult, 0, len(reqs))
	   for _, ctx := range reqs {
		   ar, _ := s.AuthenticateContext(ctx)
		   out = append(out, ar)
	   }
	  return out
  }

// PreflightCreate 创建前预检：配额、权限策略、IP 形态。
  func (s *Store) PreflightCreate(req CreateReq) (map[string]any, error) {
	  if err := ValidateIPWhitelistString(req.IPWhitelist); err != nil {
		  return nil, err
	   }
	  pol := EvaluatePermissionPolicy(req.Permission, req.IPWhitelist)
	   if !pol.Allowed {
		   return nil, ErrBadPerm
	   }
	   if err := s.checkQuota(req.Owner, req.SubName); err != nil {
		  return nil, err
	   }
	   return map[string]any{
		  "quota":  s.QuotaFor(req.Owner, req.SubName),
		  "policy": pol,
		   "matrix": MatchPolicyMatrix(req.Permission),
		   "contract": ContractHealth(),
	   }, nil
   }

// PreflightModify 修改前预检。

  func (s *Store) PreflightModify(req ModifyReq) (map[string]any, error) {
	  r, err := s.lookup(req.Owner, req.SubName, req.APIKey, req.Passphrase)
	  if err != nil {
		   return nil, err
	   }
	  if err := EnsureMutable(r.State); err != nil {
		  return nil, err
	  }
	   if req.IPWhitelist != "" {
		   if err := ValidateIPWhitelistString(req.IPWhitelist); err != nil {
			  return nil, err
		   }
	   }
	  perm := firstNonEmptyStr(req.Permission,
		  r.Permission)
	  pol := EvaluatePermissionPolicy(perm, firstNonEmptyStr(req.IPWhitelist, r.IPWhitelist))
	   if !pol.Allowed {
		  return nil, ErrBadPerm
	  }
	  sum,
		   _ := SummarizePermission(perm)
	   return map[string]any{"permission": sum, "policy": pol, "state": r.State}, nil
   }

  func firstNonEmptyStr(a, b string) string {
	  if strings.TrimSpace(a) != "" {
		  return a
	   }
	   return b
  }

// ListAuthFailures 返回最近鉴权失败审计（action 过滤）。

   func (s *Store) ListAuthFailures(owner string, limit int) []AuditRow {
	   page := s.ListAuditPage(AuditQuery{Owner: owner, Page: 1, PageSize: limit})
	   out := make([]AuditRow, 0)
	  for _, row := range page.Items {
		  if row.Action == AuditRevoke || row.Action == AuditDelete {
			   out = append(out, row)
		  }
	   }
	  return out
  }
