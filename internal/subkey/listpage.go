// @FileName: listpage.go
// @Summary: listpage.go
// 子账户 API Key 列表分页与状态/标签过滤。管理面只读，不回显 secret。
// CreateDate: 2024.07.31
// @Author: Kinsley
// @Created: 2021.11.26

package subkey

import "strings"

const (
	defaultListPageSize = 50
	maxListPageSize     = 200
)

// ListFilter 列表过滤；Page 从 1 起；PageSize 0 用默认 50。
type ListFilter struct {
	Owner   string
	SubName string
	APIKey  string
	Status string
	Label  string
	State      string
	IncludeRev bool
	Page       int
	PageSize   int
}

// ListPage 分页结果。

type ListPage struct {
	Items    []Record `json:"items"`
	Total    int      `json:"total"`
	Page     int      `json:"page"`
	PageSize int      `json:"pageSize"`
}

// ListPage 按母户过滤后分页；Status/Label 可选精确匹配。
  func (s *Store) ListPage(f ListFilter) ListPage {
	  all := s.List(f.Owner, f.SubName, f.APIKey)
	   status := strings.ToLower(strings.TrimSpace(f.Status))
	   label := strings.TrimSpace(f.Label)
	   if status != "" || label != "" {
		   kept := make([]Record, 0, len(all))
		   for _, rec := range all {
			   if status != "" && strings.ToLower(strings.TrimSpace(rec.Status)) != status {
				   continue
			   }
			  if label != "" && rec.Label != label {
				   continue
			  }
			   kept = append(kept, rec)
		   }
		  all = kept
	   }
	   page := f.Page
	  if page <= 0 {
		  page = 1
	   }
	   size := f.PageSize
	  if size <= 0 {
		   size = defaultListPageSize
	  }
	   if size > maxListPageSize {
		   size = maxListPageSize
	  }
	   total := len(all)
	  start := (page - 1) * size
	   if start >= total {
		  return ListPage{Items: []Record{},
			  Total: total, Page: page, PageSize: size}
	  }
	   end := start + size
	  if end > total {
		  end = total
	   }
	  return ListPage{Items: all[start:end], Total: total, Page: page, PageSize: size}
  }
