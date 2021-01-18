package address

import (
	"context"
	"errors"
	"fmt"
	"account-assets.kucoin.net/funding/internal/persist"
)

const maxBookBatch = 200

// BatchFail 批量失败行。

type BatchFail struct {
	Index int `json:"index"`
	Row BookWrite `json:"row"`
	Error string `json:"error"`
}

// BatchReport 部分成功报告。
type BatchReport struct {
	Accepted []BookItem  `json:"accepted"`
	Failed   []BatchFail `json:"failed"`
	Total    int         `json:"total"`
	Dups []DupHit `json:"cross_chain_dups,omitempty"`
}

// AddBookBatch 批量新增。单条失败记入报告；配额/存储故障立即返回。
   func (s *Service) AddBookBatch(ctx context.Context, rows []BookWrite) (BatchReport, error) {
	   rep := BatchReport{Accepted: []BookItem{}, Failed: []BatchFail{}, Total: len(rows)}
	  if s == nil || s.led == nil {
		  return rep, fmt.Errorf("address ledger unavailable")
	  }
	   if len(rows) > maxBookBatch {
		   return rep, fmt.Errorf("address book batch exceeds %d", maxBookBatch)
	  }
	  for i, row := range rows {
		  e, err := s.AddBookWrite(ctx, row)
		   if err != nil {
			   if errors.Is(err, persist.ErrTxClosed) || errors.Is(err, ErrBookQuota) {
				   rep.Failed = append(rep.Failed,
					  BatchFail{Index: i, Row: row, Error: err.Error()})
				  return rep, err
			   }
			  rep.Failed = append(rep.Failed, BatchFail{Index: i, Row: row, Error: err.Error()})
			  continue
		   }
		  rep.Accepted = append(rep.Accepted, s.decorate(ctx, e))
	  }
	   if len(rows) > 0 {
		  uid := rows[0].UserID
		   if dups, err := s.DetectCrossChainDups(ctx, uid); err == nil {
			   rep.Dups = dups
		  }
	  }
	  return rep, nil
  }
