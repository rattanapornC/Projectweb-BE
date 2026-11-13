package handler

import (
	"context"
	"encoding/json"
	"errors"
	"mangahub/auth"
	"mangahub/mw"
	"mangahub/store"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Handler struct {
	Store   *store.Store
	Tickets *auth.Issuer
}

func out(w http.ResponseWriter, s int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(s)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, s int, e string) { out(w, s, map[string]string{"error": e}) }
func id(r *http.Request, k string) (int64, bool) {
	v, e := strconv.ParseInt(r.PathValue(k), 10, 64)
	return v, e == nil && v > 0
}
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var x struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Name     string `json:"display_name"`
	}
	if json.NewDecoder(r.Body).Decode(&x) != nil {
		fail(w, 400, "ข้อมูลไม่ถูกต้อง")
		return
	}
	x.Email = strings.TrimSpace(strings.ToLower(x.Email))
	x.Name = strings.TrimSpace(x.Name)
	if !strings.Contains(x.Email, "@") || len(x.Password) < 8 || x.Name == "" {
		fail(w, 400, "กรุณากรอกอีเมล ชื่อ และรหัสผ่านอย่างน้อย 8 ตัว")
		return
	}
	hash, e := auth.HashPassword(x.Password)
	if e != nil {
		fail(w, 500, "ระบบขัดข้อง")
		return
	}
	m, e := h.Store.CreateMember(r.Context(), x.Email, hash, x.Name)
	if e != nil {
		fail(w, 409, "อีเมลนี้ถูกใช้แล้ว")
		return
	}
	h.issue(w, m, 201)
}
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var x struct{ Email, Password string }
	if json.NewDecoder(r.Body).Decode(&x) != nil {
		fail(w, 400, "ข้อมูลไม่ถูกต้อง")
		return
	}
	m, e := h.Store.MemberByEmail(r.Context(), x.Email)
	if e != nil || !auth.CheckPassword(m.PasswordHash, x.Password) {
		fail(w, 401, "อีเมลหรือรหัสผ่านไม่ถูกต้อง")
		return
	}
	h.issue(w, m, 200)
}
func (h *Handler) issue(w http.ResponseWriter, m store.Member, s int) {
	t, e := h.Tickets.Issue(m.ID, m.Role)
	if e != nil {
		fail(w, 500, "สร้าง token ไม่สำเร็จ")
		return
	}
	out(w, s, map[string]any{"token": t, "member": map[string]any{"id": m.ID, "email": m.Email, "display_name": m.DisplayName, "role": m.Role}})
}
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	uid, _ := mw.MemberID(r.Context())
	m, e := h.Store.MemberByID(r.Context(), uid)
	if e != nil {
		fail(w, 401, "ไม่พบบัญชี")
		return
	}
	out(w, 200, m)
}
func (h *Handler) Manga(w http.ResponseWriter, r *http.Request) {
	items, e := h.Store.SearchManga(r.Context(), r.URL.Query().Get("q"))
	if e != nil {
		fail(w, 500, e.Error())
		return
	}
	out(w, 200, map[string]any{"items": items})
}
func (h *Handler) Detail(w http.ResponseWriter, r *http.Request) {
	m, e := h.Store.GetManga(r.Context(), r.PathValue("id"))
	if errors.Is(e, store.ErrNotFound) {
		fail(w, 404, "ไม่พบมังงะ")
		return
	}
	if e != nil {
		fail(w, 500, e.Error())
		return
	}
	out(w, 200, m)
}
func (h *Handler) Chapters(w http.ResponseWriter, r *http.Request) {
	mid := r.PathValue("id")
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()
	if e := h.Store.SyncChapters(ctx, mid); e != nil {
		fail(w, 502, "ดึงรายการตอนจาก MangaDex ไม่สำเร็จ: "+e.Error())
		return
	}
	items, e := h.Store.Chapters(ctx, mid)
	if e != nil {
		fail(w, 500, e.Error())
		return
	}
	out(w, 200, map[string]any{"items": items})
}
func (h *Handler) Pages(w http.ResponseWriter, r *http.Request) {
	p, e := h.Store.Pages(r.Context(), r.PathValue("id"))
	if e != nil {
		fail(w, 502, "โหลดหน้ามังงะไม่สำเร็จ")
		return
	}
	out(w, 200, map[string]any{"pages": p})
}
func (h *Handler) Bookmark(w http.ResponseWriter, r *http.Request) {
	uid, _ := mw.MemberID(r.Context())
	if e := h.Store.Bookmark(r.Context(), uid, r.PathValue("mangaId")); e != nil {
		fail(w, 404, "ไม่พบมังงะ")
		return
	}
	w.WriteHeader(204)
}
func (h *Handler) Bookmarks(w http.ResponseWriter, r *http.Request) {
	uid, _ := mw.MemberID(r.Context())
	a, e := h.Store.Bookmarks(r.Context(), uid)
	if e != nil {
		fail(w, 500, e.Error())
		return
	}
	out(w, 200, map[string]any{"items": a})
}
func (h *Handler) Stats(w http.ResponseWriter, r *http.Request) {
	a, e := h.Store.Stats(r.Context())
	if e != nil {
		fail(w, 500, e.Error())
		return
	}
	out(w, 200, a)
}
func (h *Handler) Members(w http.ResponseWriter, r *http.Request) {
	a, e := h.Store.Members(r.Context())
	if e != nil {
		fail(w, 500, e.Error())
		return
	}
	out(w, 200, map[string]any{"items": a})
}
func (h *Handler) Role(w http.ResponseWriter, r *http.Request) {
	n, ok := id(r, "memberId")
	if !ok {
		fail(w, 400, "member id ไม่ถูกต้อง")
		return
	}
	var x struct {
		Role string `json:"role"`
	}
	if json.NewDecoder(r.Body).Decode(&x) != nil || (x.Role != "member" && x.Role != "admin") {
		fail(w, 400, "role ไม่ถูกต้อง")
		return
	}
	m, e := h.Store.SetRole(r.Context(), n, x.Role)
	if e != nil {
		fail(w, 404, "ไม่พบสมาชิก")
		return
	}
	out(w, 200, m)
}
func (h *Handler) Sync(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	n, e := h.Store.SyncManga(ctx)
	if e != nil {
		fail(w, 502, "MangaDex sync failed: "+e.Error())
		return
	}
	out(w, 200, map[string]int{"synced": n})
}
