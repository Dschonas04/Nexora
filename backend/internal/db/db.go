// Package db owns the connection pool and the schema. There is no migration
// tool: the whole schema lives in one idempotent script that runs on every boot.
package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect opens a pgx pool and waits for the database to become reachable
// (Postgres may still be starting up in a fresh Docker Compose stack).
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	// pgxpool.NewWithConfig does not dial, so a bad address only surfaces here.
	// Retry for about 30 seconds because Postgres in a fresh Compose stack needs
	// a moment before it accepts connections.
	for i := 0; i < 15; i++ {
		if err = pool.Ping(ctx); err == nil {
			return pool, nil
		}
		time.Sleep(2 * time.Second)
	}
	pool.Close()
	return nil, err
}

// schema is written so it can run against an empty database and against one
// that is already up to date. New columns are added with ALTER TABLE ... IF NOT
// EXISTS rather than by editing the CREATE TABLE above, so existing installs
// pick them up on the next start.
const schema = `
-- pgcrypto supplies gen_random_uuid(), used as the default for every id.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS users (
	id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	email         text UNIQUE NOT NULL,
	name          text NOT NULL,
	password_hash text NOT NULL,
	created_at    timestamptz NOT NULL DEFAULT now()
);

-- The page tree is self-referential: parent_id points at another page, and the
-- cascade means purging a page takes its whole subtree with it. content holds
-- the BlockNote document as JSON; the backend never interprets it.
CREATE TABLE IF NOT EXISTS pages (
	id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	owner_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	parent_id    uuid REFERENCES pages(id) ON DELETE CASCADE,
	title        text NOT NULL DEFAULT 'Untitled',
	content      jsonb NOT NULL DEFAULT '[]'::jsonb,
	icon         text NOT NULL DEFAULT '',
	is_public    boolean NOT NULL DEFAULT false,
	public_token text UNIQUE,
	sort_order   double precision NOT NULL DEFAULT 0,
	created_at   timestamptz NOT NULL DEFAULT now(),
	updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS pages_owner_idx  ON pages(owner_id);
CREATE INDEX IF NOT EXISTS pages_parent_idx ON pages(parent_id);

-- Tags are per user, not global, hence the UNIQUE over (owner_id, name).
CREATE TABLE IF NOT EXISTS tags (
	id       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	name     text NOT NULL,
	color    text NOT NULL DEFAULT '#6b7280',
	UNIQUE (owner_id, name)
);

CREATE TABLE IF NOT EXISTS page_tags (
	page_id uuid REFERENCES pages(id) ON DELETE CASCADE,
	tag_id  uuid REFERENCES tags(id)  ON DELETE CASCADE,
	PRIMARY KEY (page_id, tag_id)
);

CREATE TABLE IF NOT EXISTS favorites (
	user_id uuid REFERENCES users(id)  ON DELETE CASCADE,
	page_id uuid REFERENCES pages(id)  ON DELETE CASCADE,
	PRIMARY KEY (user_id, page_id)
);

-- Roles: 'admin' can see and edit every page; 'user' is the default.
ALTER TABLE users ADD COLUMN IF NOT EXISTS role text NOT NULL DEFAULT 'user';

-- Spaces group pages into top-level workspaces (beyond page nesting).
CREATE TABLE IF NOT EXISTS spaces (
	id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	owner_id   uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	name       text NOT NULL,
	created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS spaces_owner_idx ON spaces(owner_id);

-- A public space is visible to every logged in account of the instance without
-- anybody having to grant a right one by one.
--   'nein', only the owner and those explicitly entitled
--   'lesen', everybody logged in may read
--   'schreiben', everybody logged in may read and edit
-- Deliberately not "public on the internet": anonymous access still runs
-- exclusively through the share link of a single page.
-- Without a CHECK, because ALTER TABLE ... ADD CONSTRAINT has no IF NOT EXISTS
-- and this script runs on every start. The allowed values are enforced by the
-- handler: what it does not know becomes 'nein', so a typo can open nothing.
ALTER TABLE spaces ADD COLUMN IF NOT EXISTS oeffentlich text NOT NULL DEFAULT 'nein';
CREATE INDEX IF NOT EXISTS spaces_oeffentlich_idx ON spaces(oeffentlich) WHERE oeffentlich <> 'nein';

-- Deleting a space keeps its pages and only clears their space_id.
ALTER TABLE pages ADD COLUMN IF NOT EXISTS space_id   uuid REFERENCES spaces(id) ON DELETE SET NULL;
-- deleted_at is the trash: NULL means live, a timestamp means deleted but
-- recoverable. Every page query has to filter on it.
ALTER TABLE pages ADD COLUMN IF NOT EXISTS deleted_at timestamptz;
CREATE INDEX IF NOT EXISTS pages_space_idx   ON pages(space_id);
CREATE INDEX IF NOT EXISTS pages_deleted_idx ON pages(deleted_at);

-- Where a space sits in the sidebar, per account.
--
-- Deliberately NOT a column on spaces: the sidebar is personal, and a space
-- opened to the whole instance stands in everybody's list. A shared column
-- would mean that whoever drags it last decides the order for all the others,
-- including in workspaces they cannot even see.
--
-- Spaces without an entry sort after the ordered ones, by name -- that is
-- where a newly created space appears until somebody drags it.
CREATE TABLE IF NOT EXISTS space_reihenfolge (
	user_id  uuid NOT NULL REFERENCES users(id)  ON DELETE CASCADE,
	space_id uuid NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
	platz    integer NOT NULL,
	PRIMARY KEY (user_id, space_id)
);

-- Immutable snapshots of a page, written on save (coalesced).
CREATE TABLE IF NOT EXISTS page_versions (
	id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	page_id    uuid NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
	title      text NOT NULL,
	content    jsonb NOT NULL DEFAULT '[]'::jsonb,
	icon       text NOT NULL DEFAULT '',
	author_id  uuid REFERENCES users(id) ON DELETE SET NULL,
	created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS page_versions_idx ON page_versions(page_id, created_at DESC);

-- File attachments stored on disk (path = attachments/<id>), metadata here.
CREATE TABLE IF NOT EXISTS attachments (
	id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	page_id    uuid NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
	owner_id   uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	filename   text NOT NULL,
	mime       text NOT NULL DEFAULT 'application/octet-stream',
	size       bigint NOT NULL DEFAULT 0,
	created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS attachments_page_idx ON attachments(page_id);

-- Explicit per-user sharing with a read/edit permission.
CREATE TABLE IF NOT EXISTS page_shares (
	page_id    uuid REFERENCES pages(id)  ON DELETE CASCADE,
	user_id    uuid REFERENCES users(id)  ON DELETE CASCADE,
	permission text NOT NULL DEFAULT 'read',
	created_at timestamptz NOT NULL DEFAULT now(),
	PRIMARY KEY (page_id, user_id)
);
CREATE INDEX IF NOT EXISTS page_shares_user_idx ON page_shares(user_id);

-- Manual page-to-page links, edited via the UI (independent of [[wiki-links]]
-- written into the text). Feed the knowledge graph and backlinks like wiki-links.
CREATE TABLE IF NOT EXISTS page_links (
	source_id  uuid NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
	target_id  uuid NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
	created_at timestamptz NOT NULL DEFAULT now(),
	PRIMARY KEY (source_id, target_id)
);
CREATE INDEX IF NOT EXISTS page_links_target_idx ON page_links(target_id);

-- Full text search.
--
-- content is BlockNote JSON. The earlier search ran ILIKE over its raw text and
-- therefore also hit key names and block ids, could use no index (leading %) and
-- knew no ranking. So the plain running text is stored in content_text on save
-- and a tsvector is generated from it.
--
-- The column is GENERATED: it cannot go stale, no matter which way a row is
-- written. The title weighs more with weight A than the running text with B, so
-- that a page carrying the search term in its title stands before one that only
-- mentions it.
--
-- The dictionary is 'german'. That costs a little precision on English content
-- but is clearly better than 'simple' for German language pages: the search then
-- also reaches across word forms.
ALTER TABLE pages ADD COLUMN IF NOT EXISTS content_text text NOT NULL DEFAULT '';
ALTER TABLE pages ADD COLUMN IF NOT EXISTS such_tsv tsvector
	GENERATED ALWAYS AS (
		setweight(to_tsvector('german', coalesce(title, '')), 'A') ||
		setweight(to_tsvector('german', coalesce(content_text, '')), 'B')
	) STORED;
CREATE INDEX IF NOT EXISTS pages_such_idx ON pages USING GIN (such_tsv);

-- Audit trail: who did what and when.
--
-- Deliberately a table of its own instead of columns on the objects: an entry
-- has to survive even when the page is deleted, and deleting is precisely the
-- event a review wants to see. So there is NO foreign key with ON DELETE CASCADE
-- here; objekt_id is a loose reference, and objekt_titel records what the object
-- was called back then.
--
-- akteur_id also refers loosely to users: a deleted account must not take its
-- trail with it. akteur_name freezes the name at that moment.
CREATE TABLE IF NOT EXISTS pruefspur (
	id           bigserial PRIMARY KEY,
	zeitpunkt    timestamptz NOT NULL DEFAULT now(),
	akteur_id    uuid,
	akteur_name  text NOT NULL DEFAULT '',
	akteur_email text NOT NULL DEFAULT '',
	aktion       text NOT NULL,
	objekt_art   text NOT NULL DEFAULT '',
	objekt_id    text NOT NULL DEFAULT '',
	objekt_titel text NOT NULL DEFAULT '',
	details      jsonb NOT NULL DEFAULT '{}'::jsonb,
	ip           text NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS pruefspur_zeit_idx   ON pruefspur(zeitpunkt DESC);
CREATE INDEX IF NOT EXISTS pruefspur_akteur_idx ON pruefspur(akteur_id, zeitpunkt DESC);
CREATE INDEX IF NOT EXISTS pruefspur_objekt_idx ON pruefspur(objekt_art, objekt_id, zeitpunkt DESC);
CREATE INDEX IF NOT EXISTS pruefspur_aktion_idx ON pruefspur(aktion, zeitpunkt DESC);

-- Comments on a page.
--
-- Replies hang on the parent comment through eltern_id, but only one level deep:
-- a thread with arbitrarily deep nesting becomes unreadable, and the second level
-- covers what people actually do, which is reply to a post. The limit is enforced
-- by the handler, not by the schema.
--
-- geloescht_am instead of DELETE: a deleted comment with replies hanging on it
-- would otherwise take them along. The text is emptied on deletion, the shell
-- stays so that the thread holds together.
CREATE TABLE IF NOT EXISTS kommentare (
	id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	page_id      uuid NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
	eltern_id    uuid REFERENCES kommentare(id) ON DELETE CASCADE,
	autor_id     uuid REFERENCES users(id) ON DELETE SET NULL,
	autor_name   text NOT NULL DEFAULT '',
	text         text NOT NULL,
	erledigt     boolean NOT NULL DEFAULT false,
	erstellt_am  timestamptz NOT NULL DEFAULT now(),
	geaendert_am timestamptz,
	geloescht_am timestamptz
);
CREATE INDEX IF NOT EXISTS kommentare_page_idx   ON kommentare(page_id, erstellt_am);
CREATE INDEX IF NOT EXISTS kommentare_eltern_idx ON kommentare(eltern_id);

-- The inbox: what was addressed to an account since it last looked.
--
-- Without this table a comment reached nobody. It stood under a page and waited
-- for somebody to open it again by chance, and a question that goes unanswered
-- for a week is no longer a question.
--
-- The details about who triggered it are copies and not references. An entry has
-- to stay readable when the account that triggered it was deleted; the same
-- argument as in the audit trail.
--
-- The page reference on the other hand hangs on the page: when it disappears for
-- good, the message about it disappears too. An inbox entry leading nowhere would
-- be nothing but a nuisance.
CREATE TABLE IF NOT EXISTS postfach (
	id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	empfaenger_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	art           text NOT NULL,
	page_id       uuid REFERENCES pages(id) ON DELETE CASCADE,
	kommentar_id  uuid REFERENCES kommentare(id) ON DELETE CASCADE,
	ausloeser_id  uuid REFERENCES users(id) ON DELETE SET NULL,
	ausloeser_name text NOT NULL DEFAULT '',
	seiten_titel  text NOT NULL DEFAULT '',
	text          text NOT NULL DEFAULT '',
	gelesen_am    timestamptz,
	erstellt_am   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS postfach_empfaenger_idx ON postfach(empfaenger_id, erstellt_am DESC);
-- Partial index: almost every query asks only for the unread, and that is the
-- small part of the table.
CREATE INDEX IF NOT EXISTS postfach_ungelesen_idx ON postfach(empfaenger_id) WHERE gelesen_am IS NULL;

-- Settings that are changed at runtime through the interface.
--
-- They do NOT live in config.conf, because nobody can change a file inside the
-- container from the browser. The other way round, values like the database
-- address do not belong here: they are needed before the database is open.
--
-- The split is therefore not arbitrary: here stands what may change while
-- running, in config.conf stands what has to be fixed at startup.
--
-- geaendert_von records who wrote last; the same information also stands in the
-- audit trail, here it is merely at hand for the display.
-- Full text inside attachments.
--
-- The text is won during the upload and stored here, not fetched from the file
-- again on every search: reading a PDF costs tenths of a second, and a search
-- across a hundred attachments would be unusably slow that way.
--
-- The same procedure as with the pages: a GENERATED column that cannot go stale,
-- plus a GIN index. The file name weighs more with A than the content with B;
-- whoever searches for "offer" usually means the file called that.
ALTER TABLE attachments ADD COLUMN IF NOT EXISTS inhalt_text text NOT NULL DEFAULT '';
ALTER TABLE attachments ADD COLUMN IF NOT EXISTS such_tsv tsvector
	GENERATED ALWAYS AS (
		setweight(to_tsvector('german', coalesce(filename, '')), 'A') ||
		setweight(to_tsvector('german', coalesce(inhalt_text, '')), 'B')
	) STORED;
CREATE INDEX IF NOT EXISTS attachments_such_idx ON attachments USING GIN (such_tsv);

-- Vorlagen gab es einmal: ein Haken an einer gewoehnlichen Seite, die damit im
-- Baum zwischen dem echten Inhalt lag. Die Funktion ist entfernt, und mit ihr
-- die Spalte -- die Seiten selbst bleiben unangetastet, sie sind wieder
-- gewoehnliche Seiten.
DROP INDEX IF EXISTS pages_vorlage_idx;
ALTER TABLE pages DROP COLUMN IF EXISTS ist_vorlage;

-- Groups and space rights.
--
-- Why at all: shares per page do not scale. Whoever wants to let fourteen
-- colleagues into an area clicks fourteen times per page today. A group is the
-- answer to that, and the space is the level on which it is granted.
--
-- Groups are deliberately NOT per account but for the whole instance: a
-- department is not a private matter, and two people meaning the same group
-- should mean the same one.
CREATE TABLE IF NOT EXISTS gruppen (
	id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	name         text UNIQUE NOT NULL,
	beschreibung text NOT NULL DEFAULT '',
	erstellt_am  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS gruppen_mitglieder (
	gruppe_id uuid NOT NULL REFERENCES gruppen(id) ON DELETE CASCADE,
	user_id   uuid NOT NULL REFERENCES users(id)   ON DELETE CASCADE,
	seit      timestamptz NOT NULL DEFAULT now(),
	PRIMARY KEY (gruppe_id, user_id)
);
CREATE INDEX IF NOT EXISTS gruppen_mitglieder_user_idx ON gruppen_mitglieder(user_id);

-- A right applies either to a group or to a single account, never to both. The
-- CHECK enforces that in the schema instead of leaving it to the handler: a row
-- with both or with neither could not be evaluated, and the database is the only
-- place where it is guaranteed never to come into being.
--
-- recht is a ladder: lesen < schreiben < verwalten. Whoever may verwalten grants
-- rights for this space, which is the person responsible for it, without needing
-- a global role for it.
CREATE TABLE IF NOT EXISTS space_rechte (
	space_id  uuid NOT NULL REFERENCES spaces(id)  ON DELETE CASCADE,
	gruppe_id uuid          REFERENCES gruppen(id) ON DELETE CASCADE,
	user_id   uuid          REFERENCES users(id)   ON DELETE CASCADE,
	recht     text NOT NULL DEFAULT 'lesen',
	erteilt_am timestamptz NOT NULL DEFAULT now(),
	CHECK ((gruppe_id IS NULL) <> (user_id IS NULL)),
	CHECK (recht IN ('lesen', 'schreiben', 'verwalten'))
);
CREATE UNIQUE INDEX IF NOT EXISTS space_rechte_gruppe_idx
	ON space_rechte(space_id, gruppe_id) WHERE gruppe_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS space_rechte_user_idx
	ON space_rechte(space_id, user_id) WHERE user_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS sitzungen (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    angelegt_am   timestamptz NOT NULL DEFAULT now(),
    zuletzt_am    timestamptz NOT NULL DEFAULT now(),
    laeuft_ab     timestamptz NOT NULL,
    widerrufen_am timestamptz,
    ip            text NOT NULL DEFAULT '',
    browser       text NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS sitzungen_user_idx ON sitzungen(user_id);
-- The cleanup query looks for expired rows; without the index it runs over
-- everything, including the many valid ones.
CREATE INDEX IF NOT EXISTS sitzungen_ablauf_idx ON sitzungen(laeuft_ab);

-- Wie breit der Text einer Seite steht: 'normal', 'breit' oder 'voll'.
--
-- An der Seite und nicht am Konto: eine Tabelle mit zwoelf Spalten braucht die
-- Breite, ein Merkzettel nicht, und beide liegen im selben Wiki. Wer sie
-- umstellt, stellt sie fuer alle um, die diese Seite lesen -- das ist gewollt,
-- denn die Breite gehoert zum Satz des Textes wie eine Ueberschrift.
ALTER TABLE pages ADD COLUMN IF NOT EXISTS breite text NOT NULL DEFAULT 'normal';

-- Der Benutzername ist der zweite Weg an der Anmeldung: wer sich seine Adresse
-- nicht merken mag, tippt ihn statt ihrer. Er darf leer bleiben, deshalb keine
-- NOT-NULL-Spalte -- ein Konto aus SSO oder aus einer alten Fassung hat unter
-- Umstaenden keinen, und ohne ihn muss die Anmeldung ueber die Adresse weiter
-- gehen.
--
-- Eindeutig ohne Ruecksicht auf Gross- und Kleinschreibung: sonst waeren Anna
-- und anna zwei Konten, und an der Anmeldung koennte niemand sagen, welches
-- gemeint ist. NULL faellt aus dem Index heraus, beliebig viele Konten duerfen
-- also ohne Namen bleiben.
ALTER TABLE users ADD COLUMN IF NOT EXISTS benutzername text;
CREATE UNIQUE INDEX IF NOT EXISTS benutzer_name_einmalig
	ON users (lower(benutzername)) WHERE benutzername IS NOT NULL;

-- Bestandskonten bekommen einen Namen aus dem vorderen Teil ihrer Adresse:
-- sonst haette nach der Umstellung nur wer sich neu anmeldet einen, und der
-- neue Weg waere fuer alle anderen zu.
--
-- Was sich dabei doppeln wuerde, bleibt leer statt zu raten. Zwei Adressen bei
-- verschiedenen Anbietern koennen denselben vorderen Teil haben, und dann ist
-- anna@a.de nicht mehr "anna" als anna@b.de; wer von beiden ihn bekommt, waere
-- Zufall. Diese Konten melden sich weiter mit ihrer Adresse an und koennen
-- ihren Namen selbst setzen.
UPDATE users u SET benutzername = k.name
FROM (
	SELECT id,
	       regexp_replace(lower(split_part(email, '@', 1)), '[^a-z0-9._-]', '', 'g') AS name,
	       count(*) OVER (
	           PARTITION BY regexp_replace(lower(split_part(email, '@', 1)), '[^a-z0-9._-]', '', 'g')
	       ) AS wieoft
	FROM users
	WHERE benutzername IS NULL
) k
WHERE u.id = k.id
  AND k.wieoft = 1
  AND length(k.name) BETWEEN 3 AND 32
  AND NOT EXISTS (SELECT 1 FROM users x WHERE lower(x.benutzername) = k.name);

CREATE TABLE IF NOT EXISTS einstellungen (
	schluessel    text PRIMARY KEY,
	wert          text NOT NULL,
	geaendert_am  timestamptz NOT NULL DEFAULT now(),
	geaendert_von text NOT NULL DEFAULT ''
);

-- Rechner, die diese Instanz im Blick behalten soll. Reine Nachschlageliste:
-- geprueft wird bei jedem Aufruf neu, gespeichert wird nur, WAS zu pruefen ist.
-- Kein Zustand in der Tabelle, damit ein Neustart nichts Falsches behauptet.
CREATE TABLE IF NOT EXISTS rechner (
	id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	name         text NOT NULL,
	ziel         text NOT NULL,
	notiz        text NOT NULL DEFAULT '',
	-- Die Kennung, unter der Prometheus den Rechner fuehrt (label instance).
	-- Leer heisst: aus dem Ziel erraten, was in aller Regel stimmt.
	instanz      text NOT NULL DEFAULT '',
	reihenfolge  int NOT NULL DEFAULT 0,
	angelegt_am  timestamptz NOT NULL DEFAULT now()
);
`

// Migrate applies the schema. It is idempotent and safe to run on every start,
// which is exactly what main does.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, schema)
	return err
}
