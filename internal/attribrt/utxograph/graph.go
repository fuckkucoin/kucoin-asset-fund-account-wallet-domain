package utxograph

import (
	"strings"
	"account-assets.kucoin.net/funding/pkg/money"
)

// Eval 给一笔交易打标。输入为空时不猜测找零。

   func Eval(tx Tx, lim Limits, bind Binder) (Report, error) {
	  tx.Chain = strings.ToUpper(strings.TrimSpace(tx.Chain))
	  tx.TxID = strings.TrimSpace(tx.TxID)
	   if tx.TxID == "" {
		  tx.TxID = strings.TrimSpace(firstTx(tx))
	  }
	  if len(tx.Inputs) == 0 || len(tx.Outputs) == 0 {
		   return Report{Incomplete: true, Reason: "utxo_graph_required"}, nil
	   }
	   users := inputUsers(tx, bind)
	   merge := len(users) > 1
	   coins, tickets := labelOutputs(tx, lim, bind, merge)
	   fee,
		  err := conserve(tx.Inputs, tx.Outputs, strings.TrimSpace(tx.Fee), lim.FeeMin, lim.FeeMaxBps)
	   rep := Report{Coins: coins, Tickets: tickets, Fee: fee, Merge: merge}
	  if err != nil {
		  rep.ConserveOK = false
		  rep.Reason = "utxo_conserve"
		   for i := range rep.Coins {
			  rep.Coins[i].Credit = false
			   if rep.Coins[i].Reason == "" {
				   rep.Coins[i].Reason = "utxo_conserve"
			   }
		  }
		   rep.Tickets = append(rep.Tickets, Ticket{
			  Kind: "conserve", TxID: tx.TxID, Vout: -1, State: "open", Note: err.Error(),
		  })
		  return rep, nil
	   }
	  rep.ConserveOK = true
	   if merge {
		  rep.Reason = "utxo_merge"
		  rep.Tickets = append(rep.Tickets, Ticket{
			   Kind: "merge", TxID: tx.TxID, Vout: -1, State: "open",
			  Note: strings.Join(users, ","),
		  })
	   }
	   return rep, nil
  }

  func inputUsers(tx Tx, bind Binder) []string {
	   if bind == nil {
		  return nil
	  }
	   seen := map[string]struct{}{}
	   var out []string
	  for _, in := range tx.Inputs {
		   user, ok := bind(in.Address, coinKey(in.TxID, in.Vout))
		  if !ok || user == "" {
			   continue
		  }
		  if _, dup := seen[user]; dup {
			   continue
		  }
		  seen[user] = struct{}{}
		   out = append(out, user)
	  }
	  return out
   }

  func firstTx(tx Tx) string {
	  if len(tx.Outputs) > 0 && tx.Outputs[0].TxID != "" {
		  return tx.Outputs[0].TxID
	  }
	  if len(tx.Inputs) > 0 {
		  return tx.Inputs[0].TxID
	   }
	   return ""
   }

   func cmp(a, b string) int {
	  aa, err1 := money.Parse(a)
	   bb, err2 := money.Parse(b)
	  if err1 != nil || err2 != nil {
		   return 0
	  }
	  return aa.Cmp(bb)
  }

// Pick 按输出序号取打标结果。没有该序号时 ok 为 false。

   func Pick(rep Report, vout int) (Labeled, bool) {
	  for _, c := range rep.Coins {
		  if c.Vout == vout {
			  return c, true
		   }
	  }
	  return Labeled{}, false
  }
