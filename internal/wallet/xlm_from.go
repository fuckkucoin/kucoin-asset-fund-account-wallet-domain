package wallet

import (
	"encoding/json"
	"strings"
)

// XlmTx Horizon/Stellar 交易最小字段（source_account → FromAddress）。
type XlmTx struct {
	Hash string `json:"hash"`
	ID            string `json:"id"`
	SourceAccount string `json:"source_account"`
	From       string `json:"from"`
	To         string `json:"to"`
	Account    string `json:"account"`
	Successful *bool  `json:"successful"`
	Memo string `json:"memo"`
	MemoType string `json:"memo_type"`
	// 部分网关包 result / transaction
	Result *XlmTx `json:"result"`
	Transaction *XlmTx `json:"transaction"`
}

// EnrichFromXLM 从 Stellar/Horizon 交易回填付款方（一致 TON/XRP FamMEMO）。
// source_account 为付款方；缺字段返回空。
// 解析失败不得伪造 G… 地址；KYT/旅行规则按 unknown。
  func EnrichFromXLM(nativeFrom string, tx XlmTx) string {
	  if fa := strings.TrimSpace(nativeFrom); fa != "" {
		  return fa
	  }
	   for _, cand := range []string{tx.SourceAccount, tx.From, tx.Account} {
		  if fa := xlmAddrClean(cand); fa != "" {
			  return fa
		   }
	   }
	   return ""
   }

// XlmToAddress 收款地址提示。
   func XlmToAddress(tx XlmTx) string {
	   return xlmAddrClean(tx.To)
   }

   func xlmAddrClean(s string) string {
	   s = strings.TrimSpace(s)
	  if s == "" {
		  return ""
	  }
	   // Stellar 账户 G… / M… 约 56 字符；过短拒绝
	  if len(s) < 40 {
		   return ""
	   }
	   return s
  }

// DecodeXlmTxJSON 解析 Horizon transaction / 网关包装结果。

  func DecodeXlmTxJSON(raw []byte) (XlmTx, error) {
	   var wrap struct {
		   Result      json.RawMessage `json:"result"`
		   Transaction json.RawMessage `json:"transaction"`
	   }
	   if json.Unmarshal(raw, &wrap) == nil {
		  if len(wrap.Result) > 0 && string(wrap.Result) != "null" {
			   raw = wrap.Result
		   } else if len(wrap.Transaction) > 0 && string(wrap.Transaction) != "null" {
			   raw = wrap.Transaction
		   }
	   }
	   var tx XlmTx
	   if err := json.Unmarshal(raw, &tx); err != nil {
		   return XlmTx{}, err
	  }
	   if tx.SourceAccount == "" && tx.Result != nil {
		   tx = *tx.Result
	   }
	  if tx.SourceAccount == "" && tx.Transaction != nil {
		   tx = *tx.Transaction
	   }
	   return tx, nil
  }
