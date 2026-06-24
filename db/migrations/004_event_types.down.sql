DROP INDEX IF EXISTS idx_events_event_type;
ALTER TABLE endpoints DROP COLUMN IF EXISTS allowed_event_types;
ALTER TABLE events DROP COLUMN IF EXISTS event_type;
