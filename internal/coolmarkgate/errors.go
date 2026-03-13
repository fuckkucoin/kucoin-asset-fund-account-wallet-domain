package coolmarkgate

import "fmt"

var (
	// ErrCoolLeaseNotFound 冷却租约不存在或已被清除。
	ErrCoolLeaseNotFound = fmt.Errorf("coolmarkgate: cool lease not found")
	// ErrCoolWindowActive 冷却窗口尚未结束，禁止放行归集执行。
	ErrCoolWindowActive = fmt.Errorf("coolmarkgate: cool window is still active")
	// ErrGenMismatch 调用方携带的 cool_gen 与当前世代不符，拒绝执行。
	ErrGenMismatch = fmt.Errorf("coolmarkgate: cool generation mismatch")
	// ErrNoProof 执行前未能找到有效冷却证明，拒绝放行。
	ErrNoProof = fmt.Errorf("coolmarkgate: cool proof missing, exec blocked")
	// ErrIllegalTransition CoolLease 状态机非法跃迁。
	ErrIllegalTransition = fmt.Errorf("coolmarkgate: illegal cool lease state transition")
	// ErrDuplicateMark 相同资产冷却租约已存在且处于活跃状态，不得重复 Mark。
	ErrDuplicateMark = fmt.Errorf("coolmarkgate: duplicate mark for same asset")
	// ErrInvalidInput 参数校验失败。
	ErrInvalidInput = fmt.Errorf("coolmarkgate: invalid input")
	// ErrProofNotFound 冷却证明不存在。
	ErrProofNotFound = fmt.Errorf("coolmarkgate: cool proof not found")
	// ErrBreachCaseNotFound 违约案例不存在。
	ErrBreachCaseNotFound = fmt.Errorf("coolmarkgate: cool breach case not found")
	// ErrExecBlockedByBreach 因存在未处置违约案例，禁止接着归集执行。
	ErrExecBlockedByBreach = fmt.Errorf("coolmarkgate: exec blocked due to open breach case")
)
