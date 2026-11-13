CREATE TABLE IF NOT EXISTS members (
 id BIGSERIAL PRIMARY KEY,
 email VARCHAR(255) NOT NULL UNIQUE,
 password_hash VARCHAR(255) NOT NULL,
 display_name VARCHAR(80) NOT NULL,
 role VARCHAR(10) NOT NULL DEFAULT 'member' CHECK (role IN ('member','admin')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS mangas (
 id TEXT PRIMARY KEY,
 title TEXT NOT NULL,
 title_th TEXT,
 cover_url TEXT,
 original_language VARCHAR(10),
 content_rating TEXT,
 description TEXT,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_mangas_title ON mangas USING gin (to_tsvector('simple', title));
CREATE TABLE IF NOT EXISTS chapters (
 id TEXT PRIMARY KEY,
 manga_id TEXT NOT NULL REFERENCES mangas(id) ON DELETE CASCADE,
 chapter_number TEXT,
 title TEXT,
 translated_language VARCHAR(10),
 scanlation_group TEXT,
 is_free BOOLEAN NOT NULL DEFAULT false,
 published_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_chapters_manga ON chapters(manga_id, published_at DESC);
CREATE TABLE IF NOT EXISTS bookmarks (
 member_id BIGINT NOT NULL REFERENCES members(id) ON DELETE CASCADE,
 manga_id TEXT NOT NULL REFERENCES mangas(id) ON DELETE CASCADE,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(member_id,manga_id)
);
