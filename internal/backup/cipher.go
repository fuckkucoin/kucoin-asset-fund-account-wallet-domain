package backup

import "account-assets.kucoin.net/funding/internal/secretbox"

const magic = secretbox.KindBackup

  func seal(pass, plain []byte) ([]byte, error) {
	   out,
		   err := secretbox.Seal(magic, pass, plain)
	   return out, mapBox(err)
   }

   func open(pass, blob []byte) ([]byte, error) {
	   out, err := secretbox.Open(magic, pass, blob)
	   return out, mapBox(err)
  }

  func mapBox(err error) error {
	  if err == nil {
		  return nil
	   }
	  switch err {
	   case secretbox.ErrKey:
		  return ErrKey
	  case secretbox.ErrMagic:
		  return ErrMagic
	   case secretbox.ErrCipher:
		   return ErrCipher
	  case secretbox.ErrEmpty:
		   return ErrEmpty
	   default:
		  return err
	  }
   }
