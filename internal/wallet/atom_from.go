package wallet

import (
	"encoding/json"
	"strings"
)

// AtomTx Cosmos LCD/Tendermint 交易最小字段（signer/from_address → FromAddress）。
type AtomTx struct {
	TxHash string `json:"txhash"`
	Hash string `json:"hash"`
	Tx *struct {
		Body *struct {
			Messages []json.RawMessage `json:"messages"`
			Memo     string            `json:"memo"`
		} `json:"body"`
		AuthInfo *struct {
			Fee *struct {
				Payer string `json:"payer"`
			} `json:"fee"`
			SignerInfos []struct {
				PublicKey json.RawMessage `json:"public_key"`
			} `json:"signer_infos"`
		} `json:"auth_info"`
	} `json:"tx"`
	TxResponse *struct {
		TxHash string `json:"txhash"`
	} `json:"tx_response"`
	Result *AtomTx `json:"result"`
	// 扁平字段（部分网关）
	FromAddress string `json:"from_address"`
	Sender      string `json:"sender"`
	Memo        string `json:"memo"`
}

// EnrichFromATOM 从 Cosmos 交易回填付款方（一致 TON/XRP FamMEMO）。
// MsgSend.from_address / fee.payer；缺字段返回空。
// 解析失败不得伪造 bech32；KYT/旅行规则按 unknown。

   func EnrichFromATOM(nativeFrom string, tx AtomTx) string {
	   if fa := strings.TrimSpace(nativeFrom); fa != "" {
		   return fa
	  }
	  if fa := atomAddrClean(tx.FromAddress); fa != "" {
		  return fa
	  }
	  if fa := atomAddrClean(tx.Sender); fa != "" {
		  return fa
	  }
	  if tx.Tx != nil {
		  if tx.Tx.AuthInfo != nil && tx.Tx.AuthInfo.Fee != nil {
			  if fa := atomAddrClean(tx.Tx.AuthInfo.Fee.Payer); fa != "" {
				   return fa
			  }
		  }
		   if tx.Tx.Body != nil {
			   for _, raw := range tx.Tx.Body.Messages {
				  var msg struct {
					  FromAddress string `json:"from_address"`
					   Sender      string `json:"sender"`
					   From        string `json:"from"`
				   }
				  if json.Unmarshal(raw, &msg) != nil {
					  continue
				   }
				   for _, cand := range []string{msg.FromAddress, msg.Sender, msg.From} {
					  if fa := atomAddrClean(cand); fa != "" {
						  return fa
					   }
				   }
			   }
		  }
	   }
	  return ""
  }

   func atomAddrClean(s string) string {
	  s = strings.TrimSpace(s)
	  if s == "" {
		  return ""
	  }
	   // bech32 cosmos1…/osmo1… 等；过短拒绝
	  if len(s) < 20 {
		  return ""
	  }
	   return s
   }

// DecodeAtomTxJSON 解析 Cosmos LCD txs / Tendermint tx 结果。

   func DecodeAtomTxJSON(raw []byte) (AtomTx, error) {
	  var wrap struct {
		   Result json.RawMessage `json:"result"`
	   }
	  if json.Unmarshal(raw, &wrap) == nil && len(wrap.Result) > 0 && string(wrap.Result) != "null" {
		  raw = wrap.Result
	  }
	   var tx AtomTx
	  if err := json.Unmarshal(raw, &tx); err != nil {
		   return AtomTx{}, err
	  }
	  if tx.Tx == nil && tx.Result != nil {
		  tx = *tx.Result
	   }
	  return tx, nil
  }
