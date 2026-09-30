-- +goose Up
-- The ticket form's weak-reason rule (web/lib/weak.ts, R-DC-8), for the ticket
-- list and Home, which see tickets filed through the API and imports too: a
-- reason under 20 characters once stock phrases such as "permintaan klien"
-- are removed. An empty reason is missing, not weak.
-- +goose StatementBegin
CREATE FUNCTION weak_reason(reason text) RETURNS boolean
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
  SELECT btrim(reason) <> '' AND char_length(btrim(regexp_replace(
    regexp_replace(lower(reason), '\m(per request|as requested|client request|sesuai permintaan|permintaan klien|permintaan client|request user|ok|done)\M', '', 'g'),
    '\s+', ' ', 'g'))) < 20
$$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION weak_reason(text);
