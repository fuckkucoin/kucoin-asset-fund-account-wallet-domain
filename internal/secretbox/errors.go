package secretbox

import "fmt"

var (
	ErrKey = fmt.Errorf("secret box passphrase invalid")
	ErrMagic = fmt.Errorf("secret box magic invalid")
	ErrCipher = fmt.Errorf("secret box ciphertext invalid")
	ErrEmpty  = fmt.Errorf("secret box payload empty")
	ErrKind   = fmt.Errorf("secret box kind invalid")
	ErrHash = fmt.Errorf("secret box payload hash mismatch")
)
