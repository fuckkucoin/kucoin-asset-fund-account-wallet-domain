package xferloop

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/ledger"
)

// UserStatus 历史与运维共用的读面。

type UserStatus struct {
	Code string `json:"code"`
	Desc string `json:"desc"`
}

   func mapUserStatus(c *Case) UserStatus {
	  if c == nil {
		   return UserStatus{Code: ledger.RailRecovering, Desc: "close-loop recovering"}
	   }
	  if c.State == StateRecovered {
		   return UserStatus{}
	  }
	   if c.State == StateEscalated {
		  switch c.Kind {
		   case KindInner:
			   return UserStatus{Code: ledger.RailCounterparty, Desc: "counterparty not credited"}
		   case KindCover:
			  return UserStatus{Code: ledger.RailCoverStranded, Desc: "cover stranded"}
		  default:
			   if strings.HasPrefix(c.Edge, "hop_comp") || strings.HasPrefix(c.Phase, "aborting") {
				   return UserStatus{Code: ledger.RailEscalated, Desc: "hop compensate escalated"}
			  }
			  return UserStatus{Code: ledger.RailEscalated, Desc: "close-loop escalated"}
		  }
	   }
	  switch c.Kind {
	   case KindInner:
		   if strings.HasPrefix(c.Phase, "counterparty_") || c.Edge == "inner_self" || c.Edge == "inner_no_dest" || c.Edge == "inner_amount" {
			   return UserStatus{Code: ledger.RailCounterparty, Desc: "counterparty not credited"}
		  }
		   return UserStatus{Code: ledger.RailRecovering, Desc: "close-loop recovering"}
	  case KindCover:
		   return UserStatus{Code: ledger.RailCoverStranded, Desc: "cover stranded"}
	  default:
		  if strings.HasPrefix(c.Phase, "aborting") || c.Edge == "hop_compensate" || c.Edge == "hop_compensate_infer" {
			  return UserStatus{Code: ledger.RailHopCompensate, Desc: "hop compensating"}
		  }
		   return UserStatus{Code: ledger.RailRecovering, Desc: "close-loop recovering"}
	  }
   }

// StatusCatalog 用户读面，给操作手册和运维查询。
   func StatusCatalog() []UserStatus {
	   return []UserStatus{
		   {Code: ledger.RailRecovering, Desc: "自动收口尚未结束"},
		   {Code: ledger.RailHopCompensate, Desc: "母子划转正在补偿，不会按已 hopped 冲正"},
		   {Code: ledger.RailEscalated, Desc: "自动收口失败，已升级人工"},
		  {Code: ledger.RailCounterparty, Desc: "内部转对手贷记缺失、短少、多记或对手无效"},
		  {Code: ledger.RailCoverStranded, Desc: "自动补账未能划回"},
	   }
  }
