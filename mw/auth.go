package mw

import (
	"context"
	"encoding/json"
	"mangahub/auth"
	"net/http"
	"strings"
)

type key int

const (
	idKey key = iota
	roleKey
)

func MemberID(c context.Context) (int64, bool) { v, ok := c.Value(idKey).(int64); return v, ok }
func Role(c context.Context) string            { v, _ := c.Value(roleKey).(string); return v }
func fail(w http.ResponseWriter, s int, m string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(s)
	json.NewEncoder(w).Encode(map[string]string{"error": m})
}
func RequireLogin(i *auth.Issuer, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			fail(w, 401, "กรุณาเข้าสู่ระบบ")
			return
		}
		id, role, e := i.Parse(strings.TrimPrefix(h, "Bearer "))
		if e != nil {
			fail(w, 401, "Token ไม่ถูกต้องหรือหมดอายุ")
			return
		}
		c := context.WithValue(r.Context(), idKey, id)
		c = context.WithValue(c, roleKey, role)
		next(w, r.WithContext(c))
	}
}
func RequireRole(role string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if Role(r.Context()) != role {
			fail(w, 403, "ไม่มีสิทธิ์ใช้งาน")
			return
		}
		next(w, r)
	}
}
