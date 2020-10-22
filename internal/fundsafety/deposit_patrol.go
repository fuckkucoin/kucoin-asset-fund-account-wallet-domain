package fundsafety

import (
	"account-assets.kucoin.net/funding/internal/deposit"
	"account-assets.kucoin.net/funding/internal/recon"
)

// DepositPatrol 跑观测。窗口内差额落单，并把字段追加到对账工单。
  func (s *Service) DepositPatrol(in deposit.PatrolIn, book *recon.Book) (deposit.PatrolOut, error) {
	  if s == nil {
		  return deposit.PatrolOut{}, ErrUnavailable
	   }
	  if s.dep == nil {
		  s.dep = deposit.New(s.led, nil)
	  }
	   if book != nil {
		  s.dep.BindRecon(book)
	   }
	  return s.dep.ApplyPatrol(in)
  }

// DepositSweep 连续多笔。任一笔差额都带上对账字段；提现请求不在此关闭。
  func (s *Service) DepositSweep(rows []deposit.PatrolIn, book *recon.Book) ([]deposit.PatrolOut, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	  }
	  out := make([]deposit.PatrolOut, 0, len(rows))
	   var first error
	   for _, row := range rows {
		   got, err := s.DepositPatrol(row, book)
		  out = append(out, got)
		  if err != nil && first == nil {
			   first = err
		   }
	  }
	  return out, first
  }

// DepositFace 只读确认中 / 已冲正。
   func (s *Service) DepositFace(chain, hash string, output int) (deposit.Face, error) {
	  if s == nil || s.dep == nil {
		   return deposit.Face{}, deposit.ErrFaceMiss
	   }
	  return s.dep.ReadFace(chain, hash, output)
   }
