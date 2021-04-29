package feeeng

import (
	"fmt"
	"strings"
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/pkg/money"
)

// ChainFamily 链族标识符（与 feeoracle.Family 字符串保持一致，
// 在此重新声明以避免 import cycle；调用方可直接核对字符串）。

type ChainFamily string

const (
	ChainFamilyUTXO    ChainFamily = "utxo"
	ChainFamilyEVM     ChainFamily = "evm"
	ChainFamilyTRON    ChainFamily = "tron"
	ChainFamilySOL     ChainFamily = "sol"
	ChainFamilyGeneric ChainFamily = "generic"
)

// ChainFeeParams 链费率参数（由 feeoracle 层计算后传入，不在 feeeng 层重算）。

type ChainFeeParams struct {
	// Chain 链名（ETH/BTC/TRON/SOL 等）。
	Chain string `json:"chain"`
	// Family 链族。
	Family ChainFamily `json:"family"`
	// NativeAsset 原生资产代码（BTC/ETH/TRX/SOL）。
	NativeAsset string `json:"native_asset"`
	// NativeFee 原生资产计价的估算手续费（由 feeoracle 规范化后给入）。
	// 单位与 Asset 对应；若 NativeAsset == Asset，则此即用户需支付的费用。
	NativeFee decimal.Decimal `json:"native_fee"`
	// QuotedFee 已经换算成用户 Asset 计价的估算费用。
	// 若与 NativeAsset 相同则 QuotedFee == NativeFee。
	QuotedFee decimal.Decimal `json:"quoted_fee"`
	// TickID oracle tick 标识，用于追踪费率来源（可为空）。
	TickID string `json:"tick_id,omitempty"`
	// TickedAt oracle tick 时间戳。
	TickedAt time.Time `json:"ticked_at,omitempty"`
}

// ChainFeeQuote 多链增强报价，是 Quote 的超集。
type ChainFeeQuote struct {
	// 内嵌基础报价。
	Quote `json:"base"`
	// Params 链费率参数快照。
	Params ChainFeeParams `json:"chain_params"`
	// ScheduleFallback 若 true，表示 oracle 不可用，本次使用了静态档位兜底。
	ScheduleFallback bool `json:"schedule_fallback"`
	// OracleError oracle 报错原因（仅 ScheduleFallback=true 时非空）。
	OracleError string `json:"oracle_error,omitempty"`
}

// ChainQuoteReq 多链报价请求。
type ChainQuoteReq struct {
	// Kind 费用类型（withdraw/deposit/transfer 等）。
	Kind string `json:"kind"`
	// Asset 用户资产代码。
	Asset string `json:"asset"`
	// Gross 用户出金总额（用户资产计价）。
	Gross string `json:"gross"`
	// Params 链费率参数（来自 feeoracle）。
	// 若 Params.QuotedFee 为零，退化到纯静态档位报价（ScheduleFallback=true）。
	Params ChainFeeParams `json:"chain_params"`
}

// QuoteChain 生成多链增强报价。
// 报价逻辑：
// 1. 若 Params.QuotedFee > 0，以 oracle 报价为费用基准，再用 Schedule 的 min/max 夹值；
// 2. 若 Params.QuotedFee == 0（oracle 未提供），退化到纯静态档位报价；
// 3. 无论哪种路径，fee+net 必须守恒 == gross；
// 4. net <= 0 时拒绝（最终净额必须大于零）。

   func (b *Book) QuoteChain(req ChainQuoteReq) (ChainFeeQuote, error) {
	  if b == nil {
		   return ChainFeeQuote{}, ErrUnavailable
	  }
	  kind, err := ParseKind(req.Kind)
	  if err != nil {
		  return ChainFeeQuote{}, err
	  }
	  asset := strings.ToUpper(strings.TrimSpace(req.Asset))
	   if asset == "" {
		  return ChainFeeQuote{}, ErrUnknownAsset
	   }
	  gross,
		  err := money.Parse(req.Gross)
	  if err != nil {
		   return ChainFeeQuote{}, err
	  }
	   sch, err := b.Get(kind, asset)
	   if err != nil {
		  return ChainFeeQuote{}, err
	  }
	   // oracle 提供了报价费用：以 oracle 为主，Schedule min/max 夹值。
	  if req.Params.QuotedFee.IsPositive() {
		   return b.quoteFromOracle(kind, asset, gross, sch, req.Params)
	  }
	  // oracle 未提供：纯静态档位。
	   q, err := b.Quote(kind, asset, req.Gross)
	   if err != nil {
		   return ChainFeeQuote{}, err
	   }
	   return ChainFeeQuote{
		   Quote:  q,
		   Params: req.Params,
		   ScheduleFallback: true,
		  OracleError:      "oracle quoted_fee is zero; using schedule fallback",
	   }, nil
   }

// quoteFromOracle oracle 报价 + Schedule min/max 夹值路径。
   func (b *Book) quoteFromOracle(
	  kind Kind, asset string, gross money.Amount, sch Schedule, params ChainFeeParams,
  ) (ChainFeeQuote, error) {
	  // 以 QuotedFee 为初始估算费。
	  rawFee, err := money.ParseBook(params.QuotedFee.String())
	  if err != nil {
		  return ChainFeeQuote{}, fmt.Errorf("%w: oracle quoted_fee: %v", ErrQuote, err)
	  }
	   fee := rawFee
	   // 夹 min。
	  if sch.MinFee != "" {
		   if min, merr := money.ParseBook(sch.MinFee); merr == nil && fee.Cmp(min) < 0 && min.Cmp(gross) < 0 {
			   fee = min
		   }
	   }
	   // 夹 max。
	  if sch.MaxFee != "" {
		   if max, merr := money.ParseBook(sch.MaxFee); merr == nil && max.IsPositive() && fee.Cmp(max) > 0 {
			  fee = max
		   }
	  }
	  // 费用不得超过 gross。
	   if fee.Cmp(gross) >= 0 {
		  fee = gross
	  }
	  net, err := gross.Sub(fee)
	   if err != nil {
		   return ChainFeeQuote{}, fmt.Errorf("%w: gross - fee underflow", ErrQuote)
	  }
	  if !net.IsPositive() {
		  // 净额为零时拒绝（出金后用户到账为零）。
		  return ChainFeeQuote{}, fmt.Errorf("%w: net amount is zero after fee", ErrQuote)
	   }
	  // 守恒校验：fee + net == gross。
	  sum, err := fee.Add(net)
	   if err != nil {
		  return ChainFeeQuote{}, fmt.Errorf("%w: fee+net overflow", ErrQuote)
	   }
	  if err := money.Conserve(sum, gross); err != nil {
		  return ChainFeeQuote{}, fmt.Errorf("%w: fee+net != gross: %v", ErrQuote, err)
	   }
	   q := Quote{
		  Kind:  kind,
		  Asset: asset,
		   Gross: gross.String(),
		  Fee:   fee.String(),
		   Net:   net.String(),
		  BPS: sch.BPS,
		   Schedule: sch.ID,
		   Version:  sch.Version,
	  }
	  return ChainFeeQuote{
		  Quote: q,
		  Params:           params,
		  ScheduleFallback: false,
	  }, nil
  }

// ApplyChain 生成多链增强报价的账本凭证。
// 凭证语义与 Apply 相同（fee+net 守恒），但附加链族信息。
   func (b *Book) ApplyChain(req ChainQuoteReq) (Applied, ChainFeeQuote, error) {
	  cq, err := b.QuoteChain(req)
	   if err != nil {
		  return Applied{}, ChainFeeQuote{}, err
	  }
	  app,
		   err := b.Apply(cq.Quote.Kind, cq.Quote.Asset, cq.Quote.Gross)
	   if err != nil {
		  return Applied{}, ChainFeeQuote{}, err
	  }
	  return app, cq, nil
   }

// BPSFromOracleFee 按 oracle 报价费用反推等效 BPS（用于档位核对）。
// gross 为零时返回 0。结果仅供参考，不用于实际报价计算。

  func BPSFromOracleFee(gross, fee string) (int, error) {
	  g, err := money.Parse(gross)
	   if err != nil {
		  return 0, err
	   }
	  f, err := money.ParseBook(fee)
	   if err != nil {
		  return 0, err
	   }
	   if g.IsZero() {
		   return 0, nil
	  }
	   bps := f.Dec().Div(g.Dec()).Mul(decimal.NewFromInt(10000))
	   return int(bps.IntPart()), nil
  }
