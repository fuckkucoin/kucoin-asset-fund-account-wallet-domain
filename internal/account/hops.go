// 账户账本领域之簿间 hop 划转。

// Author: Blaise
// Date: 2020.09.17

package account

import "account-assets.kucoin.net/funding/internal/ledger"

type Hop struct {
	From Book `json:"from"`
	To Book `json:"to"`
	Dir ledger.Direction `json:"direction"`
}

// LegalHops 资金账与交易/全仓/逐仓/合约/理财之间的合法 hop。科目之间不得直跳。
   func LegalHops() []Hop {
	   pairs := [][2]Book{
		  {BookFunding, BookTrading},
		  {BookTrading, BookFunding},
		   {BookFunding, BookMargin},
		  {BookMargin, BookFunding},
		   {BookFunding, BookIsolated},
		  {BookIsolated, BookFunding},
		   {BookFunding, BookFutures},
		   {BookFutures, BookFunding},
		  {BookFunding, BookEarn},
		  {BookEarn, BookFunding},
	  }
	   out := make([]Hop, 0, len(pairs))
	   for _, p := range pairs {
		  dir, err := MoveDirection(p[0], p[1])
		  if err != nil {
			  continue
		   }
		  out = append(out,
			  Hop{From: p[0], To: p[1], Dir: dir})
	   }
	   return out
   }

  func HopAllowed(from, to Book) bool {
	   _, err := MoveDirection(from, to)
	   return err == nil
   }
