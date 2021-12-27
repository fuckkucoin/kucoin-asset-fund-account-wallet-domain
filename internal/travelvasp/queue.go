package travelvasp

import (
	"sort"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/validate"
)

// QueueItem 运营案件队列条。

type QueueItem struct {
	Case
	AgeSec      int64 `json:"age_sec"`
	OutboxRetry int   `json:"outbox_retry"`
	NeedsAdapter bool `json:"needs_adapter"`
}

// QueuePage 运营队列分页。

type QueuePage struct {
	Items []QueueItem `json:"items"`
	Total int `json:"totalNum"`
	Page     int `json:"currentPage"`
	PageSize int `json:"pageSize"`
	Pages int `json:"totalPage"`
}

   func (s *Service) ListQueue(status string, page, size int) QueuePage {
	   status = strings.TrimSpace(status)
	   if status == "" {
		   status = string(CaseSubmitted)
	   }
	  list := s.ListCases("", status, 200)
	   sort.Slice(list, func(i, j int) bool {
		  return list[i].UpdatedAt.After(list[j].UpdatedAt)
	  })
	  now := time.Now().UTC()
	  items := make([]QueueItem, 0, len(list))
	   for _, c := range list {
		   peerProto := ""
		  if v, err := s.GetVASP(c.PeerVASP); err == nil {
			  peerProto = v.Protocol
		  }
		   need := peerProto != "" && peerProto != ProtoLOCAL && peerProto != ProtoUNHOSTED && c.Status == CaseSubmitted
		   items = append(items, QueueItem{
			   Case: c, AgeSec: int64(now.Sub(c.UpdatedAt) / time.Second),
			   OutboxRetry: s.outboxRetry(c.ID), NeedsAdapter: need,
		   })
	   }
	   page = validate.Page(page)
	   size = validate.Limit(size, 50, 200)
	  total := len(items)
	  pages := (total + size - 1) / size
	  if pages < 1 {
		   pages = 1
	   }
	   start := (page - 1) * size
	   if start >= total {
		   return QueuePage{Items: []QueueItem{}, Total: total, Page: page, PageSize: size, Pages: pages}
	   }
	   end := start + size
	   if end > total {
		   end = total
	  }
	   return QueuePage{Items: items[start:end], Total: total, Page: page, PageSize: size, Pages: pages}
  }
