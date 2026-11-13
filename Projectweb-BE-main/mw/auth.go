package mw

import (
	"context"
	"encoding/json"
	"errors"
	"mangahub/auth"
	"mangahub/store"
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

// RequireRoleDB เช็ก role จากฐานข้อมูลทุกครั้ง (ไม่เชื่อ role ใน token)
// ทำให้แอดมินที่ถูกลดสิทธิ์หรือถูกลบ เสียสิทธิ์ทันที ไม่ต้องรอ token หมดอายุ
func RequireRoleDB(role string, lookup func(context.Context, int64) (string, error), next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := MemberID(r.Context())
		if !ok {
			fail(w, 401, "กรุณาเข้าสู่ระบบ")
			return
		}
		cur, e := lookup(r.Context(), id)
		if errors.Is(e, store.ErrNotFound) {
			fail(w, 401, "ไม่พบบัญชี")
			return
		}
		if e != nil {
			fail(w, 500, "ระบบขัดข้อง")
			return
		}
		if cur != role {
			fail(w, 403, "ไม่มีสิทธิ์ใช้งาน")
			return
		}
		next(w, r)
	}
}
