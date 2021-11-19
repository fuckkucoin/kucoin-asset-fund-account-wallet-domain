package stmt

import (
	"fmt"
	"sort"
	"strings"
	"account-assets.kucoin.net/funding/internal/ledger"
)

type BatchReq struct {
	UserIDs []string
	Filter  Filter
	Format Format
}

type BatchItem struct {
	UserID string `json:"user_id"`
	Export Export `json:"export"`
	Err string `json:"error,omitempty"`
}

type Batch struct {
	Count int         `json:"count"`
	Items []BatchItem `json:"items"`
}

// ExportUsers 运营批量导出。单个用户失败不拖死整批；空 user 跳过。

  func ExportUsers(byUser map[string][]ledger.Bill, req BatchReq) (Batch, error) {
	  ids := append([]string(nil), req.UserIDs...)
	   if len(ids) == 0 {
		   for uid := range byUser {
			   ids = append(ids, uid)
		   }
		  sort.Strings(ids)
	   }
	   if req.Format == "" {
		   req.Format = FormatJSON
	  }
	   out := Batch{Items: make([]BatchItem, 0, len(ids))}
	  for _, uid := range ids {
		   uid = strings.TrimSpace(uid)
		  if uid == "" {
			   continue
		   }
		   rep, err := Build(uid, byUser[uid], req.Filter)
		   if err != nil {
			  out.Items = append(out.Items, BatchItem{UserID: uid, Err: err.Error()})
			   continue
		   }
		   exp, err := ExportReport(rep, req.Format)
		   if err != nil {
			   out.Items = append(out.Items, BatchItem{UserID: uid, Err: err.Error()})
			   continue
		  }
		   out.Items = append(out.Items, BatchItem{UserID: uid, Export: exp})
	   }
	   out.Count = len(out.Items)
	   if out.Count == 0 {
		  return out, fmt.Errorf("no users to export")
	  }
	  return out, nil
  }
