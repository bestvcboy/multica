import { queryOptions } from "@tanstack/react-query";
import { api } from "../api";

/** Query key namespace for everything LWeixin-installation-related. Realtime
 * sync invalidates `installations(wsId)` on `lweixin_installation:*` events
 * so the Settings panel updates without a manual refetch. */
export const lweixinKeys = {
  all: (wsId: string) => ["lweixin", wsId] as const,
  installations: (wsId: string) => [...lweixinKeys.all(wsId), "installations"] as const,
};

export const lweixinInstallationsOptions = (wsId: string) =>
  queryOptions({
    queryKey: lweixinKeys.installations(wsId),
    queryFn: () => api.listLweixinInstallations(wsId),
    enabled: !!wsId,
  });
