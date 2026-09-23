CREATE TABLE lweixin_routing_policy (
    workspace_id UUID NOT NULL,
    installation_id UUID NOT NULL,
    account_id TEXT NOT NULL,
    private_mode TEXT NOT NULL DEFAULT 'silent' CHECK (private_mode IN ('silent', 'agent')),
    private_agent_id UUID,
    private_revision BIGINT NOT NULL DEFAULT 0,
    group_mode TEXT NOT NULL DEFAULT 'silent' CHECK (group_mode IN ('silent', 'agent')),
    group_agent_id UUID,
    group_revision BIGINT NOT NULL DEFAULT 0,
    CHECK ((private_mode = 'agent') = (private_agent_id IS NOT NULL)),
    CHECK ((group_mode = 'agent') = (group_agent_id IS NOT NULL))
);

ALTER TABLE lweixin_conversation
    ADD COLUMN route_mode TEXT NOT NULL DEFAULT 'inherit' CHECK (route_mode IN ('inherit', 'silent', 'agent')),
    ADD COLUMN route_agent_id UUID,
    ADD COLUMN route_revision BIGINT NOT NULL DEFAULT 0,
    ADD CONSTRAINT lweixin_route_agent_consistent CHECK ((route_mode = 'agent') = (route_agent_id IS NOT NULL));
