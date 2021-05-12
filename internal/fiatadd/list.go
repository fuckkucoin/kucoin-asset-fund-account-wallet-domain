package fiatadd

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
)

// ListForUser 返回最近会话，按创建时间降序；limit 上限 50。

  func (b *Book) ListForUser(ctx context.Context, userID string, limit int) ([]Session, error) {
	   userID = strings.TrimSpace(userID)
	  if userID == "" {
		  return nil, ErrUser
	   }
	  if b == nil || b.kv == nil {
		  return nil, ErrStore
	   }
	   if limit <= 0 || limit > 50 {
		  limit = 20
	  }
	   rows, err := b.kv.List(ctx, kind, limit*8)
	   if err != nil {
		  return nil, err
	  }
	   out := make([]Session, 0, limit)
	   for _, row := range rows {
		   var sess Session
		  if json.Unmarshal(row.Body, &sess) != nil || sess.ID == "" {
			   continue
		   }
		   if sess.UserID != userID {
			   continue
		   }
		  out = append(out, sess)
		  if len(out) >= limit {
			  break
		   }
	  }
	  sort.Slice(out, func(i, j int) bool {
		  return out[i].CreatedAt > out[j].CreatedAt
	  })
	   return out, nil
   }
