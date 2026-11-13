package store

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const md = "https://api.mangadex.org"

type mdResp struct {
	Data []struct {
		ID         string `json:"id"`
		Attributes struct {
			Title            map[string]string   `json:"title"`
			AltTitles        []map[string]string `json:"altTitles"`
			Description      map[string]string   `json:"description"`
			OriginalLanguage string              `json:"originalLanguage"`
			ContentRating    string              `json:"contentRating"`
		} `json:"attributes"`
		Relationships []struct {
			ID, Type   string
			Attributes struct {
				FileName string `json:"fileName"`
			} `json:"attributes"`
		} `json:"relationships"`
	} `json:"data"`
	Result string `json:"result"`
}

func mdGet(ctx context.Context, path string, v any) error {
	req, e := http.NewRequestWithContext(ctx, "GET", md+path, nil)
	if e != nil {
		return e
	}
	req.Header.Set("User-Agent", "MangaHub/1.0")
	cl := &http.Client{Timeout: 35 * time.Second}
	res, e := cl.Do(req)
	if e != nil {
		return e
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("MangaDex HTTP %d", res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(v)
}
func (s *Store) SyncManga(ctx context.Context) (int, error) {
	q := url.Values{}
	q.Set("limit", "100")
	q.Set("offset", "0")
	q.Add("includes[]", "cover_art")
	q.Add("order[followedCount]", "desc")
	var r mdResp
	if e := mdGet(ctx, "/manga?"+q.Encode(), &r); e != nil {
		return 0, e
	}
	n := 0
	for _, x := range r.Data {
		title := x.Attributes.Title["en"]
		if title == "" {
			for _, v := range x.Attributes.Title {
				title = v
				break
			}
		}
		var th, desc, cover *string
		if len(x.Attributes.AltTitles) > 0 {
			for _, a := range x.Attributes.AltTitles {
				if v := a["th"]; v != "" {
					th = &v
					break
				}
			}
		}
		if v := x.Attributes.Description["en"]; v != "" {
			desc = &v
		} else if v := x.Attributes.Description["th"]; v != "" {
			desc = &v
		}
		for _, rel := range x.Relationships {
			if rel.Type == "cover_art" && rel.Attributes.FileName != "" {
				v := "https://uploads.mangadex.org/covers/" + x.ID + "/" + rel.Attributes.FileName + ".256.jpg"
				cover = &v
			}
		}
		_, e := s.Pool.Exec(ctx, `insert into mangas(id,title,title_th,cover_url,original_language,content_rating,description) values($1,$2,$3,$4,$5,$6,$7) on conflict(id) do update set title=excluded.title,title_th=excluded.title_th,cover_url=excluded.cover_url,original_language=excluded.original_language,content_rating=excluded.content_rating,description=excluded.description,updated_at=now()`, x.ID, title, th, cover, x.Attributes.OriginalLanguage, x.Attributes.ContentRating, desc)
		if e != nil {
			return n, e
		}
		n++
	}
	return n, nil
}

type mdChapterResp struct {
	Data []struct {
		ID         string `json:"id"`
		Attributes struct {
			Chapter            string `json:"chapter"`
			Title              string `json:"title"`
			TranslatedLanguage string `json:"translatedLanguage"`
		} `json:"attributes"`
		Relationships []struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"relationships"`
	} `json:"data"`
}

func (s *Store) SyncChapters(ctx context.Context, mangaID string) error {
	q := url.Values{}
	q.Set("limit", "500")
	q.Add("translatedLanguage[]", "en")
	q.Add("translatedLanguage[]", "th")
	q.Set("order[chapter]", "desc")
	q.Add("includes[]", "scanlation_group")
	var r mdChapterResp
	if e := mdGet(ctx, "/manga/"+url.PathEscape(mangaID)+"/feed?"+q.Encode(), &r); e != nil {
		return e
	}
	for _, x := range r.Data {
		var group *string
		for _, rel := range x.Relationships {
			if rel.Type == "scanlation_group" {
				v := rel.ID
				group = &v
			}
		}
		var number, title *string
		if x.Attributes.Chapter != "" {
			v := x.Attributes.Chapter
			number = &v
		}
		if x.Attributes.Title != "" {
			v := x.Attributes.Title
			title = &v
		}
		_, e := s.Pool.Exec(ctx, `insert into chapters(id,manga_id,chapter_number,title,translated_language,scanlation_group,is_free) values($1,$2,$3,$4,$5,$6,true) on conflict(id) do update set chapter_number=excluded.chapter_number,title=excluded.title,translated_language=excluded.translated_language,scanlation_group=excluded.scanlation_group`, x.ID, mangaID, number, title, x.Attributes.TranslatedLanguage, group)
		if e != nil {
			return e
		}
	}
	return nil
}
func (s *Store) Pages(ctx context.Context, id string) ([]string, error) {
	var r struct {
		BaseURL string `json:"baseUrl"`
		Chapter struct {
			Hash      string   `json:"hash"`
			Data      []string `json:"data"`
			DataSaver []string `json:"dataSaver"`
		} `json:"chapter"`
	}
	if e := mdGet(ctx, "/at-home/server/"+url.PathEscape(id), &r); e != nil {
		return nil, e
	}
	out := []string{}
	for _, f := range r.Chapter.Data {
		out = append(out, strings.TrimRight(r.BaseURL, "/")+"/data/"+r.Chapter.Hash+"/"+f)
	}
	return out, nil
}
