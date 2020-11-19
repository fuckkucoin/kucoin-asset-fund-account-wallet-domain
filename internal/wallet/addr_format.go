package wallet

  import "strings"

   // AddrFormatSpec 单链地址校验 UX 字段（充提页提示）。
   type AddrFormatSpec struct {
   	Chain string `json:"chain"`
   	Family string `json:"family"` // EVM / UTXO / SOL / TRON / FamMEMO
   	Example string `json:"example,omitempty"`
   	Prefixes []string `json:"prefixes,omitempty"`
   	MinLen int `json:"minLen"`
   	MaxLen int `json:"maxLen"`
   	Charset     string `json:"charset,omitempty"` // 人类可读描述
   	NeedMemo    bool   `json:"needMemo"`
   	TagName     string `json:"tagName,omitempty"`
   	MemoExample string `json:"memoExample,omitempty"`
   	MemoHint string `json:"memoHint,omitempty"`
   }

   // FormatSpec 返回链地址的形态约束：前缀、长度、字符集、是否必须带备注。
   // 不附示例地址。充提地址只接受调用方传入并校验通过的值。
   func FormatSpec(chain string) AddrFormatSpec {
   	chain = strings.ToUpper(strings.TrimSpace(chain))
   	spec := AddrFormatSpec{Chain: chain, MinLen: 1, MaxLen: 128}
   	switch {
   		case chain == "EVM" || IsEVM(chain):
   		spec.Family = "EVM"
   		spec.Prefixes = []string{"0x"}
   		spec.MinLen, spec.MaxLen = 42, 42
   		spec.Charset = "hex"
   		case chain == "BTC":
   		spec.Family = "UTXO"
   		spec.Prefixes = []string{"bc1", "1", "3"}
   		spec.MinLen, spec.MaxLen = 26, 90
   		spec.Charset = "bech32|base58"
   		case chain == "SOL":
   		spec.Family = "SOL"
   		spec.MinLen, spec.MaxLen = 32, 44
   		spec.Charset = "base58"
   		case chain == "TRON":
   		spec.Family = "TRON"
   		spec.Prefixes = []string{"T"}
   		spec.MinLen, spec.MaxLen = 34, 34
   		spec.Charset = "base58check"
   		case chain == "TON":
   		spec.Family = "FamMEMO"
   		spec.Prefixes = []string{"EQ", "UQ"}
   		spec.MinLen, spec.MaxLen = 48, 48
   		spec.Charset = "base64url"
   		spec.NeedMemo = true
   		spec.TagName = "comment"
   		spec.MemoHint = "漏填 comment 不入账"
   		case chain == "XLM":
   		spec.Family = "FamMEMO"
   		spec.Prefixes = []string{"G", "M"}
   		spec.MinLen, spec.MaxLen = 56, 56
   		spec.Charset = "base32"
   		spec.NeedMemo = true
   		spec.TagName = "memo"
   		spec.MemoHint = "漏填 memo 不入账"
   		case chain == "XRP":
   		spec.Family = "FamMEMO"
   		spec.Prefixes = []string{"r", "X"}
   		spec.MinLen, spec.MaxLen = 25, 64
   		spec.Charset = "base58|x-address"
   		spec.NeedMemo = true
   		spec.TagName = "destination_tag"
   		spec.MemoHint = "漏填 destination_tag 不入账；须为 uint32 十进制"
   		case chain == "ATOM":
   		spec.Family = "FamMEMO"
   		spec.Prefixes = []string{"cosmos1"}
   		spec.MinLen, spec.MaxLen = 39, 90
   		spec.Charset = "bech32"
   		spec.NeedMemo = true
   		spec.TagName = "memo"
   		spec.MemoHint = "漏填 memo 不入账"
   		case chain == "EOS":
   		spec.Family = "FamMEMO"
   		spec.MinLen, spec.MaxLen = 1, 12
   		spec.Charset = "a-z1-5."
   		spec.NeedMemo = true
   		spec.TagName = "memo"
   		spec.MemoHint = "漏填 memo 不入账"
   		default:
   		spec.Family = "UNKNOWN"
   	}
   	return spec
   }

   // FormatSpecs 已知资金链形态清单（公开选网/校验目录）。

   func FormatSpecs() []AddrFormatSpec {
   	chains := []string{"ETH", "BTC", "TRON", "SOL", "TON", "XLM", "XRP", "ATOM", "EOS", "BSC", "MATIC", "BASE", "ARB", "OP", "AVAX"}
   	out := make([]AddrFormatSpec, 0, len(chains))
   	for _, ch := range chains {
   		out = append(out, FormatSpec(ch))
   	}
   	return out
   }
