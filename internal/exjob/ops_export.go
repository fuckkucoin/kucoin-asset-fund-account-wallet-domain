package exjob

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// OpsExportReq 运营侧触发充提/闪兑/法币/理财导出。
// 复用窗口与日限额；kind=all|p2p 含资金邻接，不含现货/杠杆/P2P 盘口成交。
// 不得伪造对象存储 URL；正文仅在绑定账本后生成。

type OpsExportReq struct {
	UserID string `json:"user_id"`
	Kind    string `json:"kind"`
	Format  string `json:"format"`
	StartAt int64  `json:"start_at"`
	EndAt   int64  `json:"end_at"`
	Actor   string `json:"actor"`
}

// EnqueueOps 运营导出入队。

  func (b *Book) EnqueueOps(ctx context.Context, req OpsExportReq) (Job, error) {
	  _ = ctx
	   if b == nil {
		   return Job{}, fmt.Errorf("export job book unavailable")
	   }
	   uid := strings.TrimSpace(req.UserID)
	   if uid == "" {
		  return Job{}, fmt.Errorf("user_id required")
	  }
	  kind := strings.ToLower(strings.TrimSpace(req.Kind))
	   if kind == "" {
		   kind = "all"
	  }
	   format := strings.ToLower(strings.TrimSpace(req.Format))
	   if format == "" {
		   format = "csv"
	  }
	   var start, end time.Time
	  if req.StartAt > 0 {
		   start = time.UnixMilli(req.StartAt).UTC()
	  }
	  if req.EndAt > 0 {
		   end = time.UnixMilli(req.EndAt).UTC()
	  }
	  if start.IsZero() && end.IsZero() {
		  return b.Enqueue(uid, kind, format)
	   }
	   return b.EnqueueWindow(uid, kind, format, start, end)
   }
