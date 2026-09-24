-- +goose Up
-- The default statuses take the Terakota colors of the UI design. Statuses
-- that still wear the old default colors change too; colors an admin chose stay.
ALTER TABLE statuses ALTER COLUMN color SET DEFAULT '#7D746C';

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION insert_default_statuses(pid bigint) RETURNS void LANGUAGE sql AS $$
  INSERT INTO statuses (project_id, name, category, position, color, is_default)
  VALUES (pid, 'To do', 'todo', 0, '#7D746C', true),
         (pid, 'In progress', 'in_progress', 1, '#C0622F', false),
         (pid, 'In review', 'in_progress', 2, '#6C5BB5', false),
         (pid, 'Done', 'done', 3, '#3C7D5A', false),
         (pid, 'Cancelled', 'cancelled', 4, '#A8A097', false);
$$;
-- +goose StatementEnd

UPDATE statuses SET color = CASE color
    WHEN '#6B7280' THEN '#7D746C'
    WHEN '#2563EB' THEN '#C0622F'
    WHEN '#7C3AED' THEN '#6C5BB5'
    WHEN '#16A34A' THEN '#3C7D5A'
    ELSE '#A8A097' END
  WHERE color IN ('#6B7280', '#2563EB', '#7C3AED', '#16A34A', '#9CA3AF');

-- +goose Down
ALTER TABLE statuses ALTER COLUMN color SET DEFAULT '#6B7280';

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION insert_default_statuses(pid bigint) RETURNS void LANGUAGE sql AS $$
  INSERT INTO statuses (project_id, name, category, position, color, is_default)
  VALUES (pid, 'To do', 'todo', 0, '#6B7280', true),
         (pid, 'In progress', 'in_progress', 1, '#2563EB', false),
         (pid, 'In review', 'in_progress', 2, '#7C3AED', false),
         (pid, 'Done', 'done', 3, '#16A34A', false),
         (pid, 'Cancelled', 'cancelled', 4, '#9CA3AF', false);
$$;
-- +goose StatementEnd

UPDATE statuses SET color = CASE color
    WHEN '#7D746C' THEN '#6B7280'
    WHEN '#C0622F' THEN '#2563EB'
    WHEN '#6C5BB5' THEN '#7C3AED'
    WHEN '#3C7D5A' THEN '#16A34A'
    ELSE '#9CA3AF' END
  WHERE color IN ('#7D746C', '#C0622F', '#6C5BB5', '#3C7D5A', '#A8A097');
