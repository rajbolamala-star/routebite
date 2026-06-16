CREATE TABLE IF NOT EXISTS agent_searches (
  id BIGSERIAL PRIMARY KEY,
  request_id TEXT NOT NULL,
  query TEXT NOT NULL DEFAULT '',
  start_location TEXT NOT NULL DEFAULT '',
  destination TEXT NOT NULL DEFAULT '',
  preference TEXT NOT NULL DEFAULT '',
  max_detour_minutes INTEGER NOT NULL DEFAULT 0,
  result_count INTEGER NOT NULL DEFAULT 0,
  summary TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_agent_searches_created_at
  ON agent_searches (created_at DESC);

CREATE INDEX IF NOT EXISTS idx_agent_searches_request_id
  ON agent_searches (request_id);
