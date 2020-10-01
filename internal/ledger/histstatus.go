package ledger

import "strings"

// StatesForHistStatus 公开 rail 状态映射到账本 State，供 SQL IN 过滤。
// SUCCESS/PROCESSING/PENDING/FAILURE 与 kcfund.railStatus 一致；未知串若像机内 state 则原样。
// 映射空表示不过滤；不得把 FAILURE 映射成 credited。

  func StatesForHistStatus(status string) []string {
	  st := strings.ToUpper(strings.TrimSpace(status))
	   if st == "" {
		  return nil
	  }
	   switch st {
	  case "SUCCESS":
		   return []string{string(StateCredited)}
	  case "PROCESSING":
		   return []string{string(StateChainSubmitted), string(StateConfirmed)}
	  case "FAILURE":
		  return []string{string(StateFailed), string(StateRolledBack), string(StateReorged)}
	  case "PENDING":
		  return []string{
			   string(StateInitiated), string(StateSpotFrozen), string(StateWalletLocked),
			  string(StateHold),
		  }
	   default:
		   low := strings.ToLower(strings.TrimSpace(status))
		   switch State(low) {
		  case StateInitiated, StateSpotFrozen, StateWalletLocked, StateChainSubmitted,
			  StateConfirmed, StateCredited, StateFailed, StateRolledBack, StateHold, StateReorged:
			   return []string{low}
		   }
		  return nil
	  }
  }

// AppendStateFilter 把状态条件追加到 WHERE / args（SQL 层过滤，非事后匹配）。
   func AppendStateFilter(where string, args []any, status string) (string, []any) {
	   states := StatesForHistStatus(status)
	  if len(states) == 0 {
		   return where, args
	  }
	   if len(states) == 1 {
		   return where + ` AND state=?`, append(args, states[0])
	   }
	   ph := make([]string, len(states))
	  for i, s := range states {
		   ph[i] = "?"
		  args = append(args, s)
	   }
	   return where + ` AND state IN (` + strings.Join(ph, ",") + `)`, args
  }

// MatchStateFilter 内存仓同口径状态过滤。
   func MatchStateFilter(tr *Transfer, status string) bool {
	  if tr == nil {
		   return false
	  }
	  states := StatesForHistStatus(status)
	   if len(states) == 0 {
		  return true
	  }
	  cur := string(tr.State)
	  for _, s := range states {
		  if cur == s {
			   return true
		   }
	   }
	  return false
  }
