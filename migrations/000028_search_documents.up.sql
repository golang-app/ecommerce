BEGIN;

-- search_document is the storage for the search bounded context's published
-- language (a Document value object). Producers (productcatalog today;
-- blog/faq later) translate their records into rows here via a small ACL
-- on their side; the search context itself does not know about products
-- or any other domain.
--
-- (kind, id) is the composite primary key: hits are uniquely identified by
-- the pair, so two producers can safely share an id namespace.
--
-- `ts` aggregates title (weight A), body (weight B) and tags (weight C)
-- into a tsvector. We use the 'simple' configuration (lowercases + tokenises
-- with no stemming) — matches the storefront's plain-substring mental model
-- for short product titles. Postgres rejects a GENERATED expression that
-- chains setweight + to_tsvector('simple', ...) as "not immutable" on its
-- conservative immutability check, so we maintain ts via a trigger instead.
CREATE TABLE public.search_document (
    kind text NOT NULL,
    id text NOT NULL,
    title text NOT NULL,
    body text NOT NULL,
    url text NOT NULL,
    tags text[] NOT NULL DEFAULT '{}',
    meta jsonb NOT NULL DEFAULT '{}'::jsonb,
    updated_at timestamptz NOT NULL DEFAULT now(),
    ts tsvector,
    PRIMARY KEY (kind, id)
);

CREATE OR REPLACE FUNCTION public.search_document_refresh_ts() RETURNS trigger AS $$
BEGIN
    NEW.ts :=
        setweight(to_tsvector('simple', coalesce(NEW.title, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(NEW.body, '')), 'B') ||
        setweight(to_tsvector('simple', coalesce(array_to_string(NEW.tags, ' '), '')), 'C');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER search_document_refresh_ts_bi
    BEFORE INSERT OR UPDATE OF title, body, tags
    ON public.search_document
    FOR EACH ROW EXECUTE FUNCTION public.search_document_refresh_ts();

CREATE INDEX search_document_ts_idx ON public.search_document USING GIN (ts);
CREATE INDEX search_document_kind_idx ON public.search_document(kind);

COMMIT;
