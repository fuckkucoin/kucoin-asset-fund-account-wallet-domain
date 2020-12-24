package custodyhalt

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

const KindAudit = "aa_wd_halt_audit"

// AuditStore 把熔断事件追加写入 KV；与进程内 History 并联。
type AuditStore struct {
	kv persist.KV
}

  func NewAuditStore(kv persist.KV) *AuditStore {
	   return &AuditStore{kv: kv}
   }

// Record 追加一条审计。存储不可用时静默返回，避免拖垮 Engage。

   func (a *AuditStore) Record(ctx context.Context, e Event) {
	   if a == nil || a.kv == nil {
		  return
	   }
	  if e.At.IsZero() {
		   e.At = time.Now().UTC()
	  }
	   key := fmt.Sprintf("%d:%s", e.At.UnixNano(), strings.TrimSpace(e.Action))
	   body, _ := json.Marshal(e)
	  _ = a.kv.Put(ctx, persist.Row{Kind: KindAudit, Key: key, Body: body}, persist.Fence{})
  }

// Load 按时间键升序取最近 limit 条。
  func (a *AuditStore) Load(ctx context.Context, limit int) []Event {
	   if a == nil || a.kv == nil {
		  return nil
	   }
	   if limit <= 0 || limit > 200 {
		   limit = 50
	  }
	  rows, err := a.kv.List(ctx, KindAudit, limit*2)
	   if err != nil || len(rows) == 0 {
		  return nil
	   }
	  out := make([]Event, 0, len(rows))
	   for _, r := range rows {
		   var e Event
		  if json.Unmarshal(r.Body, &e) != nil {
			  continue
		  }
		  out = append(out, e)
	   }
	  if len(out) > limit {
		  out = out[len(out)-limit:]
	   }
	  return out
   }

// AuditQuery 审计过滤条件；空字段表示不过滤。

type AuditQuery struct {
	Action string
	Actor string
	Scope string
	Chain string
	Asset string
	Limit int
}

// Query 按动作/操作者/作用域过滤审计；供运营复盘。
// 跨进程检索 Engage/Clear 轨迹。limit 上限 200；无匹配返回空切片而非错误。KV List 非严格时间序时结果可能夹杂更早记录，值班以 At 字段为准。

  func (a *AuditStore) Query(ctx context.Context, q AuditQuery) []Event {
	   limit := q.Limit
	   if limit <= 0 || limit > 200 {
		   limit = 50
	   }
	  all := a.Load(ctx, limit*2)
	   if len(all) == 0 {
		   return nil
	   }
	   action := strings.TrimSpace(strings.ToLower(q.Action))
	   actor := strings.TrimSpace(q.Actor)
	   scope := strings.TrimSpace(strings.ToLower(q.Scope))
	   chain := strings.ToUpper(strings.TrimSpace(q.Chain))
	  asset := strings.ToUpper(strings.TrimSpace(q.Asset))
	  out := make([]Event, 0, len(all))
	  for _, e := range all {
		   if action != "" && strings.ToLower(strings.TrimSpace(e.Action)) != action {
			  continue
		   }
		   if actor != "" && e.Actor != actor {
			  continue
		   }
		   if scope != "" && strings.ToLower(strings.TrimSpace(e.Scope)) != scope {
			  continue
		   }
		  if chain != "" && strings.ToUpper(strings.TrimSpace(e.Chain)) != chain {
			  continue
		  }
		   if asset != "" && strings.ToUpper(strings.TrimSpace(e.Asset)) != asset {
			   continue
		   }
		  out = append(out, e)
	  }
	   if len(out) > limit {
		  out = out[len(out)-limit:]
	  }
	   return out
  }
