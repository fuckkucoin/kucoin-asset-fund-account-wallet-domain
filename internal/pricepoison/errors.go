package pricepoison

import "fmt"

var (
	// ErrPoisonedTick 毒价 tick；不得签发令牌。
	ErrPoisonedTick = fmt.Errorf("pricepoison: tick is poisoned")
	// ErrTokenRevoked 令牌已被撤销或代次不符。
	ErrTokenRevoked = fmt.Errorf("pricepoison: token revoked")
	// ErrTokenExpired 令牌超过最大有效期。
	ErrTokenExpired = fmt.Errorf("pricepoison: token expired")
	// ErrRegistryUnavailable 令牌注册表不可用（nil 接收器）。
	ErrRegistryUnavailable = fmt.Errorf("pricepoison: token registry unavailable")
	// ErrDetectorNil 检测器为 nil。
	ErrDetectorNil = fmt.Errorf("pricepoison: detector is nil")
)
