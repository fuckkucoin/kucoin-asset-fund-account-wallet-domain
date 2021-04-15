package destallow

import (
	"fmt"
	"strings"
)

// ImportRow 批量导入一行。
type ImportRow struct {
	UserID string `json:"user_id"`
	Chain   string `json:"chain"`
	Address string `json:"address"`
	Label string `json:"label,omitempty"`
}

// ImportFail 导入失败行。
type ImportFail struct {
	Row   ImportRow `json:"row"`
	Error string    `json:"error"`
}

// ImportReport 导入报告。
type ImportReport struct {
	Accepted []EntryView `json:"accepted"`
	Failed []ImportFail `json:"failed"`
	Total  int          `json:"total"`
}

// ImportExact 批量精确导入。单条校验失败入 Failed；存储故障立即返回。

  func (b *Book) ImportExact(rows []ImportRow, actor string) (ImportReport, error) {
	  rep := ImportReport{Accepted: []EntryView{}, Failed: []ImportFail{}, Total: len(rows)}
	   if b == nil {
		   return rep, ErrMissing
	   }
	   if len(rows) > 200 {
		   return rep, fmt.Errorf("destallow import exceeds 200")
	   }
	   for _, row := range rows {
		  e, err := b.AddExact(row.UserID, row.Chain, row.Address, row.Label)
		  if err != nil {
			   if err == ErrStore {
				   return rep, err
			   }
			   rep.Failed = append(rep.Failed, ImportFail{Row: row, Error: err.Error()})
			   continue
		  }
		   b.noteAudit(actor, "import", e.UserID, e.Chain, e.Address, e.Label, "bulk import")
		  rep.Accepted = append(rep.Accepted,
			  b.ViewOf(e))
	   }
	  return rep, nil
   }

// ExportFilter 导出过滤后的白名单视图。

  func (b *Book) ExportFilter(userID, chain, label string) []EntryView {
	   if b == nil {
		  return []EntryView{}
	   }
	  return b.DecorateEntries(b.ListFilter(userID, strings.TrimSpace(chain), strings.TrimSpace(label)))
   }
