package wallet

import (
	"encoding/json"
	"strings"
)

// EosTx EOS get_transaction 最小字段（transfer.from / authorization.actor → FromAddress）。
type EosTx struct {
	ID string `json:"id"`
	Trx *struct {
		Receipt json.RawMessage `json:"receipt"`
		Trx *struct {
			Actions []EosAction `json:"actions"`
		} `json:"trx"`
		Actions []EosAction `json:"actions"`
	} `json:"trx"`
	Transaction *struct {
		Actions []EosAction `json:"actions"`
	} `json:"transaction"`
	Actions []EosAction `json:"actions"`
	Result *EosTx `json:"result"`
}

// EosAction EOS action（transfer / 授权）。
type EosAction struct {
	Account string `json:"account"`
	Name string `json:"name"`
	Authorization []EosAuth       `json:"authorization"`
	Data          json.RawMessage `json:"data"`
}

// EosAuth EOS 授权 actor。

type EosAuth struct {
	Actor string `json:"actor"`
	Permission string `json:"permission"`
}

// EnrichFromEOS 从 EOS 交易回填付款方（一致 TON/XRP FamMEMO）。
// transfer data.from；否则首个 authorization.actor。
// 解析失败不得伪造账户名；KYT/旅行规则按 unknown。
  func EnrichFromEOS(nativeFrom string, tx EosTx) string {
	  if fa := strings.TrimSpace(nativeFrom); fa != "" {
		   return fa
	  }
	  actions := eosActions(tx)
	   for _, act := range actions {
		  if from := eosActionFrom(act); from != "" {
			  return from
		  }
	   }
	   return ""
  }

   func eosActions(tx EosTx) []EosAction {
	   if len(tx.Actions) > 0 {
		   return tx.Actions
	   }
	  if tx.Transaction != nil && len(tx.Transaction.Actions) > 0 {
		  return tx.Transaction.Actions
	  }
	   if tx.Trx != nil {
		   if tx.Trx.Trx != nil && len(tx.Trx.Trx.Actions) > 0 {
			   return tx.Trx.Trx.Actions
		   }
		  if len(tx.Trx.Actions) > 0 {
			   return tx.Trx.Actions
		   }
	  }
	   return nil
   }

   func eosActionFrom(act EosAction) string {
	   if len(act.Data) > 0 && string(act.Data) != "null" {
		   var data struct {
			   From string `json:"from"`
			  To   string `json:"to"`
		  }
		  if json.Unmarshal(act.Data, &data) == nil {
			   if fa := eosAddrClean(data.From); fa != "" {
				  return fa
			  }
		   }
		   // data 有时是字符串化 JSON
		  var asStr string
		  if json.Unmarshal(act.Data, &asStr) == nil && asStr != "" {
			  if json.Unmarshal([]byte(asStr), &data) == nil {
				   if fa := eosAddrClean(data.From); fa != "" {
					   return fa
				  }
			   }
		  }
	   }
	   for _, auth := range act.Authorization {
		   if fa := eosAddrClean(auth.Actor); fa != "" {
			  return fa
		  }
	  }
	  return ""
  }

  func eosAddrClean(s string) string {
	  s = strings.TrimSpace(s)
	   if s == "" {
		   return ""
	  }
	  // EOS 账户名最长 12；过短/过长拒绝
	  if len(s) < 1 || len(s) > 13 {
		  return ""
	   }
	  return s
  }

// DecodeEosTxJSON 解析 EOS get_transaction / 网关包装结果。

   func DecodeEosTxJSON(raw []byte) (EosTx, error) {
	   var wrap struct {
		   Result json.RawMessage `json:"result"`
	  }
	   if json.Unmarshal(raw, &wrap) == nil && len(wrap.Result) > 0 && string(wrap.Result) != "null" {
		  raw = wrap.Result
	  }
	  var tx EosTx
	   if err := json.Unmarshal(raw, &tx); err != nil {
		  return EosTx{}, err
	   }
	   if tx.Trx == nil && tx.Transaction == nil && len(tx.Actions) == 0 && tx.Result != nil {
		  tx = *tx.Result
	  }
	  return tx, nil
   }
