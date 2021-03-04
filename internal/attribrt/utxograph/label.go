package utxograph

import "strings"

  func labelOutputs(tx Tx, lim Limits, bind Binder, merge bool) ([]Labeled, []Ticket) {
	   inAddr := map[string]struct{}{}
	  for _, in := range tx.Inputs {
		  if a := normAddr(in.Address); a != "" {
			  inAddr[a] = struct{}{}
		   }
	   }
	  hot := map[string]struct{}{}
	   for _, h := range lim.Hot {
		   if a := normAddr(h); a != "" {
			  hot[a] = struct{}{}
		   }
	  }
	   out := make([]Labeled, 0, len(tx.Outputs))
	   var tickets []Ticket
	  for _, c := range tx.Outputs {
		  lab := Labeled{Coin: c, Label: "unbound", Reason: "utxo_unbound"}
		   script := strings.TrimSpace(c.Script)
		   if script == "nulldata" || script == "op_return" {
			   lab.Label = "nulldata"
			   lab.Reason = "noise_script"
			  lab.Credit = false
			  out = append(out, lab)
			  continue
		   }
		   if script != "" && !knownScript(script, lim.Scripts) {
			  lab.Label = "unknown_script"
			  lab.Reason = "noise_script"
			   lab.Credit = false
			  tickets = append(tickets, Ticket{
				  Kind: "unknown_script", TxID: tx.TxID, Vout: c.Vout, Amount: c.Amount,
				   Address: c.Address, State: "open", Note: script,
			   })
			   out = append(out, lab)
			   continue
		  }
		   dustFloor := scriptDust(strings.ToUpper(tx.Chain), script, lim.Dust)
		  if below(c.Amount, dustFloor) {
			  lab.Label = "dust"
			   lab.Reason = "noise_dust"
			   lab.Credit = false
			   out = append(out, lab)
			  continue
		   }
		  addr := normAddr(c.Address)
		   if addr != "" {
			  if _, ok := inAddr[addr]; ok {
				   lab.Label = "change"
				   lab.Reason = "utxo_change"
				  out = append(out, lab)
				  tickets = append(tickets, Ticket{
					   Kind: "change", TxID: tx.TxID, Vout: c.Vout, Amount: c.Amount,
					   Address: c.Address, State: "labeled", Note: "input address",
				  })
				  continue
			   }
			  if _, ok := hot[addr]; ok {
				   lab.Label = "change"
				  lab.Reason = "utxo_change"
				   out = append(out, lab)
				  tickets = append(tickets, Ticket{
					   Kind: "change", TxID: tx.TxID, Vout: c.Vout, Amount: c.Amount,
					  Address: c.Address, State: "labeled", Note: "hot change",
				   })
				  continue
			  }
		   }
		   user, ok := "", false
		   if bind != nil {
			   user,
				   ok = bind(c.Address, coinKey(c.TxID, c.Vout))
			  if !ok {
				  user, ok = bind(c.Address, coinKey(tx.TxID, c.Vout))
			  }
		  }
		  if ok && user != "" {
			  lab.UserID = user
			  lab.Label = "payment"
			   lab.Reason = ""
			   lab.Credit = !merge
			   if merge {
				  lab.Reason = "utxo_merge"
				   lab.Credit = false
			  }
			  out = append(out, lab)
			   continue
		  }
		   // 恰好一笔已识别付款、且只剩一个陌生输出时，陌生输出视作找零。
		  lab.Label = "unbound"
		  lab.Reason = "utxo_unbound"
		  out = append(out, lab)
	   }
	   out, tickets = markLoneChange(out, tickets, tx.TxID)
	   return out, tickets
   }

// markLoneChange 有且仅有一个未绑定输出、且已有付款时，把该输出改标找零。
// 多个未绑定输出保持工单，不猜哪个是找零。
  func markLoneChange(coins []Labeled, tickets []Ticket, txid string) ([]Labeled, []Ticket) {
	  pay := 0
	  unboundIdx := -1
	  unboundN := 0
	  for i, c := range coins {
		   if c.Label == "payment" {
			   pay++
		  }
		  if c.Label == "unbound" {
			  unboundN++
			  unboundIdx = i
		  }
	   }
	   if pay >= 1 && unboundN == 1 && unboundIdx >= 0 {
		  c := &coins[unboundIdx]
		   c.Label = "change"
		   c.Reason = "utxo_change"
		  c.Credit = false
		   c.UserID = ""
		   tickets = append(tickets, Ticket{
			   Kind: "change", TxID: txid, Vout: c.Vout, Amount: c.Amount,
			  Address: c.Address, State: "labeled", Note: "lone remainder",
		  })
		   return coins, tickets
	  }
	  for _, c := range coins {
		   if c.Label != "unbound" {
			   continue
		   }
		   tickets = append(tickets, Ticket{
			   Kind: "unbound", TxID: txid, Vout: c.Vout, Amount: c.Amount,
			   Address: c.Address, State: "open", Note: "no user binding",
		   })
	  }
	  return coins, tickets
   }

   func normAddr(s string) string {
	   return strings.ToLower(strings.TrimSpace(s))
   }

   func below(amount, floor string) bool {
	  if floor == "" || floor == "0" {
		   return false
	  }
	  return cmp(amount, floor) < 0
   }
