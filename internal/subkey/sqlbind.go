// 子账户 API Key SQL 落盘。以 aa_sub_api_keys 为权威；secret 仅存信封密文。

// Author: Axel
// Date: 2021.11.28

package subkey

import (
	"context"
	"encoding/hex"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

type persistSQL = persist.SQLDB

   func (s *Store) BindSQL(db *persist.SQLDB) *Store {
	  if s != nil {
		   s.sql = db
		  s.loadSQL()
	   }
	   return s
  }

  func (s *Store) persistRow(r *row) error {
	  if s == nil || s.sql == nil || s.sql.DB == nil || r == nil {
		  return nil
	  }
	   st := r.Status
	   life := r.State
	   switch life {
	  case StateActive, StateRotating, StateRevoked:
	  default:
		  life = StateRevoked
		   st = StatusDisabled
	  }
	  if st == "" {
		  st = StatusDisabled
		   life = StateRevoked
	   }
	  return sqlrepo.NewFundingSubjects(s.sql).PutSubKey(context.Background(), sqlrepo.SubAPIKey{
		   APIKey: r.APIKey, Owner: r.Owner, SubName: r.SubName, Remark: r.Remark,
		   APIVersion: r.APIVersion,
		   Permission: r.Permission, IPWhitelist: r.IPWhitelist,
		  Status: st, PassHash: hex.EncodeToString(r.PassHash[:]), SecretEnc: r.SecretEnc,
		   ExpireAt: r.ExpireAt, LastUsedAt: r.LastUsedAt, LastUsedIP: r.LastUsedIP, RotatedAt: r.RotatedAt,
		  FailCount: r.FailCount, LockedUntil: r.LockedUntil, CallCount: r.CallCount, PassEnc: r.PassEnc,
		   DayCount: r.DayCount, DayStamp: r.DayStamp, DayCap: r.DayCap, RateMax: r.RateMax,
		   SecretFP: r.SecretFP, PrevSecretEnc: r.PrevSecretEnc, GraceUntil: r.GraceUntil,
		   CreatedIP: r.CreatedIP, PathAllow: r.PathAllow, IdleTTL: r.IdleTTL,
		   MethodAllow: r.MethodAllow,
		  NotBefore:   r.NotBefore, StickyIP: r.StickyIP,
		  SiteAllow: r.SiteAllow, SignVer: r.SignVer, Label: r.Label,
		  RotateCount: r.RotateCount, RotatedIP: r.RotatedIP,
		   LastFailAt: r.LastFailAt, LastFailIP: r.LastFailIP, DenyCount: r.DenyCount,
		   CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		  State: string(life), RevokedAt: r.RevokedAt,
	   })
  }

   func (s *Store) deleteRow(apiKey string) {
	   if s == nil || s.sql == nil || s.sql.DB == nil {
		   return
	  }
	   _ = sqlrepo.NewFundingSubjects(s.sql).DeleteSubKey(context.Background(), apiKey)
  }

// touchLastUsed 轻量刷新最近调用时间与 IP；失败忽略，内存侧已更新。

   func (s *Store) touchLastUsed(apiKey string, ts int64, clientIP string) {
	  if s == nil || s.sql == nil || s.sql.DB == nil {
		   return
	   }
	   _ = sqlrepo.NewFundingSubjects(s.sql).TouchSubKeyLastUsed(context.Background(), apiKey, ts, clientIP)
  }

  func (s *Store) persistAudit(ev AuditEvent) {
	  if s == nil || s.sql == nil || s.sql.DB == nil {
		  return
	   }
	  id, err := randHex(8)
	   if err != nil || id == "" {
		   return
	   }
	   _ = sqlrepo.NewFundingSubjects(s.sql).PutSubKeyAudit(context.Background(), sqlrepo.SubKeyAuditRow{
		   ID: "aud-" + id, Owner: ev.Owner, SubName: ev.SubName, APIKey: ev.APIKey,
		   Action: ev.Action, Detail: ev.Detail, ClientIP: ev.ActorIP, ActorIP: ev.ActorIP, CreatedAt: ev.At,
	   })
   }

  func (s *Store) loadAuditSQL(owner, subName, apiKey, action string, limit int, since, until int64) []AuditEvent {
	   if s == nil || s.sql == nil || s.sql.DB == nil {
		   return nil
	   }
	   rows, err := sqlrepo.NewFundingSubjects(s.sql).ListSubKeyAuditFiltered(context.Background(), owner, subName, apiKey, action, limit, since, until)
	   if err != nil || len(rows) == 0 {
		   return nil
	   }
	  out := make([]AuditEvent, 0, len(rows))
	   for _, r := range rows {
		  ip := r.ActorIP
		  if ip == "" {
			  ip = r.ClientIP
		   }
		   out = append(out, AuditEvent{Owner: r.Owner, SubName: r.SubName, APIKey: r.APIKey, Action: r.Action, Detail: r.Detail, ActorIP: ip, At: r.CreatedAt})
	   }
	  return out
   }

   func (s *Store) deleteAuditBefore(owner string, beforeMs int64) int {
	   if s == nil || s.sql == nil || s.sql.DB == nil {
		   return 0
	  }
	  n, err := sqlrepo.NewFundingSubjects(s.sql).SweepSubKeyAudit(context.Background(), owner, beforeMs)
	  if err != nil || n <= 0 {
		  return 0
	  }
	  return n
  }

  func (s *Store) loadSQL() {
	   s.loadSQLWithFallback()
   }

  func (s *Store) hydrateKeys(rows []sqlrepo.SubAPIKey) {
	   if s == nil || len(rows) == 0 {
		  return
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   for _, k := range rows {
		   sum,
			   ok := decodePassHash(k.PassHash)
		  if !ok {
			   continue
		   }
		  st := strings.ToLower(strings.TrimSpace(k.Status))
		  switch st {
		   case StatusEnabled, "enable", "on":
			  st = StatusEnabled
		  case StatusDisabled, "disable", "inactive", "off":
			   st = StatusDisabled
		   case StatusRevoked, "revoke":
			   st = StatusRevoked
		  default:
			   st = StatusDisabled
		  }
		  life := normalizeState(k.State)
		  if life == StateRevoked || strings.EqualFold(st, StatusRevoked) {
			  life = StateRevoked
			  st = StatusRevoked
		  }
		  bits, _ := ParsePermission(k.Permission)
		  s.by[k.APIKey] = &row{
			   Record: Record{
				   SubName: k.SubName,
				  Remark:  k.Remark, APIKey: k.APIKey, APIVersion: k.APIVersion,
				  Permission: k.Permission, IPWhitelist: k.IPWhitelist, Status: st,
				  ExpireAt:   k.ExpireAt,
				  LastUsedAt: k.LastUsedAt, LastUsedIP: k.LastUsedIP, RotatedAt: k.RotatedAt,
				   FailCount: k.FailCount, LockedUntil: k.LockedUntil, CallCount: k.CallCount,
				  DayCount: k.DayCount,
				   DayStamp: k.DayStamp, DayCap: k.DayCap, RateMax: k.RateMax,
				   SecretFP: k.SecretFP, GraceUntil: k.GraceUntil,
				  CreatedIP: k.CreatedIP, PathAllow: k.PathAllow, IdleTTL: k.IdleTTL,
				   MethodAllow: k.MethodAllow, NotBefore: k.NotBefore, StickyIP: k.StickyIP,
				  SiteAllow: k.SiteAllow, SignVer: k.SignVer, Label: k.Label,
				  RotateCount: k.RotateCount, RotatedIP: k.RotatedIP,
				  LastFailAt: k.LastFailAt, LastFailIP: k.LastFailIP, DenyCount: k.DenyCount,
				   CreatedAt: k.CreatedAt, UpdatedAt: k.UpdatedAt, UID: k.Owner, IsMaster: false,
				  State:     life,
				  RevokedAt: k.RevokedAt,
			  },
			   Owner: k.Owner, PassHash: sum, SecretEnc: k.SecretEnc, PassEnc: k.PassEnc, PrevSecretEnc: k.PrevSecretEnc,
			  PermBits: bits,
		  }
	  }
  }
