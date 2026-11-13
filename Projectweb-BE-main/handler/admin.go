package handler

import (
	_ "embed"
	"net/http"
)

//go:embed admin.html
var adminHTML []byte

// AdminPanel ส่งหน้าเมนูแอดมิน (หน้าเดียว ทำงานฝั่งเบราว์เซอร์)
func (h *Handler) AdminPanel(w http.ResponseWriter, r *http.Request) {
	hd := w.Header()
	hd.Set("Content-Type", "text/html; charset=utf-8")
	hd.Set("Cache-Control", "no-store")
	hd.Set("X-Frame-Options", "DENY")
	hd.Set("X-Content-Type-Options", "nosniff")
	hd.Set("Referrer-Policy", "no-referrer")
	hd.Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src https: data:; connect-src 'self'; form-action 'none'; base-uri 'none'; frame-ancestors 'none'")
	_, _ = w.Write(adminHTML)
}
