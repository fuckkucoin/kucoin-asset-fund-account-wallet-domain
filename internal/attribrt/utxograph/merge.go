package utxograph

// MergeUsers 从报告工单里抽出合并涉及的用户。没有合并时返回空。
   func MergeUsers(rep Report) []string {
	  if !rep.Merge {
		  return nil
	  }
	  for _, t := range rep.Tickets {
		   if t.Kind != "merge" || t.Note == "" {
			   continue
		  }
		   parts := splitComma(t.Note)
		  return parts
	   }
	   return nil
  }

  func splitComma(s string) []string {
	  var out []string
	   start := 0
	  for i := 0; i <= len(s); i++ {
		  if i == len(s) || s[i] == ',' {
			   p := s[start:i]
			  if p != "" {
				  out = append(out, p)
			   }
			   start = i + 1
		   }
	   }
	  return out
   }

// PaymentUsers 可以入账的付款用户。找零和未绑定不算。

  func PaymentUsers(rep Report) []string {
	   seen := map[string]struct{}{}
	  var out []string
	   for _, c := range rep.Coins {
		   if !c.Credit || c.UserID == "" || c.Label != "payment" {
			   continue
		  }
		  if _, ok := seen[c.UserID]; ok {
			   continue
		   }
		  seen[c.UserID] = struct{}{}
		  out = append(out, c.UserID)
	   }
	  return out
  }
