package wallet

import (
	"encoding/hex"
	"encoding/json"
	"math/big"
	"strings"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/config"
)

// ERC20 Transfer(address,address,uint256) topic0

const erc20TransferTopic0 = "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"

// ReceiptLog ETH 回执日志条目（token Transfer 解析用）。
type ReceiptLog struct {
	Address string `json:"address"`
	Topics []string `json:"topics"`
	Data string `json:"data"`
}

// EthReceipt eth_getTransactionReceipt 最小字段。

type EthReceipt struct {
	From string `json:"from"`
	To   string `json:"to"`
	Status string `json:"status"`
	Logs []ReceiptLog `json:"logs"`
}

// EnrichFromReceipt 从回执与 Transfer 日志回填付款方（超越原生 tx.from）。
// 原生 from 先；空时取首个匹配合约的 ERC20 Transfer 的 from topic；不得伪造。
// 多 Transfer 日志时按 contract 过滤；无匹配返回空串。
  func EnrichFromReceipt(nativeFrom string, receipt EthReceipt, tokenContract string) string {
	   if fa := strings.TrimSpace(nativeFrom); fa != "" {
		   return fa
	  }
	   if fa := strings.TrimSpace(receipt.From); fa != "" {
		  return fa
	   }
	   want := strings.ToLower(strings.TrimSpace(tokenContract))
	   for _, lg := range receipt.Logs {
		   from, _, ok := ParseERC20Transfer(lg)
		   if !ok {
			   continue
		  }
		  if want != "" && strings.ToLower(strings.TrimSpace(lg.Address)) != want {
			  continue
		   }
		   if from != "" {
			   return from
		  }
	   }
	   // 无合约过滤时取首个 Transfer from
	  for _, lg := range receipt.Logs {
		  from,
			  _, ok := ParseERC20Transfer(lg)
		   if ok && from != "" {
			   return from
		   }
	   }
	   return ""
  }

// ParseERC20Transfer 解析 ERC20 Transfer 日志的 from/to。
  func ParseERC20Transfer(lg ReceiptLog) (from, to string, ok bool) {
	   if len(lg.Topics) < 3 {
		   return "", "", false
	   }
	  topic0 := strings.ToLower(strings.TrimSpace(lg.Topics[0]))
	  if topic0 != erc20TransferTopic0 {
		  return "", "", false
	  }
	   from = topicAddress(lg.Topics[1])
	  to = topicAddress(lg.Topics[2])
	   if from == "" && to == "" {
		   return "", "", false
	  }
	  return from, to, true
   }

   func topicAddress(topic string) string {
	   topic = strings.TrimSpace(topic)
	   topic = strings.TrimPrefix(strings.ToLower(topic), "0x")
	   if len(topic) < 40 {
		  return ""
	   }
	  return "0x" + topic[len(topic)-40:]
   }

// ParseTransferAmount 从 Transfer data 解析数量（按 decimals）。

  func ParseTransferAmount(data string, decimals int32) (decimal.Decimal, error) {
	   data = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(data)), "0x")
	  if data == "" {
		  return decimal.Zero, nil
	   }
	   raw, err := hex.DecodeString(data)
	   if err != nil {
		   return decimal.Zero, err
	   }
	  n := new(big.Int).SetBytes(raw)
	  if decimals <= 0 {
		   decimals = 18
	  }
	   den := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	   return decimal.NewFromBigInt(n, 0).Div(decimal.NewFromBigInt(den, 0)), nil
  }

// MatchBookedTransfer 在回执日志里找已登记合约的 ERC20 Transfer。
// 登记来自 aa_token_book。对上则返回币种、按精度换算后的数量和收款地址。
func MatchBookedTransfer(logs []ReceiptLog, tokens map[string]config.TokenSpec) (asset string, amount decimal.Decimal, to string, ok bool) {
	if len(logs) == 0 || len(tokens) == 0 {
		return "", decimal.Zero, "", false
	}
	byContract := make(map[string]struct {
		asset string
		dec   int
	}, len(tokens))
	for asset, spec := range tokens {
		contract := strings.ToLower(strings.TrimSpace(spec.Contract))
		if contract == "" {
			continue
		}
		byContract[contract] = struct {
			asset string
			dec   int
		}{asset: asset, dec: spec.Decimals}
	}
	for _, lg := range logs {
		hit, found := byContract[strings.ToLower(strings.TrimSpace(lg.Address))]
		if !found {
			continue
		}
		_, recv, parsed := ParseERC20Transfer(lg)
		if !parsed {
			continue
		}
		dec := hit.dec
		if dec <= 0 {
			dec = 18
		}
		amt, err := ParseTransferAmount(lg.Data, int32(dec))
		if err != nil || !amt.IsPositive() {
			continue
		}
		return hit.asset, amt, recv, true
	}
	return "", decimal.Zero, "", false
}

// DecodeEthReceiptJSON 解析回执 JSON。
   func DecodeEthReceiptJSON(raw []byte) (EthReceipt, error) {
	   var rec EthReceipt
	   if err := json.Unmarshal(raw, &rec); err != nil {
		  return EthReceipt{}, err
	  }
	   return rec, nil
  }
