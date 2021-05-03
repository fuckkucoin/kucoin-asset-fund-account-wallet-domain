package feeoracle

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/persist"
)

// CongestionLevel 拥堵等级枚举。
type CongestionLevel string

const (
	// CongestionNormal 正常：fee ≤ median × NormalMult
	CongestionNormal CongestionLevel = "normal"
	// CongestionModerate 中度：fee ≤ median × ModerateMult
	CongestionModerate CongestionLevel = "moderate"
	// CongestionHigh 高度：fee ≤ median × HighMult
	CongestionHigh CongestionLevel = "high"
	// CongestionExtreme 极端：fee > median × HighMult
	CongestionExtreme CongestionLevel = "extreme"
)

// CongestionTier 拥堵档位与额外溢价倍数。

type CongestionTier struct {
	// Level 档位标识。
	Level CongestionLevel `json:"level"`
	// MultThreshold 触发此档位的倍数（相对于链历史中位数 fee）。
	// fee / median_fee ≥ MultThreshold → 进入此档位。
	MultThreshold decimal.Decimal `json:"mult_threshold"`
	// SurchargeBPS 额外收取的溢价，单位 BPS（叠加在 feeeng.Schedule BPS 之上）。
	// 溢价只加到 oracle 报价部分，守恒仍对 gross 负责。
	SurchargeBPS int `json:"surcharge_bps"`
}

// ChainFeeSpec 单链费率规格表。
type ChainFeeSpec struct {
	// ID 全局唯一标识。
	ID string `json:"id"`
	// Chain 链名（ETH/BTC/TRON/SOL 等）。
	Chain string `json:"chain"`
	// Family 链族。
	Family Family `json:"family"`
	// --- 默认估算量（无 oracle 时的 fallback 基准）---
	// DefaultFeeRateSatPerVByte UTXO 默认费率（sat/vByte）；FamilyUTXO 有效。
	DefaultFeeRateSatPerVByte decimal.Decimal `json:"default_fee_rate_sat_per_vbyte,omitempty"`
	// DefaultEstimatedVBytes UTXO 默认交易体积估算。
	DefaultEstimatedVBytes int64 `json:"default_estimated_vbytes,omitempty"`
	// DefaultBaseFeeGwei EVM 默认 base fee（gwei）。
	DefaultBaseFeeGwei decimal.Decimal `json:"default_base_fee_gwei,omitempty"`
	// DefaultPriorityFeeGwei EVM 默认 priority tip（gwei）。
	DefaultPriorityFeeGwei decimal.Decimal `json:"default_priority_fee_gwei,omitempty"`
	// DefaultGasLimit EVM 默认 gas limit。
	DefaultGasLimit int64 `json:"default_gas_limit,omitempty"`
	// DefaultEnergyUnits TRON 默认 energy 单位数。
	DefaultEnergyUnits int64 `json:"default_energy_units,omitempty"`
	// DefaultEnergyPriceSun TRON 默认 energy 单价（SUN）。
	DefaultEnergyPriceSun decimal.Decimal `json:"default_energy_price_sun,omitempty"`
	// DefaultBandwidthBytes TRON 默认带宽字节数。
	DefaultBandwidthBytes int64 `json:"default_bandwidth_bytes,omitempty"`
	// DefaultBandwidthPriceSun TRON 默认带宽单价（SUN）。
	DefaultBandwidthPriceSun decimal.Decimal `json:"default_bandwidth_price_sun,omitempty"`
	// DefaultBaseLamports SOL 默认基础 lamport 数。
	DefaultBaseLamports int64 `json:"default_base_lamports,omitempty"`
	// DefaultComputeUnits SOL 默认计算单元数。
	DefaultComputeUnits int64 `json:"default_compute_units,omitempty"`
	// DefaultMicroLamportsPerCU SOL 默认先费（micro-lamports/CU）。
	DefaultMicroLamportsPerCU int64 `json:"default_micro_lamports_per_cu,omitempty"`
	// --- 拥堵溢价 ---
	// CongestionTiers 拥堵档位表，按 MultThreshold 升序排列。
	// 为空表示不收拥堵溢价。
	CongestionTiers []CongestionTier `json:"congestion_tiers,omitempty"`
	// MedianFeeNative 历史中位数原生费用（供拥堵档位计算基准）；由 oracle pump 周期更新。
	MedianFeeNative decimal.Decimal `json:"median_fee_native,omitempty"`
	// --- 费用上下限（用户资产计价）---
	// MinFeeUser oracle 报价低于此值时用保底（用户资产计价）。
	MinFeeUser string `json:"min_fee_user,omitempty"`
	// MaxFeeUser oracle 报价高于此值时封顶（防毛刺）。
	MaxFeeUser string `json:"max_fee_user,omitempty"`
	// --- 状态 ---
	// Active 是否启用；false 时 oracle 不为该链报价（需 fallback 到 Schedule）。
	Active bool `json:"active"`
	// CreatedAt 创建时间。
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 最后更新时间。
	UpdatedAt time.Time `json:"updated_at"`
}

// ToFeeInput 把 ChainFeeSpec 的默认估算量组装成 ChainFeeInput（oracle 不可用时的 fallback 基准）。

   func (s ChainFeeSpec) ToFeeInput() ChainFeeInput {
	  return ChainFeeInput{
		   Family: s.Family,
		  Chain:              s.Chain,
		  FeeRateSatPerVByte: s.DefaultFeeRateSatPerVByte,
		  EstimatedVBytes:    s.DefaultEstimatedVBytes,
		  BaseFeeGwei: s.DefaultBaseFeeGwei,
		   PriorityFeeGwei: s.DefaultPriorityFeeGwei,
		  GasLimit:        s.DefaultGasLimit,
		  EnergyUnits:    s.DefaultEnergyUnits,
		   EnergyPriceSun: s.DefaultEnergyPriceSun,
		   BandwidthBytes: s.DefaultBandwidthBytes,
		  BandwidthPriceSun: s.DefaultBandwidthPriceSun,
		   BaseLamports:      s.DefaultBaseLamports,
		   ComputeUnits: s.DefaultComputeUnits,
		   MicroLamportsPerCU: s.DefaultMicroLamportsPerCU,
	   }
   }

// ClassifyCongestion 根据当前 oracle 报价费用与规格中位数，判断拥堵等级。
// 若 CongestionTiers 为空或 MedianFeeNative 为零，直接返回 CongestionNormal。
  func (s ChainFeeSpec) ClassifyCongestion(currentNativeFee decimal.Decimal) CongestionLevel {
	   if len(s.CongestionTiers) == 0 || s.MedianFeeNative.IsZero() || currentNativeFee.IsZero() {
		  return CongestionNormal
	   }
	  ratio := currentNativeFee.Div(s.MedianFeeNative)
	  // 按 MultThreshold 升序排列，找到最高命中档位。
	  sorted := append([]CongestionTier(nil),
		   s.CongestionTiers...)
	   sort.Slice(sorted, func(i, j int) bool {
		   return sorted[i].MultThreshold.Cmp(sorted[j].MultThreshold) < 0
	  })
	  level := CongestionNormal
	   for _, t := range sorted {
		   if ratio.Cmp(t.MultThreshold) >= 0 {
			   level = t.Level
		   }
	   }
	  return level
  }

// CongestionSurchargeBPS 返回当前拥堵等级对应的溢价 BPS；若未命中任何档位返回 0。
   func (s ChainFeeSpec) CongestionSurchargeBPS(currentNativeFee decimal.Decimal) int {
	  level := s.ClassifyCongestion(currentNativeFee)
	   if level == CongestionNormal {
		   return 0
	  }
	   for _, t := range s.CongestionTiers {
		  if t.Level == level {
			   return t.SurchargeBPS
		   }
	   }
	   return 0
   }

// DefaultChainFeeSpecs 内置的各链默认规格（/冷启动）。

var DefaultChainFeeSpecs = []ChainFeeSpec{
	{
		ID: "spec-btc",
		Chain:  "BTC",
		Family: FamilyUTXO,
		Active:                    true,
		DefaultFeeRateSatPerVByte: decimal.NewFromInt(15),
		DefaultEstimatedVBytes:    141,
		MinFeeUser:                "0.0001",
		MaxFeeUser:                "0.01",
		CongestionTiers: []CongestionTier{
			{Level: CongestionModerate, MultThreshold: decimal.NewFromInt(2), SurchargeBPS: 20},
			{Level: CongestionHigh, MultThreshold: decimal.NewFromInt(5), SurchargeBPS: 50},
			{Level: CongestionExtreme, MultThreshold: decimal.NewFromInt(10), SurchargeBPS: 100},
		},
		MedianFeeNative: decimal.RequireFromString("0.00002115"),
	},
	{
		ID:     "spec-eth",
		Chain:  "ETH",
		Family: FamilyEVM,
		Active: true,
		DefaultBaseFeeGwei: decimal.NewFromInt(20),
		DefaultPriorityFeeGwei: decimal.NewFromInt(2),
		DefaultGasLimit:        65000,
		MinFeeUser:             "0.5",
		MaxFeeUser: "50",
		CongestionTiers: []CongestionTier{
			{Level: CongestionModerate, MultThreshold: decimal.NewFromInt(3), SurchargeBPS: 30},
			{Level: CongestionHigh, MultThreshold: decimal.NewFromInt(8), SurchargeBPS: 80},
			{Level: CongestionExtreme, MultThreshold: decimal.NewFromInt(20), SurchargeBPS: 150},
		},
		MedianFeeNative: decimal.RequireFromString("0.002"),
	},
	{
		ID:     "spec-tron",
		Chain:  "TRON",
		Family: FamilyTRON,
		Active: true,
		DefaultEnergyUnits:    65000,
		DefaultEnergyPriceSun: decimal.NewFromInt(420),
		DefaultBandwidthBytes: 345,
		DefaultBandwidthPriceSun: decimal.NewFromInt(1000),
		MinFeeUser: "1",
		MaxFeeUser:      "30",
		MedianFeeNative: decimal.RequireFromString("27"),
	},
	{
		ID:     "spec-sol",
		Chain:  "SOL",
		Family: FamilySOL,
		Active: true,
		DefaultBaseLamports: 5000,
		DefaultComputeUnits: 200000,
		DefaultMicroLamportsPerCU: 1000,
		MinFeeUser:                "0.0001",
		MaxFeeUser:                "1",
		MedianFeeNative: decimal.RequireFromString("0.000205"),
	},
}

// SpecBook 链费率规格表仓库，支持 SQL + 内存双模。

type SpecBook struct {
	mu sync.RWMutex
	specs map[string]ChainFeeSpec // chain → spec
	sqlDB *persist.SQLDB
	clock func() time.Time
}

// NewSpecBook 创建 SpecBook，并写入内置默认规格。

   func NewSpecBook(db *persist.SQLDB) *SpecBook {
	  b := &SpecBook{
		   specs: map[string]ChainFeeSpec{},
		  sqlDB: db,
		   clock: func() time.Time { return time.Now().UTC() },
	  }
	  for _, s := range DefaultChainFeeSpecs {
		  b.specs[strings.ToUpper(s.Chain)] = s
	   }
	  return b
   }

// Put 写入或更新一条链规格。

   func (b *SpecBook) Put(s ChainFeeSpec) error {
	   if b == nil {
		  return ErrUnavailable
	   }
	  chain := strings.ToUpper(strings.TrimSpace(s.Chain))
	   if chain == "" {
		   return fmt.Errorf("%w: chain required", ErrModel)
	  }
	  now := b.clock()
	   s.Chain = chain
	   if s.CreatedAt.IsZero() {
		  s.CreatedAt = now
	   }
	  s.UpdatedAt = now
	  b.mu.Lock()
	   b.specs[chain] = s
	  b.mu.Unlock()
	  if b.sqlOn() {
		  return b.sqlPut(context.Background(), s)
	  }
	   return nil
  }

// Get 按链名查找规格；大小写不敏感。

  func (b *SpecBook) Get(chain string) (ChainFeeSpec, bool) {
	  if b == nil {
		   return ChainFeeSpec{}, false
	  }
	   chain = strings.ToUpper(strings.TrimSpace(chain))
	   b.mu.RLock()
	   s, ok := b.specs[chain]
	   b.mu.RUnlock()
	   return s, ok
   }

// GetDefault 查找规格；若未找到返回通用默认值。
  func (b *SpecBook) GetDefault(chain string) ChainFeeSpec {
	  s, ok := b.Get(chain)
	   if ok && s.Active {
		  return s
	  }
	   return ChainFeeSpec{Chain: chain, Family: FamilyGeneric, Active: true}
  }

// UpdateMedian 更新链的历史中位费用（由 oracle pump worker 周期调用）。
  func (b *SpecBook) UpdateMedian(chain string, median decimal.Decimal) error {
	   if b == nil {
		   return ErrUnavailable
	   }
	   chain = strings.ToUpper(strings.TrimSpace(chain))
	  b.mu.Lock()
	   s, ok := b.specs[chain]
	  if !ok {
		  b.mu.Unlock()
		  return fmt.Errorf("%w: spec not found for chain %s", ErrModel, chain)
	   }
	   s.MedianFeeNative = median
	  s.UpdatedAt = b.clock()
	  b.specs[chain] = s
	   b.mu.Unlock()
	  return nil
   }

// List 返回所有规格列表。
  func (b *SpecBook) List() []ChainFeeSpec {
	  if b == nil {
		   return nil
	  }
	  b.mu.RLock()
	   defer b.mu.RUnlock()
	   out := make([]ChainFeeSpec, 0, len(b.specs))
	   for _, s := range b.specs {
		   out = append(out, s)
	  }
	   sort.Slice(out, func(i, j int) bool { return out[i].Chain < out[j].Chain })
	  return out
  }

  func (b *SpecBook) sqlOn() bool {
	  return b != nil && b.sqlDB != nil && b.sqlDB.DB != nil
  }

   func (b *SpecBook) sqlPut(ctx context.Context, s ChainFeeSpec) error {
	  q := `INSERT INTO fee_chain_specs
		(id, chain, family, default_fee_rate_sat, default_vbytes, default_base_fee_gwei,
		 default_priority_gwei, default_gas_limit, default_energy_units, default_energy_price_sun,
		 default_bandwidth_bytes, default_bw_price_sun, default_base_lamports, default_compute_units,
		 default_micro_lamports_cu, min_fee_user, max_fee_user, median_fee_native, active, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE
		  family=VALUES(family), default_fee_rate_sat=VALUES(default_fee_rate_sat),
		  default_vbytes=VALUES(default_vbytes), default_base_fee_gwei=VALUES(default_base_fee_gwei),
		  default_priority_gwei=VALUES(default_priority_gwei), default_gas_limit=VALUES(default_gas_limit),
		  default_energy_units=VALUES(default_energy_units), default_energy_price_sun=VALUES(default_energy_price_sun),
		  default_bandwidth_bytes=VALUES(default_bandwidth_bytes), default_bw_price_sun=VALUES(default_bw_price_sun),
		  default_base_lamports=VALUES(default_base_lamports), default_compute_units=VALUES(default_compute_units),
		  default_micro_lamports_cu=VALUES(default_micro_lamports_cu),
		  min_fee_user=VALUES(min_fee_user), max_fee_user=VALUES(max_fee_user),
		  median_fee_native=VALUES(median_fee_native), active=VALUES(active), updated_at=VALUES(updated_at)`
	   if b.sqlDB.Dialect == persist.Postgres {
		   q = `INSERT INTO fee_chain_specs
			(id, chain, family, default_fee_rate_sat, default_vbytes, default_base_fee_gwei,
			 default_priority_gwei, default_gas_limit, default_energy_units, default_energy_price_sun,
			 default_bandwidth_bytes, default_bw_price_sun, default_base_lamports, default_compute_units,
			 default_micro_lamports_cu, min_fee_user, max_fee_user, median_fee_native, active, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT (chain) DO UPDATE SET
			  family=EXCLUDED.family, default_fee_rate_sat=EXCLUDED.default_fee_rate_sat,
			  default_vbytes=EXCLUDED.default_vbytes, default_base_fee_gwei=EXCLUDED.default_base_fee_gwei,
			  default_priority_gwei=EXCLUDED.default_priority_gwei, default_gas_limit=EXCLUDED.default_gas_limit,
			  default_energy_units=EXCLUDED.default_energy_units, default_energy_price_sun=EXCLUDED.default_energy_price_sun,
			  default_bandwidth_bytes=EXCLUDED.default_bandwidth_bytes, default_bw_price_sun=EXCLUDED.default_bw_price_sun,
			  default_base_lamports=EXCLUDED.default_base_lamports, default_compute_units=EXCLUDED.default_compute_units,
			  default_micro_lamports_cu=EXCLUDED.default_micro_lamports_cu,
			  min_fee_user=EXCLUDED.min_fee_user, max_fee_user=EXCLUDED.max_fee_user,
			  median_fee_native=EXCLUDED.median_fee_native, active=EXCLUDED.active, updated_at=EXCLUDED.updated_at`
	  }
	   act := 0
	  if s.Active {
		   act = 1
	  }
	   _, err := b.sqlDB.Conn(ctx).ExecContext(ctx, q,
		  s.ID, s.Chain, string(s.Family),
		   s.DefaultFeeRateSatPerVByte.String(), s.DefaultEstimatedVBytes,
		  s.DefaultBaseFeeGwei.String(),
		   s.DefaultPriorityFeeGwei.String(), s.DefaultGasLimit,
		   s.DefaultEnergyUnits, s.DefaultEnergyPriceSun.String(),
		  s.DefaultBandwidthBytes, s.DefaultBandwidthPriceSun.String(),
		   s.DefaultBaseLamports,
		   s.DefaultComputeUnits, s.DefaultMicroLamportsPerCU,
		  s.MinFeeUser, s.MaxFeeUser, s.MedianFeeNative.String(),
		   act, s.CreatedAt, s.UpdatedAt,
	   )
	  return err
  }

  func (b *SpecBook) sqlLoad(ctx context.Context) error {
	   if !b.sqlOn() {
		   return nil
	   }
	  rows, err := b.sqlDB.Conn(ctx).QueryContext(ctx,
		  `SELECT id, chain, family, default_fee_rate_sat, default_vbytes,
		        default_base_fee_gwei, default_priority_gwei, default_gas_limit,
		        default_energy_units, default_energy_price_sun,
		        default_bandwidth_bytes, default_bw_price_sun,
		        default_base_lamports, default_compute_units, default_micro_lamports_cu,
		        min_fee_user, max_fee_user, median_fee_native, active, created_at, updated_at
		 FROM fee_chain_specs WHERE active=1`)
	   if err != nil {
		  return err
	  }
	  defer rows.Close()
	   for rows.Next() {
		   var s ChainFeeSpec
		  var act int
		  var famStr string
		  if err := rows.Scan(
			   &s.ID, &s.Chain, &famStr,
			  decimalScan(&s.DefaultFeeRateSatPerVByte), &s.DefaultEstimatedVBytes,
			  decimalScan(&s.DefaultBaseFeeGwei), decimalScan(&s.DefaultPriorityFeeGwei), &s.DefaultGasLimit,
			   &s.DefaultEnergyUnits,
			  decimalScan(&s.DefaultEnergyPriceSun),
			  &s.DefaultBandwidthBytes, decimalScan(&s.DefaultBandwidthPriceSun),
			   &s.DefaultBaseLamports,
			   &s.DefaultComputeUnits, &s.DefaultMicroLamportsPerCU,
			  &s.MinFeeUser, &s.MaxFeeUser, decimalScan(&s.MedianFeeNative),
			  &act, &s.CreatedAt, &s.UpdatedAt,
		  ); err != nil {
			   continue
		   }
		  s.Family = Family(famStr)
		   s.Active = act != 0
		   b.mu.Lock()
		  b.specs[strings.ToUpper(s.Chain)] = s
		  b.mu.Unlock()
	  }
	   return rows.Err()
   }

// decimalScan 返回一个 sql.Scanner 兼容接口，把 string 列扫描为 decimal.Decimal。
  func decimalScan(d *decimal.Decimal) sql.Scanner {
	   return &decimalScanner{d: d}
  }

type decimalScanner struct{ d *decimal.Decimal }

   func (ds *decimalScanner) Scan(src any) error {
	   switch v := src.(type) {
	   case string:
		   d, err := decimal.NewFromString(v)
		   if err != nil {
			  return err
		  }
		   *ds.d = d
	  case []byte:
		  d, err := decimal.NewFromString(string(v))
		   if err != nil {
			  return err
		   }
		  *ds.d = d
	   case nil:
		   *ds.d = decimal.Zero
	   default:
		   return fmt.Errorf("cannot scan %T into decimal.Decimal", src)
	   }
	  return nil
  }

// BindSQL 绑定 SQL 数据库并从库中加载规格（覆盖内置默认值）。

   func (b *SpecBook) BindSQL(db *persist.SQLDB) *SpecBook {
	   if b != nil && db != nil {
		   b.sqlDB = db
		   _ = b.sqlLoad(context.Background())
	   }
	   return b
  }

// sqlGetByChain 按链名单行查询（内部用）。

  func (b *SpecBook) sqlGetByChain(ctx context.Context, chain string) (ChainFeeSpec, error) {
	  if !b.sqlOn() {
		  return ChainFeeSpec{}, errors.New("sql not available")
	   }
	  var s ChainFeeSpec
	   var act int
	  var famStr string
	   err := b.sqlDB.Conn(ctx).QueryRowContext(ctx,
		   `SELECT id, chain, family, min_fee_user, max_fee_user, median_fee_native, active, created_at, updated_at
		 FROM fee_chain_specs WHERE chain=?`, chain,
	   ).Scan(&s.ID, &s.Chain, &famStr, &s.MinFeeUser, &s.MaxFeeUser,
		  decimalScan(&s.MedianFeeNative), &act, &s.CreatedAt, &s.UpdatedAt)
	  if errors.Is(err, sql.ErrNoRows) {
		   return ChainFeeSpec{}, ErrModel
	  }
	  if err != nil {
		   return ChainFeeSpec{}, err
	  }
	  s.Family = Family(famStr)
	   s.Active = act != 0
	   return s, nil
   }
