package wallet

import (
	"encoding/json"
	"strings"
)

// SolTxResult Solana getTransaction 最小字段（非 EVM from 回填）。
type SolTxResult struct {
	Transaction SolTxWrap  `json:"transaction"`
	Meta        *SolTxMeta `json:"meta"`
	Slot int64 `json:"slot"`
}

// SolTxWrap transaction 可为对象或 [消息,编码] 二元组；本解析只认对象形态。
type SolTxWrap struct {
	Message    SolMessage `json:"message"`
	Signatures []string   `json:"signatures"`
}

// SolMessage 账户键与指令。

type SolMessage struct {
	AccountKeys  []SolAccountKey  `json:"accountKeys"`
	Instructions []SolInstruction `json:"instructions"`
}

// SolAccountKey 账户公钥；字符串或 {pubkey,signer,writable}。

type SolAccountKey struct {
	Pubkey string `json:"pubkey"`
	raw    string
}

   func (k *SolAccountKey) UnmarshalJSON(b []byte) error {
	  var s string
	  if json.Unmarshal(b, &s) == nil {
		  k.Pubkey = s
		  k.raw = s
		   return nil
	  }
	   var obj struct {
		  Pubkey string `json:"pubkey"`
	   }
	   if err := json.Unmarshal(b, &obj); err != nil {
		  return err
	  }
	   k.Pubkey = obj.Pubkey
	  return nil
   }

// SolInstruction 顶层指令（programIdIndex + accounts）。
type SolInstruction struct {
	ProgramIDIndex int   `json:"programIdIndex"`
	Accounts       []int `json:"accounts"`
	Data string `json:"data"`
}

// SolTxMeta 费用与 token 余额变动（SPL 付款方）。
type SolTxMeta struct {
	Fee uint64          `json:"fee"`
	Err json.RawMessage `json:"err"`
	PreTokenBalances []SolTokenBalance `json:"preTokenBalances"`
	PostTokenBalances []SolTokenBalance `json:"postTokenBalances"`
}

// SolTokenBalance SPL token 账户余额快照。

type SolTokenBalance struct {
	AccountIndex int `json:"accountIndex"`
	Mint          string `json:"mint"`
	Owner         string `json:"owner"`
	UITokenAmount struct {
		Amount string `json:"amount"`
	} `json:"uiTokenAmount"`
}

// EnrichFromSOL 从 getTransaction 回填付款方（超越 EVM eth_getTransactionByHash）。
// fee payer = accountKeys[0]；SPL 入金先取余额减少的 owner；mint 可过滤。
// 解析失败/缺键返回空，不得伪造 base58 地址。
   func EnrichFromSOL(nativeFrom string, tx SolTxResult, mintFilter string) string {
	   if fa := strings.TrimSpace(nativeFrom); fa != "" {
		   return fa
	   }
	   mintFilter = strings.TrimSpace(mintFilter)
	  if from := solSPLDebitOwner(tx.Meta, mintFilter); from != "" {
		  return from
	   }
	   keys := tx.Transaction.Message.AccountKeys
	  if len(keys) > 0 {
		   if fa := strings.TrimSpace(keys[0].Pubkey); fa != "" {
			  return fa
		  }
	  }
	   return ""
  }

   func solSPLDebitOwner(meta *SolTxMeta, mintFilter string) string {
	   if meta == nil {
		  return ""
	   }
	   pre := map[string]string{} // owner|mint -> amount
	   for _, b := range meta.PreTokenBalances {
		   owner := strings.TrimSpace(b.Owner)
		  mint := strings.TrimSpace(b.Mint)
		  if owner == "" {
			   continue
		   }
		   if mintFilter != "" && !strings.EqualFold(mint, mintFilter) {
			   continue
		   }
		  pre[owner+"|"+mint] = strings.TrimSpace(b.UITokenAmount.Amount)
	   }
	  for _, b := range meta.PostTokenBalances {
		  owner := strings.TrimSpace(b.Owner)
		   mint := strings.TrimSpace(b.Mint)
		   if owner == "" {
			   continue
		   }
		   if mintFilter != "" && !strings.EqualFold(mint, mintFilter) {
			  continue
		  }
		  key := owner + "|" + mint
		   preAmt := pre[key]
		   postAmt := strings.TrimSpace(b.UITokenAmount.Amount)
		  if preAmt != "" && postAmt != "" && amountLess(postAmt, preAmt) {
			   return owner
		   }
		  delete(pre, key)
	   }
	  // pre 有、post 无：整额转出
	   for key := range pre {
		  parts := strings.SplitN(key, "|", 2)
		   if len(parts) > 0 && parts[0] != "" {
			   return parts[0]
		   }
	  }
	   return ""
  }

  func amountLess(a, b string) bool {
	   // 粗比较十进制整数字符串；非数字时按字典序失败安全返回 false
	  a = strings.TrimLeft(a, "0")
	   b = strings.TrimLeft(b, "0")
	  if a == "" {
		   a = "0"
	   }
	   if b == "" {
		   b = "0"
	  }
	   if len(a) != len(b) {
		  return len(a) < len(b)
	  }
	   return a < b
   }

// DecodeSolTxJSON 解析 getTransaction JSON（含 result 包装）。

  func DecodeSolTxJSON(raw []byte) (SolTxResult, error) {
	   var wrap struct {
		  Result json.RawMessage `json:"result"`
	  }
	   if json.Unmarshal(raw, &wrap) == nil && len(wrap.Result) > 0 && string(wrap.Result) != "null" {
		   raw = wrap.Result
	   }
	   var tx SolTxResult
	   if err := json.Unmarshal(raw, &tx); err != nil {
		  return SolTxResult{}, err
	   }
	   return tx, nil
  }

// SolToAddress 取首个非 fee-payer 账户作收款提示（可空）。

  func SolToAddress(tx SolTxResult) string {
	  keys := tx.Transaction.Message.AccountKeys
	   if len(keys) > 1 {
		   return strings.TrimSpace(keys[1].Pubkey)
	   }
	  return ""
  }
