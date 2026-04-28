package auth

import "golang.org/x/crypto/bcrypt"

// BcryptCost is the work factor used by HashPassword. It is a var so tests
// may lower it; production code must not change it.
var BcryptCost = 12

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
	return string(hash), err
}

func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
