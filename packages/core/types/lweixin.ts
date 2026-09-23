/** A LWeixin (WeChat gateway) installation bound to a single Multica agent.
 *
 * Wire shape mirrors `LweixinInstallationResponse` in
 * `server/internal/handler/lweixin.go`. New fields the backend adds in
 * the future MUST default to optional so older desktop builds keep parsing
 * the response — see AGENTS.md -> API Compatibility. */
export interface LweixinInstallation {
  id: string;
  workspace_id: string;
  agent_id: string;
  /** The gateway's WeChat account id, probed from /api/status on install. */
  app_id: string;
  /** Base URL of the LWeixin gateway, e.g. https://wchat.example.com. */
  base_url: string;
  installer_user_id: string;
  status: "active" | "revoked" | string;
  installed_at: string;
  created_at: string;
  updated_at: string;
}

export interface ListLweixinInstallationsResponse {
  installations: LweixinInstallation[];
  /** Whether the deployment has the at-rest secret key configured. When false
   * the connect entry points are hidden and the panel renders an "ask the
   * operator to enable LWeixin" state. */
  configured: boolean;
  /** Optional so an older desktop build that predates it treats it as off. */
  install_supported?: boolean;
}

/** Request body for an install: the gateway base URL and admin API token.
 * The backend validates both live (/api/status) before persisting. */
export interface RegisterLweixinRequest {
  base_url: string;
  api_token: string;
  app_id?: string;
}

/** Post-redemption echo: the LWeixin user id the token carried is now bound
 * to the logged-in Multica user in this workspace/installation. */
export interface RedeemLweixinBindingTokenResponse {
  workspace_id: string;
  installation_id: string;
  lweixin_user_id: string;
}

export interface LweixinConversation {
  id: string;
  accountId: string;
  chatType: string;
  chatId: string;
  lastMessageAt: string;
  routeMode: LweixinRouteMode;
  routeAgentId: string | null;
  effectiveMode: LweixinDefaultMode;
  effectiveAgentId: string | null;
  routeRevision: number;
  triggerMode: LweixinGroupTriggerMode;
  triggerReason: LweixinGroupTriggerReason;
}

export type LweixinDefaultMode = "silent" | "agent";
export type LweixinRouteMode = "inherit" | LweixinDefaultMode;
export type LweixinGroupTriggerMode = "mention" | "all";
export type LweixinGroupTriggerReason = "mention_metadata_unavailable" | "execution_isolation_unavailable" | null;
export interface LweixinRoutePolicy {
  mode: LweixinDefaultMode;
  agentId: string | null;
}
export interface LweixinRouting {
  privateDefault: LweixinRoutePolicy;
  groupDefault: LweixinRoutePolicy;
  privateRevision: number;
  groupRevision: number;
}

export interface LweixinTextMessage {
  messageId: string;
  senderId: string;
  text: string;
  receivedAt: string;
}

export interface ListLweixinConversationsResponse {
  conversations: LweixinConversation[];
}

export interface ListLweixinMessagesResponse {
  messages: LweixinTextMessage[];
}
