package main

import (
	"context"
	_ "embed"
	"log"
	"mangahub/auth"
	"mangahub/handler"
	"mangahub/mw"
	"mangahub/store"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

//go:embed schema.sql
var schemaSQL string

func main() {
	// โหลดไฟล์ .env ก่อนอ่านค่าตัวแปร
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: .env file not found, using system environment")
	}

	// ตรวจสอบ JWT_SECRET
	secret := os.Getenv("JWT_SECRET")
	if len(secret) < 32 {
		log.Fatal("JWT_SECRET ต้องยาวอย่างน้อย 32 ตัวอักษร")
	}

	// เชื่อมต่อ PostgreSQL
	pool, e := pgxpool.New(context.Background(), os.Getenv("DATABASE_URL"))
	if e != nil {
		log.Fatal(e)
	}
	defer pool.Close()

	if e = pool.Ping(context.Background()); e != nil {
		log.Fatal("ต่อ PostgreSQL ไม่ได้: ", e)
	}

	// สร้างตารางอัตโนมัติ (schema.sql ใช้ IF NOT EXISTS จึงรันซ้ำได้)
	if _, e = pool.Exec(context.Background(), schemaSQL); e != nil {
		log.Println("Warning: สร้างตารางอัตโนมัติไม่สำเร็จ (ลองรัน schema.sql เอง):", e)
	}

	st := &store.Store{Pool: pool}

	// สร้าง/ตั้งแอดมินคนแรกจาก ADMIN_EMAIL + ADMIN_PASSWORD (ถ้ากำหนดไว้)
	if em := strings.ToLower(strings.TrimSpace(os.Getenv("ADMIN_EMAIL"))); em != "" {
		pw := os.Getenv("ADMIN_PASSWORD")
		if len(pw) < 8 || len(pw) > 72 {
			log.Fatal("ADMIN_PASSWORD ต้องยาว 8-72 ตัวอักษร")
		}
		hash, e := auth.HashPassword(pw)
		if e != nil {
			log.Fatal(e)
		}
		if e = st.EnsureAdmin(context.Background(), em, hash); e != nil {
			log.Fatal("ตั้งค่าแอดมินไม่สำเร็จ: ", e)
		}
		log.Println("แอดมินพร้อมใช้งาน:", em)
	}

	tickets := &auth.Issuer{
		Secret: []byte(secret),
		TTL:    24 * time.Hour,
	}

	h := &handler.Handler{
		Store:   st,
		Tickets: tickets,
	}

	login := func(f http.HandlerFunc) http.HandlerFunc {
		return mw.RequireLogin(tickets, f)
	}

	// เช็กสิทธิ์แอดมินจากฐานข้อมูลทุกครั้ง (ไม่เชื่อ role ใน token)
	admin := func(f http.HandlerFunc) http.HandlerFunc {
		return login(mw.RequireRoleDB("admin", func(ctx context.Context, id int64) (string, error) {
			m, e := st.MemberByID(ctx, id)
			return m.Role, e
		}, f))
	}

	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/auth/register", h.Register)
	mux.HandleFunc("POST /api/auth/login", h.Login)
	mux.HandleFunc("GET /api/me", login(h.Me))
	mux.HandleFunc("GET /api/manga", h.Manga)
	mux.HandleFunc("GET /api/manga/{id}", h.Detail)
	mux.HandleFunc("GET /api/manga/{id}/chapters", h.Chapters)
	mux.HandleFunc("GET /api/chapters/{id}/pages", h.Pages)
	mux.HandleFunc("GET /api/me/bookmarks", login(h.Bookmarks))
	mux.HandleFunc("PUT /api/me/bookmarks/{mangaId}", login(h.Bookmark))

	// ---- เฉพาะแอดมิน ----
	mux.HandleFunc("GET /api/admin/stats", admin(h.Stats))
	mux.HandleFunc("GET /api/admin/members", admin(h.Members))
	mux.HandleFunc("PUT /api/admin/members/{memberId}/role", admin(h.Role))
	mux.HandleFunc("DELETE /api/admin/members/{memberId}", admin(h.DeleteMember))
	mux.HandleFunc("POST /api/admin/manga", admin(h.AddManga))
	mux.HandleFunc("POST /api/admin/manga/import", admin(h.ImportManga))
	mux.HandleFunc("DELETE /api/admin/manga/{id}", admin(h.DeleteManga))
	mux.HandleFunc("POST /api/admin/sync", admin(h.Sync))

	// หน้าเมนูแอดมิน: ตัว HTML เปิดได้ แต่จะแสดงเมนูก็ต่อเมื่อเป็นแอดมิน และข้อมูลทุกอย่างผ่าน API ด้านบนที่ล็อกสิทธิ์ไว้
	// เปิดผ่าน Vite ที่ http://localhost:5173/api/admin-panel จะใช้ login เดียวกับเว็บหลัก
	mux.HandleFunc("GET /api/admin-panel", h.AdminPanel)

	origins := strings.Split(os.Getenv("ALLOWED_ORIGINS"), ",")

	port := os.Getenv("PORT")
	if port == "" {
		port = "8000"
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mw.CORS(origins, mux),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Println("MangaHub API listening on :" + port)
	log.Fatal(srv.ListenAndServe())
}
