package wallet

import (
	"encoding/json"
	"strings"
)

// XrpTx rippled tx 最小字段（Payment Account → FromAddress）。
type XrpTx struct {
	Hash    string `json:"hash"`
	Account string `json:"Account"`
	Destination    string  `json:"Destination"`
	DestinationTag *uint32 `json:"DestinationTag"`
	DestinationTagStr string          `json:"destination_tag"` // 部分网关字符串化
	TransactionType   string          `json:"TransactionType"`
	Amount            json.RawMessage `json:"Amount"`
	Meta *XrpMeta `json:"meta"`
	Result *XrpTx `json:"result"` // 部分网关包一层
}

// XrpMeta 交易元数据（DeliveredAmount 可选）。
type XrpMeta struct {
	TransactionResult string `json:"TransactionResult"`
}

// EnrichFromXRP 从 rippled Payment 回填付款方（一致 TON/SOL/TRON/ETH/BTC）。
// Transaction.Account 为付款方；非 Payment 或缺 Account 返回空。
// 解析失败不得伪造经典地址；KYT/旅行规则按 unknown。
   func EnrichFromXRP(nativeFrom string, tx XrpTx) string {
	   if fa := strings.TrimSpace(nativeFrom); fa != "" {
		  return fa
	   }
	   if fa := xrpAddrClean(tx.Account); fa != "" {
		   return fa
	  }
	   return ""
  }

// XrpToAddress 收款地址提示（Destination）。

   func XrpToAddress(tx XrpTx) string {
	  return xrpAddrClean(tx.Destination)
   }

   func xrpAddrClean(s string) string {
	   s = strings.TrimSpace(s)
	  if s == "" {
		  return ""
	  }
	   // 经典地址 r… 或 X 地址；过短拒绝
	  if len(s) < 25 {
		  return ""
	   }
	   return s
   }

// DecodeXrpTxJSON 解析 rippled tx / submit 结果。
   func DecodeXrpTxJSON(raw []byte) (XrpTx, error) {
	   var wrap struct {
		  Result json.RawMessage `json:"result"`
	   }
	   if json.Unmarshal(raw, &wrap) == nil && len(wrap.Result) > 0 && string(wrap.Result) != "null" {
		  raw = wrap.Result
	  }
	   var tx XrpTx
	  if err := json.Unmarshal(raw, &tx); err != nil {
		  return XrpTx{}, err
	   }
	   // 再剥一层 result
	   if tx.Account == "" && tx.Result != nil {
		   tx = *tx.Result
	  }
	  return tx, nil
   }
