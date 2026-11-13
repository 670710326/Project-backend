package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"mangahub/auth"
	"mangahub/mw"
	"mangahub/store"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
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

// internal บันทึกสาเหตุจริงไว้ใน log แล้วตอบ client แบบทั่วไป (ไม่เปิดเผยรายละเอียดฐานข้อมูล)
func internal(w http.ResponseWriter, e error) {
	log.Println("internal error:", e)
	fail(w, 500, "ระบบขัดข้อง")
}

// decode อ่าน JSON จาก body (จำกัด 1 MB)
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	return json.NewDecoder(r.Body).Decode(v) == nil
}

func id(r *http.Request, k string) (int64, bool) {
	v, e := strconv.ParseInt(r.PathValue(k), 10, 64)
	return v, e == nil && v > 0
}

// num อ่านเลขจาก query string และบีบให้อยู่ในช่วง [lo, hi]
func num(s string, def, lo, hi int) int {
	n, e := strconv.Atoi(s)
	if e != nil {
		return def
	}
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}

// ---------------- สมัคร / เข้าสู่ระบบ ----------------

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var x struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Name     string `json:"display_name"`
	}
	if !decode(w, r, &x) {
		fail(w, 400, "ข้อมูลไม่ถูกต้อง")
		return
	}
	x.Email = strings.TrimSpace(strings.ToLower(x.Email))
	x.Name = strings.TrimSpace(x.Name)
	if !strings.Contains(x.Email, "@") || len(x.Email) > 255 || len(x.Password) < 8 || x.Name == "" {
		fail(w, 400, "กรุณากรอกอีเมล ชื่อ และรหัสผ่านอย่างน้อย 8 ตัว")
		return
	}
	if len(x.Password) > 72 { // bcrypt รับได้ไม่เกิน 72 ไบต์
		fail(w, 400, "รหัสผ่านต้องไม่เกิน 72 ตัวอักษร")
		return
	}
	if utf8.RuneCountInString(x.Name) > 80 {
		fail(w, 400, "ชื่อที่แสดงต้องไม่เกิน 80 ตัวอักษร")
		return
	}
	hash, e := auth.HashPassword(x.Password)
	if e != nil {
		internal(w, e)
		return
	}
	m, e := h.Store.CreateMember(r.Context(), x.Email, hash, x.Name)
	if errors.Is(e, store.ErrDuplicate) {
		fail(w, 409, "อีเมลนี้ถูกใช้แล้ว")
		return
	}
	if e != nil {
		internal(w, e)
		return
	}
	h.issue(w, m, 201)
}
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var x struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decode(w, r, &x) {
		fail(w, 400, "ข้อมูลไม่ถูกต้อง")
		return
	}
	m, e := h.Store.MemberByEmail(r.Context(), strings.TrimSpace(x.Email))
	if e != nil && !errors.Is(e, store.ErrNotFound) {
		internal(w, e)
		return
	}
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
	if errors.Is(e, store.ErrNotFound) {
		fail(w, 401, "ไม่พบบัญชี")
		return
	}
	if e != nil {
		internal(w, e)
		return
	}
	out(w, 200, m)
}

// ---------------- มังงะ (สาธารณะ) ----------------

func (h *Handler) Manga(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	items, e := h.Store.SearchManga(r.Context(), strings.TrimSpace(q.Get("q")), num(q.Get("limit"), 100, 1, 100), num(q.Get("offset"), 0, 0, 1000000))
	if e != nil {
		internal(w, e)
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
		internal(w, e)
		return
	}
	out(w, 200, m)
}
func (h *Handler) Chapters(w http.ResponseWriter, r *http.Request) {
	mid := r.PathValue("id")
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()
	if _, e := h.Store.GetManga(ctx, mid); errors.Is(e, store.ErrNotFound) {
		fail(w, 404, "ไม่พบมังงะ")
		return
	}
	if e := h.Store.SyncChapters(ctx, mid); e != nil {
		fail(w, 502, "ดึงรายการตอนจาก MangaDex ไม่สำเร็จ: "+e.Error())
		return
	}
	items, e := h.Store.Chapters(ctx, mid)
	if e != nil {
		internal(w, e)
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
		internal(w, e)
		return
	}
	out(w, 200, map[string]any{"items": a})
}

// ---------------- แอดมิน ----------------

func (h *Handler) Stats(w http.ResponseWriter, r *http.Request) {
	a, e := h.Store.Stats(r.Context())
	if e != nil {
		internal(w, e)
		return
	}
	out(w, 200, a)
}

func (h *Handler) Members(w http.ResponseWriter, r *http.Request) {
	a, e := h.Store.Members(r.Context(), strings.TrimSpace(r.URL.Query().Get("q")))
	if e != nil {
		internal(w, e)
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
	if !decode(w, r, &x) || (x.Role != "member" && x.Role != "admin") {
		fail(w, 400, "role ไม่ถูกต้อง")
		return
	}
	if uid, _ := mw.MemberID(r.Context()); uid == n {
		fail(w, 400, "เปลี่ยนสิทธิ์ของบัญชีตัวเองไม่ได้")
		return
	}
	m, e := h.Store.SetRole(r.Context(), n, x.Role)
	if errors.Is(e, store.ErrNotFound) {
		fail(w, 404, "ไม่พบสมาชิก")
		return
	}
	if errors.Is(e, store.ErrLastAdmin) {
		fail(w, 409, "ต้องมีแอดมินอย่างน้อย 1 คน")
		return
	}
	if e != nil {
		internal(w, e)
		return
	}
	out(w, 200, m)
}

func (h *Handler) DeleteMember(w http.ResponseWriter, r *http.Request) {
	n, ok := id(r, "memberId")
	if !ok {
		fail(w, 400, "member id ไม่ถูกต้อง")
		return
	}
	if uid, _ := mw.MemberID(r.Context()); uid == n {
		fail(w, 400, "ลบบัญชีของตัวเองไม่ได้")
		return
	}
	e := h.Store.DeleteMember(r.Context(), n)
	if errors.Is(e, store.ErrNotFound) {
		fail(w, 404, "ไม่พบสมาชิก")
		return
	}
	if errors.Is(e, store.ErrLastAdmin) {
		fail(w, 409, "ต้องมีแอดมินอย่างน้อย 1 คน")
		return
	}
	if e != nil {
		internal(w, e)
		return
	}
	w.WriteHeader(204)
}

var ratings = map[string]bool{"": true, "safe": true, "suggestive": true, "erotica": true, "pornographic": true}

// AddManga เพิ่มมังงะด้วยมือ
func (h *Handler) AddManga(w http.ResponseWriter, r *http.Request) {
	var x struct {
		Title            string `json:"title"`
		TitleTh          string `json:"title_th"`
		CoverURL         string `json:"cover_url"`
		OriginalLanguage string `json:"original_language"`
		ContentRating    string `json:"content_rating"`
		Description      string `json:"description"`
	}
	if !decode(w, r, &x) {
		fail(w, 400, "ข้อมูลไม่ถูกต้อง")
		return
	}
	in := store.MangaInput{
		Title:            strings.TrimSpace(x.Title),
		TitleTh:          strings.TrimSpace(x.TitleTh),
		CoverURL:         strings.TrimSpace(x.CoverURL),
		OriginalLanguage: strings.TrimSpace(x.OriginalLanguage),
		ContentRating:    strings.TrimSpace(x.ContentRating),
		Description:      strings.TrimSpace(x.Description),
	}
	if in.Title == "" || utf8.RuneCountInString(in.Title) > 300 || utf8.RuneCountInString(in.TitleTh) > 300 {
		fail(w, 400, "กรุณากรอกชื่อเรื่อง (ไม่เกิน 300 ตัวอักษร)")
		return
	}
	if in.CoverURL != "" {
		u, e := url.Parse(in.CoverURL)
		if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			fail(w, 400, "ลิงก์รูปปกต้องขึ้นต้นด้วย http:// หรือ https://")
			return
		}
	}
	if utf8.RuneCountInString(in.OriginalLanguage) > 10 {
		fail(w, 400, "รหัสภาษาต้นฉบับยาวเกินไป")
		return
	}
	if !ratings[in.ContentRating] {
		fail(w, 400, "content_rating ไม่ถูกต้อง")
		return
	}
	m, e := h.Store.CreateManga(r.Context(), in)
	if e != nil {
		internal(w, e)
		return
	}
	out(w, 201, m)
}

var uuidRe = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

// ImportManga เพิ่มมังงะจาก MangaDex ด้วย id หรือลิงก์ (เช่น https://mangadex.org/title/<id>/...)
func (h *Handler) ImportManga(w http.ResponseWriter, r *http.Request) {
	var x struct {
		MangadexID string `json:"mangadex_id"`
	}
	if !decode(w, r, &x) {
		fail(w, 400, "ข้อมูลไม่ถูกต้อง")
		return
	}
	mid := strings.ToLower(uuidRe.FindString(x.MangadexID))
	if mid == "" {
		fail(w, 400, "ไม่พบ MangaDex id ในข้อความที่กรอก")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()
	m, e := h.Store.ImportManga(ctx, mid)
	if errors.Is(e, store.ErrNotFound) {
		fail(w, 404, "ไม่พบมังงะนี้ใน MangaDex")
		return
	}
	if e != nil {
		fail(w, 502, "ดึงข้อมูลจาก MangaDex ไม่สำเร็จ: "+e.Error())
		return
	}
	out(w, 200, m)
}

// DeleteManga ลบมังงะ (พร้อมตอนและบุ๊กมาร์กของเรื่องนั้น)
func (h *Handler) DeleteManga(w http.ResponseWriter, r *http.Request) {
	e := h.Store.DeleteManga(r.Context(), r.PathValue("id"))
	if errors.Is(e, store.ErrNotFound) {
		fail(w, 404, "ไม่พบมังงะ")
		return
	}
	if e != nil {
		internal(w, e)
		return
	}
	w.WriteHeader(204)
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
