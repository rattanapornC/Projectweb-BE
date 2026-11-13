package store

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
)

var ErrNotFound = errors.New("not found")

type Store struct{ Pool *pgxpool.Pool }
type Member struct {
	ID           int64  `json:"id"`
	Email        string `json:"email"`
	DisplayName  string `json:"display_name"`
	Role         string `json:"role"`
	PasswordHash string `json:"-"`
}
type Manga struct {
	ID               string  `json:"id"`
	Title            string  `json:"title"`
	TitleTh          *string `json:"title_th"`
	CoverURL         *string `json:"cover_url"`
	OriginalLanguage *string `json:"original_language"`
	ContentRating    *string `json:"content_rating"`
	Description      *string `json:"description"`
}
type Chapter struct {
	ID                 string  `json:"id"`
	ChapterNumber      *string `json:"chapter_number"`
	Title              *string `json:"title"`
	TranslatedLanguage string  `json:"translated_language"`
	ScanlationGroup    *string `json:"scanlation_group"`
	IsFree             bool    `json:"is_free"`
}

func (s *Store) MemberByEmail(c context.Context, e string) (Member, error) {
	var m Member
	err := s.Pool.QueryRow(c, `select id,email,display_name,role,password_hash from members where lower(email)=lower($1)`, e).Scan(&m.ID, &m.Email, &m.DisplayName, &m.Role, &m.PasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return m, ErrNotFound
	}
	return m, err
}
func (s *Store) CreateMember(c context.Context, e, p, n string) (Member, error) {
	var m Member
	err := s.Pool.QueryRow(c, `insert into members(email,password_hash,display_name) values($1,$2,$3) returning id,email,display_name,role`, strings.ToLower(e), p, n).Scan(&m.ID, &m.Email, &m.DisplayName, &m.Role)
	return m, err
}
func (s *Store) MemberByID(c context.Context, id int64) (Member, error) {
	var m Member
	err := s.Pool.QueryRow(c, `select id,email,display_name,role from members where id=$1`, id).Scan(&m.ID, &m.Email, &m.DisplayName, &m.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return m, ErrNotFound
	}
	return m, err
}
func (s *Store) SearchManga(c context.Context, q string) ([]Manga, error) {
	rows, e := s.Pool.Query(c, `select id,title,title_th,cover_url,original_language,content_rating,description from mangas where $1='' or title ilike '%'||$1||'%' or coalesce(title_th,'') ilike '%'||$1||'%' order by title limit 100`, q)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Manga{}
	for rows.Next() {
		var m Manga
		if e = rows.Scan(&m.ID, &m.Title, &m.TitleTh, &m.CoverURL, &m.OriginalLanguage, &m.ContentRating, &m.Description); e != nil {
			return nil, e
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
func (s *Store) GetManga(c context.Context, id string) (Manga, error) {
	var m Manga
	e := s.Pool.QueryRow(c, `select id,title,title_th,cover_url,original_language,content_rating,description from mangas where id=$1`, id).Scan(&m.ID, &m.Title, &m.TitleTh, &m.CoverURL, &m.OriginalLanguage, &m.ContentRating, &m.Description)
	if errors.Is(e, pgx.ErrNoRows) {
		return m, ErrNotFound
	}
	return m, e
}
func (s *Store) Chapters(c context.Context, id string) ([]Chapter, error) {
	rows, e := s.Pool.Query(c, `select id,chapter_number,title,translated_language,scanlation_group,is_free from chapters where manga_id=$1 order by chapter_number desc nulls last, id`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Chapter{}
	for rows.Next() {
		var x Chapter
		if e = rows.Scan(&x.ID, &x.ChapterNumber, &x.Title, &x.TranslatedLanguage, &x.ScanlationGroup, &x.IsFree); e != nil {
			return nil, e
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (s *Store) Bookmark(c context.Context, uid int64, mid string) error {
	_, e := s.Pool.Exec(c, `insert into bookmarks(member_id,manga_id) values($1,$2) on conflict do nothing`, uid, mid)
	return e
}
func (s *Store) Bookmarks(c context.Context, uid int64) ([]Manga, error) {
	rows, e := s.Pool.Query(c, `select m.id,m.title,m.title_th,m.cover_url,m.original_language,m.content_rating,m.description from mangas m join bookmarks b on b.manga_id=m.id where b.member_id=$1 order by b.created_at desc`, uid)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Manga{}
	for rows.Next() {
		var m Manga
		if e = rows.Scan(&m.ID, &m.Title, &m.TitleTh, &m.CoverURL, &m.OriginalLanguage, &m.ContentRating, &m.Description); e != nil {
			return nil, e
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
func (s *Store) Stats(c context.Context) (map[string]int, error) {
	var a, b, d int
	e := s.Pool.QueryRow(c, `select (select count(*) from mangas),(select count(*) from members),(select count(*) from chapters)`).Scan(&a, &b, &d)
	return map[string]int{"mangas": a, "members": b, "chapters": d}, e
}
func (s *Store) Members(c context.Context) ([]Member, error) {
	rows, e := s.Pool.Query(c, `select id,email,display_name,role from members order by id desc`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Member{}
	for rows.Next() {
		var m Member
		if e = rows.Scan(&m.ID, &m.Email, &m.DisplayName, &m.Role); e != nil {
			return nil, e
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
func (s *Store) SetRole(c context.Context, id int64, role string) (Member, error) {
	var m Member
	e := s.Pool.QueryRow(c, `update members set role=$2 where id=$1 returning id,email,display_name,role`, id, role).Scan(&m.ID, &m.Email, &m.DisplayName, &m.Role)
	if errors.Is(e, pgx.ErrNoRows) {
		return m, ErrNotFound
	}
	return m, e
}
