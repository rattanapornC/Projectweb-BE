package auth

import "golang.org/x/crypto/bcrypt"

func HashPassword(s string) (string, error) {
	b, e := bcrypt.GenerateFromPassword([]byte(s), bcrypt.DefaultCost)
	return string(b), e
}
func CheckPassword(h, s string) bool {
	return bcrypt.CompareHashAndPassword([]byte(h), []byte(s)) == nil
}
