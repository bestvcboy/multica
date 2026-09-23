CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_lweixin_routing_policy_installation
    ON lweixin_routing_policy (installation_id, account_id);
