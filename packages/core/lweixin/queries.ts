import { queryOptions } from "@tanstack/react-query";
import { api } from "../api";

/** Query key namespace for everything LWeixin-installation-related. Realtime
 * sync invalidates `installations(wsId)` on `lweixin_installation:*` events
 * so the Settings panel updates without a manual refetch. */
export const lweixinKeys = {
  all: (wsId: string) => ["lweixin", wsId] as const,
  installations: (wsId: string) => [...lweixinKeys.all(wsId), "installations"] as const,
  routing: (wsId: string, installationId: string) =>
    [...lweixinKeys.all(wsId), installationId, "routing"] as const,
  conversations: (wsId: string, installationId: string, offset: number) =>
    [...lweixinKeys.all(wsId), installationId, "conversations", offset] as const,
  messages: (wsId: string, installationId: string, conversationId: string, offset: number) =>
    [...lweixinKeys.all(wsId), installationId, "messages", conversationId, offset] as const,
};

export const lweixinInstallationsOptions = (wsId: string) =>
  queryOptions({
    queryKey: lweixinKeys.installations(wsId),
    queryFn: () => api.listLweixinInstallations(wsId),
    enabled: !!wsId,
  });

export const lweixinConversationsOptions = (wsId: string, installationId: string, offset: number) =>
  queryOptions({
    queryKey: lweixinKeys.conversations(wsId, installationId, offset),
    queryFn: () => api.listLweixinConversations(wsId, installationId, 50, offset),
    enabled: !!wsId && !!installationId,
  });

export const lweixinRoutingOptions = (wsId: string, installationId: string) =>
  queryOptions({
    queryKey: lweixinKeys.routing(wsId, installationId),
    queryFn: () => api.getLweixinRouting(wsId, installationId),
    enabled: !!wsId && !!installationId,
  });

export const lweixinMessagesOptions = (wsId: string, installationId: string, conversationId: string, offset: number) =>
  queryOptions({
    queryKey: lweixinKeys.messages(wsId, installationId, conversationId, offset),
    queryFn: () => api.listLweixinMessages(wsId, installationId, conversationId, 50, offset),
    enabled: !!wsId && !!installationId && !!conversationId,
  });
