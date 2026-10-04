-- +goose Up
-- MSL-66: who at the client accepted a ticket's work (UAT sign-off), when,
-- and a note; both or neither of the contact and the date.
ALTER TABLE tickets
  ADD COLUMN accepted_contact_id bigint REFERENCES contacts (id),
  ADD COLUMN accepted_on date,
  ADD COLUMN acceptance_note text NOT NULL DEFAULT '' CHECK (length(acceptance_note) <= 2000),
  ADD CONSTRAINT tickets_acceptance_whole CHECK ((accepted_contact_id IS NULL) = (accepted_on IS NULL));

-- +goose Down
ALTER TABLE tickets DROP CONSTRAINT tickets_acceptance_whole,
  DROP COLUMN acceptance_note, DROP COLUMN accepted_on, DROP COLUMN accepted_contact_id;
