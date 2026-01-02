-- +goose Up
-- +goose StatementBegin

-----------------------------------------------------------
-- 5. EVENT RULES
-- Dynamic logic for event-driven actions within the TMS.
-----------------------------------------------------------
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE TABLE event_rules
(
    id            UUID PRIMARY KEY      DEFAULT uuid_generate_v4(),

    -- Kafka Topic or Category (e.g., "users", "teams", "loads")
    topic         VARCHAR(255) NOT NULL,

    -- Filter for the specific event (e.g., "USER_CREATED", "TEAM_DISPATCHER_CHANGED")
    event_type    VARCHAR(255) NOT NULL,

    -- JSONB for structured logic (e.g., {"role": "admin", "region": "US"})
    conditions    JSONB        NULL,

    -- The internal handler identifier (e.g., "SEND_EMAIL", "NOTIFY_SLACK")
    action_type   VARCHAR(255) NOT NULL,

    -- Configuration for the action (e.g., {"template_id": "welcome_01"})
    action_config JSONB        NULL,

    is_active     BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMP    NOT NULL DEFAULT NOW()
);

-- Indexing for fast retrieval during event processing
CREATE INDEX idx_event_rules_lookup ON event_rules (topic, event_type) WHERE is_active = TRUE;

-- GIN index to allow efficient querying inside the JSONB conditions
CREATE INDEX idx_event_rules_conditions_gin ON event_rules USING GIN (conditions);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS event_rules;
-- +goose StatementEnd