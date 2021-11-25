// 功能：subkey 模块 audit。
// 子账户 API Key 管理面审计。记录创建/改/启停/轮换/改口/清扫/解锁/删除，不写 secret/passphrase。

// 作者：TianpengLan
// 日期：2021.11.24

package subkey

import (
	"strings"
	"sync"
	"time"
)

var auditMu sync.Mutex

const maxAuditMem = 256

// AuditEvent 管理面动作；detail 仅非敏感摘要。
type AuditEvent struct {
	Owner string `json:"owner"`
	SubName string `json:"subName"`
	APIKey  string `json:"apiKey,omitempty"`
	Action  string `json:"action"`
	Detail string `json:"detail,omitempty"`
	ActorIP string `json:"actorIp,omitempty"`
	At      int64  `json:"at"`
}

// AuditFilter 审计查询；Limit<=0 用 200。
type AuditFilter struct {
	Owner string
	SubName string
	APIKey string
	Action string
	Limit  int
	Since int64
	Until int64
}

// noteAudit 成功变更后落审计；失败路径不记。独立 auditMu，避免与 s.mu 重入。

  func (s *Store) noteAudit(owner, subName, apiKey, action, detail string) {
	  s.noteAuditIP(owner, subName, apiKey, action, detail, "")
  }

   func (s *Store) noteAuditIP(owner, subName, apiKey, action, detail, actorIP string) {
	   if s == nil {
		  return
	   }
	   ev := AuditEvent{
		   Owner: strings.TrimSpace(owner),
		  SubName: strings.TrimSpace(subName),
		  APIKey:  strings.TrimSpace(apiKey),
		  Action: strings.TrimSpace(action),
		  Detail: strings.TrimSpace(detail),
		   ActorIP: strings.TrimSpace(actorIP),
		   At: time.Now().UTC().UnixMilli(),
	  }
	  if ev.Owner == "" || ev.Action == "" {
		   return
	  }
	   s.appendAuditPage(ev)
	  auditMu.Lock()
	  s.audits = append(s.audits, ev)
	   if len(s.audits) > maxAuditMem {
		   s.audits = append([]AuditEvent(nil), s.audits[len(s.audits)-maxAuditMem:]...)
	  }
	  auditMu.Unlock()
	   s.persistAudit(ev)
   }

// ListAudit 按母户过滤；可选 subName / apiKey。按时间降序。
   func (s *Store) ListAudit(owner, subName, apiKey string) []AuditEvent {
	  return s.ListAuditFilter(AuditFilter{Owner: owner, SubName: subName, APIKey: apiKey})
   }

// ListAuditFilter 按母户 + 可选 action 过滤；Limit 默认 200。
  func (s *Store) ListAuditFilter(f AuditFilter) []AuditEvent {
	   if s == nil {
		  return nil
	  }
	   f.Owner = strings.TrimSpace(f.Owner)
	   f.SubName = strings.TrimSpace(f.SubName)
	  f.APIKey = strings.TrimSpace(f.APIKey)
	   f.Action = strings.TrimSpace(f.Action)
	   if f.Owner == "" {
		  return nil
	   }
	  if f.Limit <= 0 || f.Limit > 200 {
		   f.Limit = 200
	   }
	   if rows := s.loadAuditSQL(f.Owner, f.SubName, f.APIKey, f.Action, f.Limit, f.Since, f.Until); len(rows) > 0 {
		  return rows
	   }
	   auditMu.Lock()
	  defer auditMu.Unlock()
	   out := make([]AuditEvent, 0)
	   for i := len(s.audits) - 1; i >= 0; i-- {
		  ev := s.audits[i]
		   if ev.Owner != f.Owner {
			  continue
		  }
		   if f.SubName != "" && ev.SubName != f.SubName {
			   continue
		  }
		  if f.APIKey != "" && ev.APIKey != f.APIKey {
			   continue
		  }
		  if f.Action != "" && ev.Action != f.Action {
			  continue
		  }
		   if f.Since > 0 && ev.At < f.Since {
			  continue
		   }
		  if f.Until > 0 && ev.At > f.Until {
			   continue
		   }
		  out = append(out, ev)
		   if len(out) >= f.Limit {
			  break
		  }
	   }
	  return out
  }
