package travelvasp

import (
	"sort"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/validate"
)

const defaultVASPCacheTTL = 60 * time.Second

// VASPPage 目录分页。

type VASPPage struct {
	Items []VASP `json:"items"`
	Total    int `json:"totalNum"`
	Page     int `json:"currentPage"`
	PageSize int `json:"pageSize"`
	Pages int  `json:"totalPage"`
	Exact bool `json:"exact,omitempty"`
}

// InvalidateVASPCache 显式作废目录缓存。

   func (s *Service) InvalidateVASPCache() {
	   if s == nil {
		   return
	   }
	  s.mu.Lock()
	  s.cacheList = nil
	   s.cacheAt = time.Time{}
	  s.mu.Unlock()
   }

   func (s *Service) BindCacheTTL(d time.Duration) *Service {
	   if s != nil {
		  if d <= 0 {
			  d = defaultVASPCacheTTL
		   }
		  s.cacheTTL = d
	  }
	   return s
   }

   func (s *Service) cachedVASPs() []VASP {
	   if s == nil {
		  return nil
	  }
	   ttl := s.cacheTTL
	  if ttl <= 0 {
		   ttl = defaultVASPCacheTTL
	   }
	  s.mu.Lock()
	   if len(s.cacheList) > 0 && !s.cacheAt.IsZero() && s.now().Sub(s.cacheAt) < ttl {
		   out := append([]VASP(nil),
			   s.cacheList...)
		   s.mu.Unlock()
		   return out
	   }
	  s.mu.Unlock()
	   list := s.ListVASP()
	  s.mu.Lock()
	  s.cacheList = append([]VASP(nil), list...)
	  s.cacheAt = s.now()
	   s.mu.Unlock()
	   return list
  }

// SearchVASP 目录检索：精确 LEI/DID/代码先，再模糊名称。

  func (s *Service) SearchVASP(q ResolveQuery, page, size int) VASPPage {
	   page = validate.Page(page)
	   size = validate.Limit(size, 50, 200)
	   empty := VASPPage{Items: []VASP{}, Page: page, PageSize: size, Pages: 1}
	   if s == nil {
		  return empty
	   }
	   if lei := strings.ToUpper(strings.TrimSpace(q.LEI)); lei != "" {
		   s.mu.Lock()
		   if code, ok := s.byLEI[lei]; ok {
			  got := s.vasps[code].Copy()
			  s.mu.Unlock()
			  return VASPPage{Items: []VASP{got}, Total: 1, Page: 1, PageSize: size, Pages: 1, Exact: true}
		   }
		  s.mu.Unlock()
	  }
	   if did := strings.TrimSpace(q.DID); did != "" {
		  s.mu.Lock()
		   if code, ok := s.byDID[did]; ok {
			   got := s.vasps[code].Copy()
			  s.mu.Unlock()
			  return VASPPage{Items: []VASP{got}, Total: 1, Page: 1, PageSize: size, Pages: 1, Exact: true}
		   }
		  s.mu.Unlock()
	  }
	   if code := strings.ToUpper(strings.TrimSpace(q.Code)); code != "" {
		   if got, err := s.GetVASP(code); err == nil {
			  return VASPPage{Items: []VASP{got}, Total: 1, Page: 1, PageSize: size, Pages: 1, Exact: true}
		   }
	  }
	   name := strings.ToLower(strings.TrimSpace(q.Name))
	  country := strings.ToUpper(strings.TrimSpace(q.Country))
	  list := s.cachedVASPs()
	   out := make([]VASP, 0, len(list))
	  for _, v := range list {
		  if country != "" && v.Country != country {
			  continue
		  }
		  if name != "" && !strings.Contains(strings.ToLower(v.Name), name) {
			   continue
		  }
		  out = append(out, v)
	   }
	   sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	  total := len(out)
	   pages := (total + size - 1) / size
	   if pages < 1 {
		  pages = 1
	   }
	  start := (page - 1) * size
	  if start >= total {
		   return VASPPage{Items: []VASP{}, Total: total, Page: page, PageSize: size, Pages: pages}
	   }
	  end := start + size
	  if end > total {
		   end = total
	  }
	   return VASPPage{Items: out[start:end], Total: total, Page: page, PageSize: size, Pages: pages}
  }
