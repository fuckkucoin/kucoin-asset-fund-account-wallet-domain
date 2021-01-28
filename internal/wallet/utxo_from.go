package wallet

import (
	"encoding/json"
	"strings"
)

// BtcTxVerbose BTC getrawtransaction verbose 最小字段（UTXO from 回填）。
type BtcTxVerbose struct {
	TxID string `json:"txid"`
	Hash string    `json:"hash"`
	Vin  []BtcVin  `json:"vin"`
	Vout []BtcVout `json:"vout"`
	From string    `json:"from"` // 部分适配层已摊平
}

// BtcVin 输入侧；付款方来自 vin 地址或 prevout.scriptPubKey。

type BtcVin struct {
	Coinbase string `json:"coinbase"`
	TxID     string `json:"txid"`
	Vout     int    `json:"vout"`
	Address   string      `json:"address"`
	Addresses []string    `json:"addresses"`
	Prevout   *BtcPrevout `json:"prevout"`
}

// BtcPrevout 输入对应的前序输出（部分节点/索引器回填）。
type BtcPrevout struct {
	Value float64 `json:"value"`
	ScriptPubKey BtcScriptKey `json:"scriptPubKey"`
}

// BtcVout 输出侧（收款方；本函数不用于 from）。
type BtcVout struct {
	N            int          `json:"n"`
	Value        float64      `json:"value"`
	ScriptPubKey BtcScriptKey `json:"scriptPubKey"`
}

// BtcScriptKey 脚进程内址集合。

type BtcScriptKey struct {
	Address string `json:"address"`
	Addresses []string `json:"addresses"`
	Type string `json:"type"`
}

// EnrichFromUTXO 从 BTC verbose 交易回填付款方（一致 ETH ERC20 Transfer 日志路径）。
// 原生 from 先；空时取首个非 coinbase vin 的地址/prevout；多 vin 取首个非空。
// coinbase 无付款方返回空；不得伪造地址；缺 prevout/地址时返回空串。

  func EnrichFromUTXO(nativeFrom string, tx BtcTxVerbose) string {
	   if fa := strings.TrimSpace(nativeFrom); fa != "" {
		   return fa
	   }
	  if fa := strings.TrimSpace(tx.From); fa != "" {
		   return fa
	   }
	   for _, vin := range tx.Vin {
		   if strings.TrimSpace(vin.Coinbase) != "" {
			   continue
		  }
		   if fa := firstAddr(vin.Address, vin.Addresses); fa != "" {
			   return fa
		   }
		   if vin.Prevout != nil {
			  if fa := firstAddr(vin.Prevout.ScriptPubKey.Address, vin.Prevout.ScriptPubKey.Addresses); fa != "" {
				  return fa
			   }
		   }
	   }
	   return ""
   }

  func firstAddr(single string, many []string) string {
	  if fa := strings.TrimSpace(single); fa != "" {
		  return fa
	   }
	  for _, a := range many {
		   if fa := strings.TrimSpace(a); fa != "" {
			   return fa
		   }
	   }
	  return ""
   }

// DecodeBtcTxVerboseJSON 解析 getrawtransaction verbose JSON。

   func DecodeBtcTxVerboseJSON(raw []byte) (BtcTxVerbose, error) {
	   var tx BtcTxVerbose
	  if err := json.Unmarshal(raw, &tx); err != nil {
		  return BtcTxVerbose{}, err
	  }
	  return tx, nil
  }
