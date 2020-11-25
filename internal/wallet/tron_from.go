package wallet

import (
	"encoding/json"
	"strings"
)

// TronNativeTx TRON wallet/gettransactionbyid 最小字段（非 EVM 路径）。

type TronNativeTx struct {
	TxID string `json:"txID"`
	RawData TronRawData `json:"raw_data"`
	Ret []TronRet `json:"ret"`
}

// TronRawData 合约列表。
type TronRawData struct {
	Contract []TronContract `json:"contract"`
}

// TronContract TriggerSmartContract / TransferContract。
type TronContract struct {
	Type string `json:"type"`
	Parameter struct {
		Value TronContractValue `json:"value"`
	} `json:"parameter"`
}

// TronContractValue owner / to / contract 地址（hex 或 base58）。
type TronContractValue struct {
	OwnerAddress string `json:"owner_address"`
	ToAddress       string `json:"to_address"`
	ContractAddress string `json:"contract_address"`
	Data            string `json:"data"`
}

// TronRet 执行结果。
type TronRet struct {
	ContractRet string `json:"contractRet"`
}

// TronTxInfo wallet/gettransactioninfobyid（含 log，TRC20 Transfer）。
type TronTxInfo struct {
	ID string `json:"id"`
	Fee int64 `json:"fee"`
	Log     []TronLogItem `json:"log"`
	Receipt struct {
		Result string `json:"result"`
	} `json:"receipt"`
}

// TronLogItem TRC20 事件日志（topics 无 0x 前缀时需补齐）。

type TronLogItem struct {
	Address string   `json:"address"`
	Topics  []string `json:"topics"`
	Data    string   `json:"data"`
}

// EnrichFromTRON 从原生 TRON 交易 + info 日志回填付款方（超越 eth_* 兼容层）。
// owner_address 先；空时解析 TRC20 Transfer topic1；合约可过滤。
// 缺字段返回空，不得把合约地址当付款方。

  func EnrichFromTRON(nativeFrom string, tx TronNativeTx, info *TronTxInfo, contractFilter string) string {
	  if fa := strings.TrimSpace(nativeFrom); fa != "" {
		  return fa
	   }
	  for _, c := range tx.RawData.Contract {
		  if fa := strings.TrimSpace(c.Parameter.Value.OwnerAddress); fa != "" {
			  return fa
		   }
	  }
	   if info != nil {
		  want := strings.ToLower(strings.TrimSpace(contractFilter))
		  for _, lg := range info.Log {
			   from, _, ok := ParseTRC20Transfer(lg)
			   if !ok || from == "" {
				   continue
			  }
			   if want != "" && !strings.EqualFold(strings.TrimSpace(lg.Address), want) &&
				   !strings.EqualFold(tronAddrNorm(lg.Address), tronAddrNorm(want)) {
				   continue
			   }
			  return from
		  }
		  for _, lg := range info.Log {
			  from, _, ok := ParseTRC20Transfer(lg)
			  if ok && from != "" {
				  return from
			   }
		  }
	  }
	   return ""
   }

// ParseTRC20Transfer 解析 TRC20 Transfer 日志 from/to（一致 ERC20 topic0）。
   func ParseTRC20Transfer(lg TronLogItem) (from, to string, ok bool) {
	   if len(lg.Topics) < 3 {
		   return "", "", false
	   }
	   want := strings.TrimPrefix(strings.ToLower(erc20TransferTopic0), "0x")
	   if normalizeTopic(lg.Topics[0]) != want {
		  return "", "", false
	   }
	  from = topicAddress(normalizeTopicHex(lg.Topics[1]))
	   to = topicAddress(normalizeTopicHex(lg.Topics[2]))
	   if from == "" && to == "" {
		   return "", "", false
	   }
	  return from, to, true
   }

  func normalizeTopic(t string) string {
	  t = strings.ToLower(strings.TrimSpace(t))
	  t = strings.TrimPrefix(t, "0x")
	  return t
  }

   func normalizeTopicHex(t string) string {
	  t = strings.TrimSpace(t)
	   if t == "" {
		  return ""
	   }
	   if !strings.HasPrefix(strings.ToLower(t), "0x") {
		   return "0x" + t
	   }
	  return t
  }

   func tronAddrNorm(a string) string {
	   return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(a), "0x"))
  }

// TronToAddress 收款地址提示。
   func TronToAddress(tx TronNativeTx) string {
	   for _, c := range tx.RawData.Contract {
		   if fa := strings.TrimSpace(c.Parameter.Value.ToAddress); fa != "" {
			  return fa
		  }
	   }
	  return ""
  }

// DecodeTronNativeTxJSON 解析 wallet/gettransactionbyid JSON。
  func DecodeTronNativeTxJSON(raw []byte) (TronNativeTx, error) {
	   var wrap struct {
		  Result json.RawMessage `json:"result"`
	   }
	   if json.Unmarshal(raw, &wrap) == nil && len(wrap.Result) > 0 && string(wrap.Result) != "null" {
		   raw = wrap.Result
	   }
	   var tx TronNativeTx
	  if err := json.Unmarshal(raw, &tx); err != nil {
		   return TronNativeTx{}, err
	  }
	  return tx, nil
  }

// DecodeTronTxInfoJSON 解析 wallet/gettransactioninfobyid JSON。

   func DecodeTronTxInfoJSON(raw []byte) (TronTxInfo, error) {
	  var wrap struct {
		   Result json.RawMessage `json:"result"`
	   }
	   if json.Unmarshal(raw, &wrap) == nil && len(wrap.Result) > 0 && string(wrap.Result) != "null" {
		  raw = wrap.Result
	   }
	   var info TronTxInfo
	  if err := json.Unmarshal(raw, &info); err != nil {
		  return TronTxInfo{}, err
	   }
	  return info, nil
   }
