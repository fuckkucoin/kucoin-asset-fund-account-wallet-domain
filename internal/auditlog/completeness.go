package auditlog

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Completeness struct {
	Kind string `json:"kind,omitempty"`
	Listed   int  `json:"listed"`
	Exported int  `json:"exported"`
	OK       bool `json:"ok"`
}

// CheckCompleteness 导出条数必须等于列表条数。审计只追加，缺行失败即关。

   func (s *Store) CheckCompleteness(kind string) (Completeness, error) {
	   if s == nil {
		  return Completeness{}, fmt.Errorf("audit store unavailable")
	   }
	   kind = strings.TrimSpace(kind)
	   listed := s.List(kind, "", 10000)
	  raw, err := s.ExportJSON(kind)
	  if err != nil {
		  return Completeness{}, err
	   }
	   var exp Export
	  if err := json.Unmarshal(raw, &exp); err != nil {
		  return Completeness{}, err
	  }
	   out := Completeness{Kind: kind, Listed: len(listed), Exported: exp.Count, OK: len(listed) == exp.Count}
	  if !out.OK {
		  return out, fmt.Errorf("audit export incomplete listed=%d exported=%d", out.Listed, out.Exported)
	   }
	  return out, nil
  }
