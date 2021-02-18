package backup

import "fmt"

var (
	ErrUnavailable = fmt.Errorf("backup store unavailable")
	ErrKey = fmt.Errorf("backup passphrase invalid")
	ErrChecksum = fmt.Errorf("backup checksum mismatch")
	ErrMagic = fmt.Errorf("backup blob magic invalid")
	ErrCipher   = fmt.Errorf("backup ciphertext invalid")
	ErrEmpty    = fmt.Errorf("backup payload empty")
	ErrNotFound = fmt.Errorf("backup blob not found")
)
