package feeoracle

import (
	"fmt"
	"strings"
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/quote"
	"account-assets.kucoin.net/funding/internal/quotemultigate"
)

// Family 链族枚举。一个链族对应一套费率参数定义。

type Family string

const (
	// FamilyUTXO Bitcoin 及同构 UTXO 链：BTC, LTC, BCH, DOGE 等。
	// 费用 = fee_rate (sat/vByte) × estimated_vbytes / 1e8
	FamilyUTXO Family = "utxo"
	// FamilyEVM 以太坊及 EVM 兼容链：ETH, BSC, ARB, OP, BASE, AVAX, MATIC 等。
	// 费用 = (base_fee + priority_fee) × gas_limit / 1e9 (gwei → native token)
	FamilyEVM Family = "evm"
	// FamilyTRON TRON 网络：TRX, USDT-TRC20 等。
	// 费用 = (energy × energy_price_sun + bandwidth × bandwidth_price_sun) / 1e6
	FamilyTRON Family = "tron"
	// FamilySOL Solana 网络。
	// 费用 = (base_lamports + compute_units × microlamports_per_cu / 1e6) / 1e9
	FamilySOL Family = "sol"
	// FamilyGeneric 其他链，直接使用 NativeFee 字段，不做换算。
	FamilyGeneric Family = "generic"
)

// ParseFamily 将字符串解析为链族枚举；大小写不敏感。
  func ParseFamily(s string) (Family, error) {
	  switch Family(strings.ToLower(strings.TrimSpace(s))) {
	   case FamilyUTXO:
		  return FamilyUTXO, nil
	   case FamilyEVM:
		   return FamilyEVM, nil
	   case FamilyTRON:
		  return FamilyTRON, nil
	   case FamilySOL:
		   return FamilySOL, nil
	  case FamilyGeneric:
		   return FamilyGeneric, nil
	   default:
		  return "", fmt.Errorf("%w: %q", ErrChainFamily, s)
	   }
   }

// ChainFeeInput 链费率原始输入。并非所有字段都对每种链族有效；
// 调用 Normalize 时，模型按 Family 选取对应字段，忽略其余字段。
type ChainFeeInput struct {
	// Family 链族；必填。
	Family Family `json:"family"`
	// Chain 链名（ETH/BTC/TRON/SOL/BSC 等）；仅用于日志与存储，不影响计算。
	Chain string `json:"chain"`
	// --- UTXO 字段 ---
	// FeeRateSatPerVByte sat/vByte，UTXO 链的费率。
	FeeRateSatPerVByte decimal.Decimal `json:"fee_rate_sat_per_vbyte,omitempty"`
	// EstimatedVBytes 估算的交易 vByte 大小；典型值：BTC P2WPKH ≈ 141 vByte。
	EstimatedVBytes int64 `json:"estimated_vbytes,omitempty"`
	// --- EVM 字段 ---
	// BaseFeeGwei EIP-1559 base fee，单位 gwei（1 ETH = 1e9 gwei）。
	BaseFeeGwei decimal.Decimal `json:"base_fee_gwei,omitempty"`
	// PriorityFeeGwei EIP-1559 priority tip，单位 gwei。
	PriorityFeeGwei decimal.Decimal `json:"priority_fee_gwei,omitempty"`
	// GasLimit EVM 交易 gas 上限；典型值：ETH 转账 21000，ERC20 65000。
	GasLimit int64 `json:"gas_limit,omitempty"`
	// --- TRON 字段 ---
	// EnergyUnits 消耗的能量单位数（TRX 智能合约）。
	EnergyUnits int64 `json:"energy_units,omitempty"`
	// EnergyPriceSun 每单位能量的价格，单位 SUN（1 TRX = 1e6 SUN）。
	EnergyPriceSun decimal.Decimal `json:"energy_price_sun,omitempty"`
	// BandwidthBytes 消耗的带宽字节数（基本转账）。
	BandwidthBytes int64 `json:"bandwidth_bytes,omitempty"`
	// BandwidthPriceSun 每字节带宽的价格，单位 SUN。
	BandwidthPriceSun decimal.Decimal `json:"bandwidth_price_sun,omitempty"`
	// --- SOL 字段 ---
	// BaseLamports Solana 基础签名费（固定 5000 lamports/签名）。
	BaseLamports int64 `json:"base_lamports,omitempty"`
	// ComputeUnits 请求的计算单元数；基本 SOL 转账约 300 CU。
	ComputeUnits int64 `json:"compute_units,omitempty"`
	// MicroLamportsPerCU 每计算单元的先级费用，单位 micro-lamports（1 lamport = 1e6 micro-lamports）。
	MicroLamportsPerCU int64 `json:"micro_lamports_per_cu,omitempty"`
	// --- Generic 字段 ---
	// NativeFee 原生资产数量，直接用于 FamilyGeneric；其他族忽略此字段。
	NativeFee decimal.Decimal `json:"native_fee,omitempty"`
	// NativeAsset 原生资产代码（如 "BTC"/"ETH"/"TRX"/"SOL"）；
	// FamilyGeneric 时必填，其余族由模型推断。
	NativeAsset string `json:"native_asset,omitempty"`
}

// NormalizedFee 规范化后的链手续费。
type NormalizedFee struct {
	// Family 来源链族。
	Family Family `json:"family"`
	// Chain 链名。
	Chain string `json:"chain"`
	// NativeAsset 原生资产代码。
	NativeAsset string `json:"native_asset"`
	// NativeAmount 原生资产计价的手续费数量。
	NativeAmount decimal.Decimal `json:"native_amount"`
	// RawDetail 调试用原始计算摘要；不参与业务逻辑。
	RawDetail string `json:"raw_detail"`
}

var (
	// satPerBTC BTC 链：1 BTC = 1e8 satoshis。
	satPerBTC = decimal.New(1, 8)
	// gweiPerETH EVM 链：1 ETH = 1e9 gwei。
	gweiPerETH = decimal.New(1, 9)
	// sunPerTRX Tron 链：1 TRX = 1e6 SUN。
	sunPerTRX = decimal.New(1, 6)
	// lamportsPerSOL Solana：1 SOL = 1e9 lamports。
	lamportsPerSOL = decimal.New(1, 9)
	// microPerLamport 1 lamport = 1e6 micro-lamports。
	microPerLamport = decimal.New(1, 6)
)

// Normalize 将链费率输入规范化为原生资产计价的手续费数量。
// 返回 NormalizedFee；调用方用 oracle 价格将其换算为用户报价资产。
  func Normalize(in ChainFeeInput) (NormalizedFee, error) {
	   if err := validateInput(in); err != nil {
		   return NormalizedFee{}, err
	  }
	  switch in.Family {
	   case FamilyUTXO:
		   return normalizeUTXO(in)
	   case FamilyEVM:
		   return normalizeEVM(in)
	   case FamilyTRON:
		  return normalizeTRON(in)
	  case FamilySOL:
		  return normalizeSOL(in)
	   case FamilyGeneric:
		  return normalizeGeneric(in)
	   default:
		   return NormalizedFee{}, fmt.Errorf("%w: %q", ErrChainFamily, in.Family)
	   }
   }

// validateInput 基础输入校验（非零族、非空链）。

  func validateInput(in ChainFeeInput) error {
	   if in.Family == "" {
		  return fmt.Errorf("%w: family required", ErrModel)
	  }
	   if strings.TrimSpace(in.Chain) == "" {
		   return fmt.Errorf("%w: chain required", ErrModel)
	   }
	   return nil
   }

// normalizeUTXO BTC/UTXO 链：fee = fee_rate(sat/vByte) × vbytes / 1e8。

  func normalizeUTXO(in ChainFeeInput) (NormalizedFee, error) {
	   if in.FeeRateSatPerVByte.IsNegative() {
		   return NormalizedFee{}, fmt.Errorf("%w: utxo fee_rate must be non-negative", ErrModel)
	   }
	   if in.EstimatedVBytes <= 0 {
		   return NormalizedFee{}, fmt.Errorf("%w: utxo estimated_vbytes must be positive", ErrModel)
	  }
	  vbytes := decimal.NewFromInt(in.EstimatedVBytes)
	  totalSat := in.FeeRateSatPerVByte.Mul(vbytes)
	  nativeBTC := totalSat.Div(satPerBTC)
	   native := strings.ToUpper(strings.TrimSpace(in.NativeAsset))
	   if native == "" {
		  native = "BTC"
	   }
	  return NormalizedFee{
		  Family:      FamilyUTXO,
		   Chain:       in.Chain,
		   NativeAsset: native,
		   NativeAmount: nativeBTC,
		  RawDetail:    fmt.Sprintf("utxo: %s sat/vB × %d vB = %s %s", in.FeeRateSatPerVByte.StringFixed(8), in.EstimatedVBytes, nativeBTC.StringFixed(8), native),
	   }, nil
  }

// normalizeEVM EVM 链：fee = (base_fee + priority_fee) × gas_limit gwei / 1e9。
  func normalizeEVM(in ChainFeeInput) (NormalizedFee, error) {
	   if in.BaseFeeGwei.IsNegative() {
		   return NormalizedFee{}, fmt.Errorf("%w: evm base_fee must be non-negative", ErrModel)
	   }
	  if in.PriorityFeeGwei.IsNegative() {
		   return NormalizedFee{}, fmt.Errorf("%w: evm priority_fee must be non-negative", ErrModel)
	  }
	   if in.GasLimit <= 0 {
		  return NormalizedFee{}, fmt.Errorf("%w: evm gas_limit must be positive", ErrModel)
	  }
	   totalGwei := in.BaseFeeGwei.Add(in.PriorityFeeGwei)
	  gasLimit := decimal.NewFromInt(in.GasLimit)
	  feeGwei := totalGwei.Mul(gasLimit)
	   nativeETH := feeGwei.Div(gweiPerETH)
	  native := strings.ToUpper(strings.TrimSpace(in.NativeAsset))
	   if native == "" {
		   native = "ETH"
	   }
	  return NormalizedFee{
		  Family: FamilyEVM,
		   Chain:       in.Chain,
		   NativeAsset: native,
		   NativeAmount: nativeETH,
		  RawDetail: fmt.Sprintf("evm: (%s+%s) gwei × %d gas / 1e9 = %s %s", in.BaseFeeGwei.StringFixed(4), in.PriorityFeeGwei.StringFixed(4), in.GasLimit, nativeETH.StringFixed(10), native),
	  }, nil
   }

// normalizeTRON Tron：fee = (energy × e_price + bandwidth × bw_price) SUN / 1e6。
   func normalizeTRON(in ChainFeeInput) (NormalizedFee, error) {
	  if in.EnergyUnits < 0 {
		   return NormalizedFee{}, fmt.Errorf("%w: tron energy_units must be non-negative", ErrModel)
	  }
	  if in.EnergyPriceSun.IsNegative() {
		  return NormalizedFee{}, fmt.Errorf("%w: tron energy_price_sun must be non-negative", ErrModel)
	   }
	  if in.BandwidthBytes < 0 {
		   return NormalizedFee{}, fmt.Errorf("%w: tron bandwidth_bytes must be non-negative", ErrModel)
	  }
	   if in.BandwidthPriceSun.IsNegative() {
		   return NormalizedFee{}, fmt.Errorf("%w: tron bandwidth_price_sun must be non-negative", ErrModel)
	  }
	  energySUN := decimal.NewFromInt(in.EnergyUnits).Mul(in.EnergyPriceSun)
	   bwSUN := decimal.NewFromInt(in.BandwidthBytes).Mul(in.BandwidthPriceSun)
	   totalSUN := energySUN.Add(bwSUN)
	  nativeTRX := totalSUN.Div(sunPerTRX)
	   native := strings.ToUpper(strings.TrimSpace(in.NativeAsset))
	   if native == "" {
		  native = "TRX"
	  }
	   return NormalizedFee{
		   Family: FamilyTRON,
		  Chain:        in.Chain,
		   NativeAsset:  native,
		   NativeAmount: nativeTRX,
		  RawDetail: fmt.Sprintf("tron: (%d energy × %s + %d bw × %s) SUN / 1e6 = %s %s",
			  in.EnergyUnits, in.EnergyPriceSun.StringFixed(2), in.BandwidthBytes, in.BandwidthPriceSun.StringFixed(4), nativeTRX.StringFixed(8), native),
	   }, nil
  }

// normalizeSOL Solana：fee = (base_lamports + compute_units × micro_per_cu / 1e6) / 1e9。
   func normalizeSOL(in ChainFeeInput) (NormalizedFee, error) {
	  if in.BaseLamports < 0 {
		   return NormalizedFee{}, fmt.Errorf("%w: sol base_lamports must be non-negative", ErrModel)
	   }
	   if in.ComputeUnits < 0 {
		  return NormalizedFee{}, fmt.Errorf("%w: sol compute_units must be non-negative", ErrModel)
	  }
	  if in.MicroLamportsPerCU < 0 {
		  return NormalizedFee{}, fmt.Errorf("%w: sol micro_lamports_per_cu must be non-negative", ErrModel)
	  }
	  base := decimal.NewFromInt(in.BaseLamports)
	  cu := decimal.NewFromInt(in.ComputeUnits)
	   microPerCU := decimal.NewFromInt(in.MicroLamportsPerCU)
	  // priority fee in lamports = compute_units × micro_lamports_per_cu / 1e6
	  priorityLamports := cu.Mul(microPerCU).Div(microPerLamport)
	   totalLamports := base.Add(priorityLamports)
	  nativeSOL := totalLamports.Div(lamportsPerSOL)
	   native := strings.ToUpper(strings.TrimSpace(in.NativeAsset))
	  if native == "" {
		  native = "SOL"
	   }
	  return NormalizedFee{
		  Family: FamilySOL,
		   Chain:        in.Chain,
		  NativeAsset:  native,
		   NativeAmount: nativeSOL,
		  RawDetail: fmt.Sprintf("sol: (%d base + %d CU × %d μL/CU / 1e6) / 1e9 = %s %s",
			   in.BaseLamports,
			   in.ComputeUnits, in.MicroLamportsPerCU, nativeSOL.StringFixed(12), native),
	   }, nil
   }

// normalizeGeneric 直接使用 NativeFee 字段，不做换算。

   func normalizeGeneric(in ChainFeeInput) (NormalizedFee, error) {
	  if in.NativeFee.IsNegative() {
		   return NormalizedFee{}, fmt.Errorf("%w: generic native_fee must be non-negative", ErrModel)
	  }
	   native := strings.ToUpper(strings.TrimSpace(in.NativeAsset))
	  if native == "" {
		   return NormalizedFee{}, fmt.Errorf("%w: generic family requires native_asset", ErrModel)
	  }
	  return NormalizedFee{
		   Family:      FamilyGeneric,
		  Chain:       in.Chain,
		   NativeAsset: native,
		  NativeAmount: in.NativeFee,
		  RawDetail:    fmt.Sprintf("generic: %s %s (passthrough)", in.NativeFee.StringFixed(18), native),
	  }, nil
   }

// DefaultInputs 返回各链族的典型默认参数（用于回落和离线）。
// 不得用于报价；路径必须从 oracle 取实时 tick。
var DefaultInputs = map[Family]ChainFeeInput{
	FamilyUTXO: {
		Family: FamilyUTXO,
		Chain:  "BTC",
		NativeAsset:        "BTC",
		FeeRateSatPerVByte: decimal.NewFromInt(15),
		EstimatedVBytes:    141,
	},
	FamilyEVM: {
		Family: FamilyEVM,
		Chain:           "ETH",
		NativeAsset:     "ETH",
		BaseFeeGwei:     decimal.NewFromInt(20),
		PriorityFeeGwei: decimal.NewFromInt(2),
		GasLimit:        65000,
	},
	FamilyTRON: {
		Family: FamilyTRON,
		Chain:  "TRON",
		NativeAsset: "TRX",
		EnergyUnits:    65000,
		EnergyPriceSun: decimal.NewFromInt(420),
		BandwidthBytes: 345,
		BandwidthPriceSun: decimal.NewFromInt(1000),
	},
	FamilySOL: {
		Family: FamilySOL,
		Chain:        "SOL",
		NativeAsset:  "SOL",
		BaseLamports: 5000,
		ComputeUnits: 200000,
		MicroLamportsPerCU: 1000,
	},
}

// FeeQuote 单资产提现费快照。空/过期不得当作市价。
type FeeQuote struct {
	Asset string `json:"asset"`
	Chain string          `json:"chain,omitempty"`
	BPS   int             `json:"bps"`
	Flat  decimal.Decimal `json:"flat"`
	AsOf  time.Time       `json:"as_of"`
	TTL    time.Duration `json:"ttl"`
	Source string        `json:"source"`
	RawFlat string        `json:"raw_flat,omitempty"`
	Levels  []quote.Level `json:"levels,omitempty"`
}

   func (f FeeQuote) Key() string {
	   a := quote.AssetKey(f.Asset)
	  c := quote.AssetKey(f.Chain)
	   if c == "" {
		  return a
	  }
	  return a + "@" + c
  }

   func (f FeeQuote) AsQuote() quote.Quote {
	   q := quote.Quote{
		   Pair: f.Key(),
		   Asset:  quote.AssetKey(f.Asset),
		   FeeBPS: f.BPS,
		   FeeAmount: f.Flat,
		   AsOf:      f.AsOf,
		   TTL:       f.TTL,
		  Source: quote.NormalizeSource(f.Source),
		   RawRate: f.RawFlat,
		   Levels:  f.Levels,
	  }
	   if f.Flat.IsPositive() {
		   q.Rate = f.Flat
	   }
	   return q
  }

// ConfirmReq 提现费确认。失败不得写 ledger。
type ConfirmReq struct {
	UserID string
	Asset   string
	Chain   string
	Gross   decimal.Decimal
	IdemKey string
	Now time.Time
	Multi quotemultigate.Bundle
}

// ConfirmResult 成功确认结果。

type ConfirmResult struct {
	UserID string `json:"user_id"`
	Asset string          `json:"asset"`
	Fee   decimal.Decimal `json:"fee"`
	Net    decimal.Decimal `json:"net"`
	Source string          `json:"source"`
	AsOf time.Time `json:"as_of"`
	IdemKey string `json:"idem_key"`
	Applied bool   `json:"applied"`
}
