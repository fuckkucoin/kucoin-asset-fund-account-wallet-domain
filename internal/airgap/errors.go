package airgap

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("airgap ceremony unavailable")
	ErrSession  = fmt.Errorf("airgap session not found")
	ErrPhase    = fmt.Errorf("airgap illegal phase")
	ErrActor    = fmt.Errorf("airgap actor invalid")
	ErrQuorum   = fmt.Errorf("airgap quorum not met")
	ErrHash     = fmt.Errorf("airgap payload hash mismatch")
	ErrEnvelope = fmt.Errorf("airgap envelope invalid")
	ErrSameActor = fmt.Errorf("airgap seal needs a second actor")
	ErrThreshold = fmt.Errorf("airgap threshold invalid")
	ErrPayload   = fmt.Errorf("airgap payload invalid")
	ErrImported  = fmt.Errorf("airgap import refused")
	ErrLabel     = fmt.Errorf("airgap label invalid")
	ErrKey = fmt.Errorf("airgap passphrase invalid")
	ErrCipher = fmt.Errorf("airgap sealed blob invalid")
	ErrSealed = fmt.Errorf("airgap sealed envelope invalid")
)
