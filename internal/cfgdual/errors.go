package cfgdual

import "fmt"

var (
	// ErrUnavailable 模块不可用（未初始化或 SQL 断连）。
	ErrUnavailable = fmt.Errorf("cfgdual unavailable")
	// ErrNotFound 指定资源不存在。
	ErrNotFound = fmt.Errorf("cfgdual not found")
	// ErrConflict 状态冲突（如代次倒退、状态非法迁移）。
	ErrConflict = fmt.Errorf("cfgdual conflict")
	// ErrStale 代次已过期，禁止覆盖。
	ErrStale = fmt.Errorf("cfgdual version stale")
	// ErrSealEnforced 封堵闸已强制执行，高风险写被挡回。
	ErrSealEnforced = fmt.Errorf("cfgdual seal enforced: split-brain detected, fail-closed")
	// ErrKillSwitch 任一侧 kill switch 打开，整请求拒绝。
	ErrKillSwitch = fmt.Errorf("cfgdual kill switch engaged on at least one side")
	// ErrInvalidState 非法状态迁移。
	ErrInvalidState = fmt.Errorf("cfgdual invalid state transition")
	// ErrReleaseNotActive 发布未处于 canary 状态，无法双评。
	ErrReleaseNotActive = fmt.Errorf("cfgdual release not in canary state")
	// ErrVersionRequired 代次号必须大于零。
	ErrVersionRequired = fmt.Errorf("cfgdual version required")
	// ErrPackRequired 包名不能为空。
	ErrPackRequired = fmt.Errorf("cfgdual pack name required")
	// ErrGenerationMismatch 世代号不匹配，请求需用最新世代重新评估。
	ErrGenerationMismatch = fmt.Errorf("cfgdual generation mismatch")
	// ErrRollbackProofExists 同一发布已有回滚证明，禁止重复提交。
	ErrRollbackProofExists = fmt.Errorf("cfgdual rollback proof already exists")
)
