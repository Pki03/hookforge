ALTER TABLE events ADD COLUMN IF NOT EXISTS event_type TEXT NOT NULL DEFAULT '';
ALTER TABLE endpoints ADD COLUMN IF NOT EXISTS allowed_event_types TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_events_event_type ON events(event_type);
