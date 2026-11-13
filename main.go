package main

import (
	"context"
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
	_ "github.com/joho/godotenv/autoload"
)

func main() {
	secret := os.Getenv("JWT_SECRET")
	if len(secret) < 32 {
		log.Fatal("JWT_SECRET ต้องยาวอย่างน้อย 32 ตัวอักษร")
	}
	pool, e := pgxpool.New(context.Background(), os.Getenv("DATABASE_URL"))
	if e != nil {
		log.Fatal(e)
	}
	defer pool.Close()
	if e = pool.Ping(context.Background()); e != nil {
		log.Fatal("ต่อ PostgreSQL ไม่ได้: ", e)
	}
	tickets := &auth.Issuer{Secret: []byte(secret), TTL: 24 * time.Hour}
	h := &handler.Handler{Store: &store.Store{Pool: pool}, Tickets: tickets}
	login := func(f http.HandlerFunc) http.HandlerFunc { return mw.RequireLogin(tickets, f) }
	admin := func(f http.HandlerFunc) http.HandlerFunc { return login(mw.RequireRole("admin", f)) }
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
	mux.HandleFunc("GET /api/admin/stats", admin(h.Stats))
	mux.HandleFunc("GET /api/admin/members", admin(h.Members))
	mux.HandleFunc("PUT /api/admin/members/{memberId}/role", admin(h.Role))
	mux.HandleFunc("POST /api/admin/sync", admin(h.Sync))
	origins := strings.Split(os.Getenv("ALLOWED_ORIGINS"), ",")
	port := os.Getenv("PORT")
	if port == "" {
		port = "8000"
	}
	log.Println("MangaHub API listening on :" + port)
	log.Fatal(http.ListenAndServe(":"+port, mw.CORS(origins, mux)))
}
