package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mangahub/auth"
	"mangahub/mw"
	"mangahub/store"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
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

func fail(w http.ResponseWriter, s int, e string) {
	out(w, s, map[string]string{"error": e})
}

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
	var x struct {
		Email    string
		Password string
	}

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

	out(w, s, map[string]any{
		"token": t,
		"member": map[string]any{
			"id":           m.ID,
			"email":        m.Email,
			"display_name": m.DisplayName,
			"role":         m.Role,
		},
	})
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

func (h *Handler) Popular(w http.ResponseWriter, r *http.Request) {
	items, e := h.Store.SearchManga(r.Context(), "")
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

// ChapterInfo ใช้ค้นหาว่าตอนนี้เป็นของมังงะเรื่องไหน และเป็นภาษาอะไร
func (h *Handler) ChapterInfo(w http.ResponseWriter, r *http.Request) {
	var mangaID string
	var language string

	err := h.Store.Pool.QueryRow(
		r.Context(),
		`SELECT manga_id, translated_language
		 FROM chapters
		 WHERE id = $1`,
		r.PathValue("id"),
	).Scan(&mangaID, &language)

	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "ไม่พบตอนนี้")
		return
	}

	if err != nil {
		fail(w, 500, "ค้นหาข้อมูลตอนไม่สำเร็จ")
		return
	}

	out(w, 200, map[string]string{
		"mangaId":  mangaID,
		"language": language,
	})
}

func (h *Handler) Pages(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()

	p, e := h.Store.Pages(ctx, r.PathValue("id"))
	if e != nil {
		fail(w, 502, "โหลดหน้ามังงะไม่สำเร็จ: "+e.Error())
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

	if json.NewDecoder(r.Body).Decode(&x) != nil ||
		(x.Role != "member" && x.Role != "admin") {
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

var (
	mangaIDPattern = regexp.MustCompile(
		`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`,
	)

	coverFilePattern = regexp.MustCompile(
		`^[a-zA-Z0-9_-]+\.(jpg|jpeg|png|webp)(\.256\.jpg)?$`,
	)

	// รองรับชื่อไฟล์หน้ามังงะที่มีตัวอักษร ตัวเลข _ และ -
	pageFilePattern = regexp.MustCompile(
		`^[a-zA-Z0-9_-]+\.(jpg|jpeg|png|webp)$`,
	)

	pageHashPattern = regexp.MustCompile(
		`^[a-fA-F0-9]+$`,
	)
)

func (h *Handler) Cover(w http.ResponseWriter, r *http.Request) {
	rawURL := r.URL.Query().Get("url")

	parsed, err := url.Parse(rawURL)
	if err != nil {
		http.Error(w, "URL รูปภาพไม่ถูกต้อง", http.StatusBadRequest)
		return
	}

	if parsed.Scheme != "https" ||
		parsed.Host != "uploads.mangadex.org" ||
		parsed.User != nil ||
		parsed.RawQuery != "" ||
		parsed.Fragment != "" {
		http.Error(w, "URL รูปภาพไม่ถูกต้อง", http.StatusBadRequest)
		return
	}

	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 3 ||
		parts[0] != "covers" ||
		!mangaIDPattern.MatchString(parts[1]) ||
		!coverFilePattern.MatchString(parts[2]) {
		http.Error(w, "เส้นทางรูปภาพไม่ถูกต้อง", http.StatusBadRequest)
		return
	}

	targetURL := "https://uploads.mangadex.org" + parsed.EscapedPath()

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		http.Error(w, "สร้างคำขอรูปภาพไม่สำเร็จ", http.StatusBadGateway)
		return
	}

	req.Header.Set("User-Agent", "MangaHub/1.0")
	req.Header.Set("Accept", "image/avif,image/webp,image/png,image/jpeg,*/*")

	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "เชื่อมต่อ MangaDex ไม่สำเร็จ", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		http.Error(w, "MangaDex ไม่สามารถส่งรูปภาพได้", http.StatusBadGateway)
		return
	}

	const maxImageSize = 10 << 20

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageSize+1))
	if err != nil || len(data) == 0 || len(data) > maxImageSize {
		http.Error(w, "ไฟล์รูปภาพไม่ถูกต้องหรือมีขนาดใหญ่เกินไป", http.StatusBadGateway)
		return
	}

	contentType := http.DetectContentType(data)
	if !strings.HasPrefix(contentType, "image/") {
		http.Error(w, "ข้อมูลที่ได้รับไม่ใช่รูปภาพ", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (h *Handler) PageImage(w http.ResponseWriter, r *http.Request) {
	rawURL := r.URL.Query().Get("url")

	parsed, err := url.Parse(rawURL)
	if err != nil {
		http.Error(w, "URL รูปหน้าไม่ถูกต้อง", http.StatusBadRequest)
		return
	}

	hostname := strings.ToLower(parsed.Hostname())

	allowedHost := hostname == "mangadex.network" ||
		strings.HasSuffix(hostname, ".mangadex.network")

	if parsed.Scheme != "https" ||
		hostname == "" ||
		!allowedHost ||
		parsed.Port() != "" ||
		parsed.User != nil ||
		parsed.RawQuery != "" ||
		parsed.Fragment != "" {
		http.Error(w, "URL รูปหน้าไม่ถูกต้อง", http.StatusBadRequest)
		return
	}

	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")

	if len(parts) != 3 {
		http.Error(w, "เส้นทางรูปหน้าต้องมี 3 ส่วน", http.StatusBadRequest)
		return
	}

	if parts[0] != "data" && parts[0] != "data-saver" {
		http.Error(w, "โฟลเดอร์รูปหน้าไม่ถูกต้อง", http.StatusBadRequest)
		return
	}

	if !pageHashPattern.MatchString(parts[1]) {
		http.Error(w, "Hash รูปหน้าไม่ถูกต้อง", http.StatusBadRequest)
		return
	}

	if !pageFilePattern.MatchString(parts[2]) {
		http.Error(w, "ชื่อไฟล์รูปหน้าไม่ถูกต้อง", http.StatusBadRequest)
		return
	}

	targetURL := "https://" + parsed.Host + parsed.EscapedPath()

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		http.Error(w, "สร้างคำขอรูปไม่สำเร็จ", http.StatusBadGateway)
		return
	}

	req.Header.Set("User-Agent", "MangaHub/1.0")
	req.Header.Set("Accept", "image/avif,image/webp,image/png,image/jpeg,*/*")

	client := &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "เชื่อมต่อ MangaDex ไม่สำเร็จ", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		http.Error(w, "MangaDex ไม่สามารถส่งรูปหน้านี้ได้", http.StatusBadGateway)
		return
	}

	const maxImageSize = 15 << 20

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageSize+1))
	if err != nil || len(data) == 0 || len(data) > maxImageSize {
		http.Error(w, "ไฟล์รูปไม่ถูกต้องหรือมีขนาดใหญ่เกินไป", http.StatusBadGateway)
		return
	}

	contentType := http.DetectContentType(data)
	if !strings.HasPrefix(contentType, "image/") {
		http.Error(w, "ข้อมูลที่ได้รับไม่ใช่รูปภาพ", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
