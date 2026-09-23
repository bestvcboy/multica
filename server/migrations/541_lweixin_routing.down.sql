ALTER TABLE lweixin_conversation
    DROP CONSTRAINT IF EXISTS lweixin_route_agent_consistent,
    DROP COLUMN IF EXISTS route_revision,
    DROP COLUMN IF EXISTS route_agent_id,
    DROP COLUMN IF EXISTS route_mode;
DROP TABLE IF EXISTS lweixin_routing_policy;
