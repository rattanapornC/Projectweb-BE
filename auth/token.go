package auth

import (
	"errors"
	"github.com/golang-jwt/jwt/v5"
	"strconv"
	"time"
)

type Claims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}
type Issuer struct {
	Secret []byte
	TTL    time.Duration
}

func (i *Issuer) Issue(id int64, role string) (string, error) {
	n := time.Now()
	c := Claims{Role: role, RegisteredClaims: jwt.RegisteredClaims{Subject: strconv.FormatInt(id, 10), IssuedAt: jwt.NewNumericDate(n), ExpiresAt: jwt.NewNumericDate(n.Add(i.TTL))}}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(i.Secret)
}
func (i *Issuer) Parse(s string) (int64, string, error) {
	var c Claims
	_, e := jwt.ParseWithClaims(s, &c, func(t *jwt.Token) (any, error) { return i.Secret, nil }, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
	if e != nil {
		return 0, "", e
	}
	id, e := strconv.ParseInt(c.Subject, 10, 64)
	if e != nil {
		return 0, "", errors.New("bad subject")
	}
	return id, c.Role, nil
}
