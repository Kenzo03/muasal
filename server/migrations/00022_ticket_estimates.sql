-- +goose Up
-- MSL-54: an optional effort estimate in hours, summed per person on Workload.
ALTER TABLE tickets ADD COLUMN estimate_hours double precision
  CONSTRAINT tickets_estimate_range CHECK (estimate_hours >= 0 AND estimate_hours <= 9999);

-- +goose Down
ALTER TABLE tickets DROP COLUMN estimate_hours;
