package store

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
)

// MangaInput คือข้อมูลที่แอดมินกรอกตอนเพิ่มมังงะเอง (ช่องว่างจะถูกเก็บเป็น NULL)
type MangaInput struct {
	Title            string
	TitleTh          string
	CoverURL         string
	OriginalLanguage string
	ContentRating    string
	Description      string
}

func newUUID() (string, error) {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		return "", e
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

func (s *Store) blockedSet(c context.Context) (map[string]bool, error) {
	rows, e := s.Pool.Query(c, `select id from blocked_mangas`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			return nil, e
		}
		out[id] = true
	}
	return out, rows.Err()
}

// CreateManga เพิ่มมังงะใหม่ด้วยมือ (สร้าง id แบบ UUID ให้)
func (s *Store) CreateManga(c context.Context, in MangaInput) (Manga, error) {
	id, e := newUUID()
	if e != nil {
		return Manga{}, e
	}
	_, e = s.Pool.Exec(c, `insert into mangas(id,title,title_th,cover_url,original_language,content_rating,description) values($1,$2,nullif($3,''),nullif($4,''),nullif($5,''),nullif($6,''),nullif($7,''))`,
		id, in.Title, in.TitleTh, in.CoverURL, in.OriginalLanguage, in.ContentRating, in.Description)
	if e != nil {
		return Manga{}, e
	}
	return s.GetManga(c, id)
}

// DeleteManga ลบมังงะ (ตอนและบุ๊กมาร์กถูกลบตามด้วย cascade)
// และจดไว้ใน blocked_mangas เพื่อไม่ให้ Sync จาก MangaDex ดึงกลับมาอีก
func (s *Store) DeleteManga(c context.Context, id string) error {
	tx, e := s.Pool.Begin(c)
	if e != nil {
		return e
	}
	defer tx.Rollback(c)
	tag, e := tx.Exec(c, `delete from mangas where id=$1`, id)
	if e != nil {
		return e
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if _, e = tx.Exec(c, `insert into blocked_mangas(id) values($1) on conflict do nothing`, id); e != nil {
		return e
	}
	return tx.Commit(c)
}

// DeleteMember ลบสมาชิก แต่ไม่ยอมลบแอดมินคนสุดท้าย
func (s *Store) DeleteMember(c context.Context, id int64) error {
	tag, e := s.Pool.Exec(c, `delete from members where id=$1 and (role<>'admin' or (select count(*) from members where role='admin')>1)`, id)
	if e != nil {
		return e
	}
	if tag.RowsAffected() == 0 {
		if _, e = s.MemberByID(c, id); e != nil {
			return e // ErrNotFound หรือ error อื่น
		}
		return ErrLastAdmin
	}
	return nil
}

// EnsureAdmin สร้างแอดมินจาก ADMIN_EMAIL/ADMIN_PASSWORD ตอนเปิดเซิร์ฟเวอร์
// ถ้ามีบัญชีอีเมลนี้อยู่แล้วจะแค่ตั้ง role เป็น admin (ไม่แตะรหัสผ่านเดิม)
func (s *Store) EnsureAdmin(c context.Context, email, passwordHash string) error {
	if email == "" || passwordHash == "" {
		return errors.New("email/password ว่าง")
	}
	_, e := s.Pool.Exec(c, `insert into members(email,password_hash,display_name,role) values($1,$2,'Admin','admin') on conflict(email) do update set role='admin'`, email, passwordHash)
	return e
}
