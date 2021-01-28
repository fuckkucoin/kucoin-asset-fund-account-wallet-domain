package address

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

const maxBookImport = 200

// ImportReq 导入入参。Commit=false 为干跑。

type ImportReq struct {
	UserID string      `json:"user_id"`
	Rows   []BookWrite `json:"rows"`
	Commit bool        `json:"commit"`
}

// ImportReport 干跑或提交报告。
type ImportReport struct {
	DryRun   bool       `json:"dry_run"`
	Accepted []BookItem `json:"accepted"`
	Failed   []BatchFail `json:"failed"`
	Total    int         `json:"total"`
	WouldAdd int         `json:"would_add,omitempty"`
}

// ImportBook 地址簿导入。commit=false 只校验；true 才落库。
  func (s *Service) ImportBook(ctx context.Context, req ImportReq) (ImportReport, error) {
	   rep := ImportReport{DryRun: !req.Commit, Accepted: []BookItem{}, Failed: []BatchFail{}, Total: len(req.Rows)}
	   if s == nil || s.led == nil {
		   return rep, fmt.Errorf("address ledger unavailable")
	  }
	   if len(req.Rows) > maxBookImport {
		  return rep, fmt.Errorf("address book import exceeds %d", maxBookImport)
	  }
	   uid := strings.TrimSpace(req.UserID)
	   if !req.Commit {
		   return s.importDry(ctx,
			  uid, req.Rows, rep)
	   }
	   return s.importCommit(ctx, uid, req.Rows, rep)
  }

  func (s *Service) importDry(ctx context.Context, uid string, rows []BookWrite, rep ImportReport) (ImportReport, error) {
	  need := 0
	   for i, row := range rows {
		  if strings.TrimSpace(row.UserID) == "" {
			  row.UserID = uid
		   }
		  got, err := ValidateBookWrite(row)
		  if err != nil {
			   rep.Failed = append(rep.Failed, BatchFail{Index: i, Row: row, Error: err.Error()})
			  continue
		   }
		  prev, ok, err := s.GetBook(ctx, got.UserID, got.Chain, got.Address)
		  if err != nil {
			  return rep, err
		   }
		   if ok {
			   if err := ownEntry(prev, got.UserID); err != nil {
				   rep.Failed = append(rep.Failed, BatchFail{Index: i, Row: row, Error: err.Error()})
				  continue
			  }
			  rep.Accepted = append(rep.Accepted, s.decorate(ctx, prev))
			  continue
		  }
		  need++
		  rep.Accepted = append(rep.Accepted, s.decorate(ctx, got))
	  }
	   if need > 0 {
		   view, err := s.InspectQuota(ctx, uid)
		  if err != nil {
			   return rep, err
		   }
		  if view.Remain < need {
			   return rep, ErrBookQuota
		   }
	   }
	   rep.WouldAdd = need
	  return rep, nil
   }

  func (s *Service) importCommit(ctx context.Context, uid string, rows []BookWrite, rep ImportReport) (ImportReport, error) {
	   for i := range rows {
		  if strings.TrimSpace(rows[i].UserID) == "" {
			   rows[i].UserID = uid
		   }
		   if strings.TrimSpace(rows[i].Actor) == "" {
			   rows[i].Actor = uid
		  }
	   }
	  got, err := s.AddBookBatch(ctx, rows)
	   rep.Accepted, rep.Failed, rep.Total = got.Accepted, got.Failed, got.Total
	  if err != nil {
		   if errors.Is(err, persist.ErrTxClosed) || errors.Is(err, ErrBookQuota) {
			  return rep, err
		  }
		   return rep, err
	   }
	   return rep, nil
  }
