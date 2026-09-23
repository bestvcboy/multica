"use client";

import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { ChevronRight, ExternalLink, Trash2 } from "lucide-react";
import { LweixinMark } from "./lweixin-mark";
import { cn } from "@multica/ui/lib/utils";
import { Button } from "@multica/ui/components/ui/button";
import { Card, CardContent } from "@multica/ui/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@multica/ui/components/ui/alert-dialog";
import { useAuthStore } from "@multica/core/auth";
import { useWorkspaceId } from "@multica/core/hooks";
import { memberListOptions } from "@multica/core/workspace/queries";
import { agentListOptions } from "@multica/core/workspace/queries";
import { useActorName } from "@multica/core/workspace/hooks";
import { lweixinConversationsOptions, lweixinInstallationsOptions, lweixinKeys, lweixinMessagesOptions, lweixinRoutingOptions } from "@multica/core/lweixin";
import { api } from "@multica/core/api";
import type { LweixinInstallation } from "@multica/core/types";
import type { LweixinGroupTriggerMode, LweixinGroupTriggerReason, LweixinRouteMode } from "@multica/core/types/lweixin";
import { ActorAvatar } from "../../common/actor-avatar";
import { openExternal } from "../../platform";
import { useLocale, useT } from "../../i18n";

// LweixinTab is the workspace settings panel for LWeixin gateway
// installations, mirroring TelegramTab: listing is member-visible; the
// disconnect action is admin-only (backend-enforced; the UI hides the button
// to match). Unlike Telegram's paste-a-token flow, install requires the
// gateway base URL plus an admin API token — the backend probes /api/status
// live to resolve the gateway's WeChat account id.
export function LweixinTab() {
  const { t } = useT("settings");
  const wsId = useWorkspaceId();
  const qc = useQueryClient();
  const user = useAuthStore((s) => s.user);

  const { data: members = [] } = useQuery(memberListOptions(wsId));
  const currentMember = members.find((m) => m.user_id === user?.id) ?? null;
  const canManage =
    currentMember?.role === "owner" || currentMember?.role === "admin";

  const { data, isLoading, isError } = useQuery({
    ...lweixinInstallationsOptions(wsId),
    enabled: !!wsId,
  });
  const installations = data?.installations ?? [];
  const configured = data?.configured === true;

  const [disconnectTarget, setDisconnectTarget] = useState<string | null>(null);
  const [disconnecting, setDisconnecting] = useState(false);
  const [selectedInstallation, setSelectedInstallation] = useState<string | null>(null);

  async function handleDisconnect() {
    if (!disconnectTarget || disconnecting) return;
    setDisconnecting(true);
    try {
      // Await the server before touching cache/UI (repo rule: no optimistic
      // removal on flows that confirm/destroy).
      await api.deleteLweixinInstallation(wsId, disconnectTarget);
      await qc.invalidateQueries({ queryKey: lweixinKeys.installations(wsId) });
      toast.success(t(($) => $.lweixin.toast_disconnected));
      if (selectedInstallation === disconnectTarget) setSelectedInstallation(null);
      setDisconnectTarget(null);
    } catch (e) {
      toast.error(
        e instanceof Error ? e.message : t(($) => $.lweixin.toast_disconnect_failed),
      );
    } finally {
      setDisconnecting(false);
    }
  }

  return (
    <div className="space-y-8">
      {isError ? (
        <Card>
          <CardContent>
            <p className="text-body text-muted-foreground">
              {t(($) => $.lweixin.load_failed)}
            </p>
          </CardContent>
        </Card>
      ) : isLoading ? (
        <Card>
          <CardContent>
            <p className="text-body text-muted-foreground">{t(($) => $.lweixin.loading)}</p>
          </CardContent>
        </Card>
      ) : !configured ? (
        <Card>
          <CardContent className="space-y-2">
            <p className="text-body font-medium">{t(($) => $.lweixin.not_enabled_title)}</p>
            <p className="text-caption text-muted-foreground">
              {t(($) => $.lweixin.not_enabled_description_prefix)}{" "}
              <code className="rounded-xs bg-muted px-1 py-0.5 text-micro">
                MULTICA_LWEIXIN_SECRET_KEY
              </code>{" "}
              {t(($) => $.lweixin.not_enabled_description_suffix)}{" "}
              {t(($) => $.lweixin.not_enabled_self_host_hint)}
            </p>
          </CardContent>
        </Card>
      ) : (
        <section className="space-y-3">
          <h2 className="text-body font-semibold">
            {t(($) => $.lweixin.connected_accounts)}
          </h2>
          {installations.length === 0 ? (
            <Card>
              <CardContent className="space-y-2">
                <p className="text-body font-medium">{t(($) => $.lweixin.empty_title)}</p>
                <p className="text-caption text-muted-foreground">
                  {t(($) => $.lweixin.empty_description_prefix)}{" "}
                  <strong>{t(($) => $.lweixin.empty_description_cta)}</strong>{" "}
                  {t(($) => $.lweixin.empty_description_suffix)}
                </p>
              </CardContent>
            </Card>
          ) : (
            <Card>
              <CardContent className="divide-y">
                {installations.map((inst) => (
                  <InstallationRow
                    key={inst.id}
                    installation={inst}
                    canManage={canManage}
                    onDisconnect={() => setDisconnectTarget(inst.id)}
                    onManage={() => setSelectedInstallation(selectedInstallation === inst.id ? null : inst.id)}
                  />
                ))}
              </CardContent>
            </Card>
          )}
          {canManage && selectedInstallation && (
            <LweixinConversations key={selectedInstallation} wsId={wsId} installationId={selectedInstallation} />
          )}
        </section>
      )}

      <AlertDialog
        open={!!disconnectTarget}
        onOpenChange={(v) => {
          if (!v && !disconnecting) setDisconnectTarget(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t(($) => $.lweixin.disconnect_confirm_title)}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t(($) => $.lweixin.disconnect_confirm_description)}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={disconnecting}>
              {t(($) => $.lweixin.disconnect_confirm_cancel)}
            </AlertDialogCancel>
            <AlertDialogAction onClick={handleDisconnect} disabled={disconnecting}>
              {disconnecting
                ? t(($) => $.lweixin.disconnecting)
                : t(($) => $.lweixin.disconnect)}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

function InstallationRow({
  installation,
  canManage,
  onDisconnect,
  onManage,
}: {
  installation: LweixinInstallation;
  canManage: boolean;
  onDisconnect: () => void;
  onManage: () => void;
}) {
  const { t } = useT("settings");
  const locale = useLocale();
  const { getAgentName } = useActorName();
  const isActive = installation.status === "active";
  const agentName = getAgentName(installation.agent_id);
  return (
    <div className="flex items-start justify-between gap-4 py-3 first:pt-0 last:pb-0">
      <div className="flex items-start gap-3">
        <ActorAvatar
          actorType="agent"
          actorId={installation.agent_id}
          size="lg"
          enableHoverCard
          profileLink
        />
        <div className="space-y-1">
          <p className="text-body font-medium">
            {agentName}
            {installation.app_id ? (
              <span className="ml-2 text-caption text-muted-foreground">
                {installation.app_id}
              </span>
            ) : null}
            {!isActive && (
              <span className="ml-2 rounded-xs bg-muted px-1.5 py-0.5 text-micro text-muted-foreground">
                {t(($) => $.lweixin.revoked_badge)}
              </span>
            )}
          </p>
          <p className="text-micro text-muted-foreground">
            {t(($) => $.lweixin.installed_at_label, {
              when: new Date(installation.installed_at).toLocaleString(locale),
            })}
          </p>
        </div>
      </div>
      {canManage && (
        <div className="flex shrink-0 gap-2">
          <Button variant="outline" size="sm" onClick={onManage}>
            {t(($) => $.lweixin.routing.conversations)}
          </Button>
          {isActive && (
            <Button variant="outline" size="sm" onClick={onDisconnect}>
              <Trash2 className="h-3 w-3" />
              {t(($) => $.lweixin.disconnect)}
            </Button>
          )}
        </div>
      )}
    </div>
  );
}

function LweixinConversations({ wsId, installationId }: { wsId: string; installationId: string }) {
  const { t } = useT("settings");
  const qc = useQueryClient();
  const [offset, setOffset] = useState(0);
  const [selected, setSelected] = useState<string | null>(null);
  const [messageOffset, setMessageOffset] = useState(0);
  const { data, isLoading, isError } = useQuery(lweixinConversationsOptions(wsId, installationId, offset));
  const { data: routing, isLoading: routingLoading, isError: routingError } = useQuery(lweixinRoutingOptions(wsId, installationId));
  const { data: agents = [] } = useQuery(agentListOptions(wsId));
  const { data: history, isLoading: historyLoading, isError: historyError } = useQuery({
    ...lweixinMessagesOptions(wsId, installationId, selected ?? "", messageOffset),
    enabled: !!selected,
  });
  const conversations = data?.conversations ?? [];
  const selectedConversation = conversations.find((conversation) => conversation.id === selected);
  const messages = history?.messages ?? [];
  return (
    <div className="space-y-5 border-t pt-4">
      <section className="space-y-3">
        <h3 className="text-body font-semibold">{t(($) => $.lweixin.routing.private_default)}</h3>
        {routingLoading ? <p>{t(($) => $.lweixin.loading)}</p> : routingError ? (
          <p role="alert">{t(($) => $.lweixin.routing.load_failed)}</p>
        ) : routing && (
          <RouteEditor
            key={`default-${routing.privateRevision}`}
            mode={routing.privateDefault.mode}
            agentId={routing.privateDefault.agentId}
            agents={agents}
            onSave={async (mode, agentId) => {
              await api.updateLweixinRouting(wsId, installationId, { mode: mode === "agent" ? "agent" : "silent", agentId });
              await qc.invalidateQueries({ queryKey: lweixinKeys.routing(wsId, installationId) });
              await qc.invalidateQueries({ queryKey: [...lweixinKeys.all(wsId), installationId, "conversations"] });
            }}
          />
        )}
        <p className="text-caption text-muted-foreground">{t(($) => $.lweixin.routing.dispatch_pending)}</p>
      </section>
      <section className="space-y-3">
        <h3 className="text-body font-semibold">{t(($) => $.lweixin.routing.group_default)}</h3>
        {routingLoading ? <p>{t(($) => $.lweixin.loading)}</p> : routingError ? (
          <p role="alert">{t(($) => $.lweixin.routing.load_failed)}</p>
        ) : routing && (
          <RouteEditor
            key={`group-default-${routing.groupRevision}`}
            mode={routing.groupDefault.mode}
            agentId={routing.groupDefault.agentId}
            agents={agents}
            onSave={async (mode, agentId) => {
              await api.updateLweixinGroupRouting(wsId, installationId, { mode: mode === "agent" ? "agent" : "silent", agentId });
              await qc.invalidateQueries({ queryKey: lweixinKeys.routing(wsId, installationId) });
              await qc.invalidateQueries({ queryKey: [...lweixinKeys.all(wsId), installationId, "conversations"] });
            }}
          />
        )}
      </section>
      <div className="grid gap-4 lg:grid-cols-2">
      <section className="min-w-0 space-y-3">
        <h3 className="text-body font-semibold">{t(($) => $.lweixin.routing.conversations)}</h3>
        {isLoading ? <p>{t(($) => $.lweixin.loading)}</p> : isError ? (
          <p role="alert">{t(($) => $.lweixin.routing.load_failed)}</p>
        ) : conversations.length === 0 ? (
          <p className="text-caption text-muted-foreground">{t(($) => $.lweixin.routing.no_conversations)}</p>
        ) : conversations.map((conversation) => (
          <button
            key={conversation.id}
            type="button"
            className={cn("block w-full min-w-0 border-b px-2 py-2 text-left text-body hover:bg-muted", selected === conversation.id && "bg-muted font-medium")}
            aria-pressed={selected === conversation.id}
            title={conversation.chatId}
            onClick={() => { setSelected(conversation.id); setMessageOffset(0); }}
          >
            <span className="block truncate">{conversation.chatId}</span>
            <span className="text-micro text-muted-foreground">
              {conversation.chatType === "group" ? t(($) => $.lweixin.routing.group_chat) : t(($) => $.lweixin.routing.direct_chat)}
              {" · "}
              {conversation.routeMode === "inherit" ? t(($) => $.lweixin.routing.inherit) :
                conversation.routeMode === "agent" ? t(($) => $.lweixin.routing.agent) : t(($) => $.lweixin.routing.silent)}
            </span>
          </button>
        ))}
        <div className="flex gap-2">
          <Button variant="outline" size="sm" disabled={offset === 0} onClick={() => { setSelected(null); setOffset(Math.max(0, offset - 50)); }}>{t(($) => $.lweixin.routing.previous)}</Button>
          <Button variant="outline" size="sm" disabled={conversations.length < 50} onClick={() => { setSelected(null); setOffset(offset + 50); }}>{t(($) => $.lweixin.routing.next)}</Button>
        </div>
      </section>
      <section className="min-w-0 space-y-3">
        <h3 className="text-body font-semibold">{t(($) => $.lweixin.routing.history)}</h3>
        {selectedConversation?.chatType === "p2p" && (
          <div className="space-y-2 border-b pb-3">
            <h4 className="text-caption font-medium">{t(($) => $.lweixin.routing.friend_rule)}</h4>
            <RouteEditor
              key={`${selectedConversation.id}-${selectedConversation.routeRevision}`}
              mode={selectedConversation.routeMode}
              agentId={selectedConversation.routeAgentId}
              agents={agents}
              allowInherit
              onSave={async (mode, agentId) => {
                await api.updateLweixinConversationRoute(wsId, installationId, selectedConversation.id, mode, agentId);
                await qc.invalidateQueries({ queryKey: [...lweixinKeys.all(wsId), installationId, "conversations"] });
              }}
            />
          </div>
        )}
        {selectedConversation?.chatType === "group" && (
          <div className="space-y-2 border-b pb-3">
            <h4 className="text-caption font-medium">{t(($) => $.lweixin.routing.group_rule)}</h4>
            <RouteEditor
              key={`${selectedConversation.id}-${selectedConversation.routeRevision}-${selectedConversation.triggerMode}`}
              mode={selectedConversation.routeMode}
              agentId={selectedConversation.routeAgentId}
              agents={agents}
              allowInherit
              triggerMode={selectedConversation.triggerMode}
              triggerReason={selectedConversation.triggerReason}
              onSave={async (mode, agentId, triggerMode) => {
                await api.updateLweixinConversationRoute(wsId, installationId, selectedConversation.id, mode, agentId, triggerMode);
                await qc.invalidateQueries({ queryKey: [...lweixinKeys.all(wsId), installationId, "conversations"] });
              }}
            />
          </div>
        )}
        {selected && (historyLoading ? <p>{t(($) => $.lweixin.loading)}</p> : historyError ? (
          <p role="alert">{t(($) => $.lweixin.routing.load_failed)}</p>
        ) : (
          <>
            <div className="max-h-96 space-y-2 overflow-y-auto">
              {messages.map((message) => (
                <div key={message.messageId} className="border-b py-2 text-body">
                  <p className="text-micro text-muted-foreground">{message.senderId} · {message.receivedAt}</p>
                  <p className="whitespace-pre-wrap break-words">{message.text}</p>
                </div>
              ))}
              {messages.length === 0 && <p className="text-caption text-muted-foreground">{t(($) => $.lweixin.routing.no_messages)}</p>}
            </div>
            <div className="flex gap-2">
              <Button variant="outline" size="sm" disabled={messageOffset === 0} onClick={() => setMessageOffset(Math.max(0, messageOffset - 50))}>{t(($) => $.lweixin.routing.previous)}</Button>
              <Button variant="outline" size="sm" disabled={messages.length < 50} onClick={() => setMessageOffset(messageOffset + 50)}>{t(($) => $.lweixin.routing.next)}</Button>
            </div>
          </>
        ))}
      </section>
      </div>
    </div>
  );
}

function RouteEditor({
  mode, agentId, agents, onSave, allowInherit = false, triggerMode, triggerReason,
}: {
  mode: LweixinRouteMode;
  agentId: string | null;
  agents: { id: string; name: string; archived_at?: string | null }[];
  onSave: (mode: LweixinRouteMode, agentId: string | null, triggerMode?: LweixinGroupTriggerMode) => Promise<void>;
  allowInherit?: boolean;
  triggerMode?: LweixinGroupTriggerMode;
  triggerReason?: LweixinGroupTriggerReason;
}) {
  const { t } = useT("settings");
  const [draftMode, setDraftMode] = useState(mode);
  const [draftAgent, setDraftAgent] = useState(agentId ?? "");
  const [draftTrigger, setDraftTrigger] = useState<LweixinGroupTriggerMode>(triggerMode ?? "mention");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const available = agents.filter((agent) => !agent.archived_at);
  const isDirty = draftMode !== mode || (draftMode === "agent" && draftAgent !== agentId) || (triggerMode !== undefined && draftTrigger !== triggerMode);
  async function save() {
    setSaving(true);
    setError("");
    try {
      await onSave(draftMode, draftMode === "agent" ? draftAgent : null, triggerMode === undefined ? undefined : draftTrigger);
    } catch (e) {
      setError(e instanceof Error ? e.message : t(($) => $.lweixin.routing.save_failed));
    } finally {
      setSaving(false);
    }
  }
  return (
    <div className="flex flex-wrap items-end gap-2">
      <label className="grid min-w-36 gap-1 text-caption">
        {t(($) => $.lweixin.routing.mode)}
        <select className="h-9 rounded-sm border bg-background px-2 text-body" value={draftMode} disabled={saving} onChange={(e) => setDraftMode(e.target.value as LweixinRouteMode)}>
          {allowInherit && <option value="inherit">{t(($) => $.lweixin.routing.inherit)}</option>}
          <option value="silent">{t(($) => $.lweixin.routing.silent)}</option>
          <option value="agent">{t(($) => $.lweixin.routing.agent)}</option>
        </select>
      </label>
      {draftMode === "agent" && (
        <label className="grid min-w-40 gap-1 text-caption">
          {t(($) => $.lweixin.routing.agent)}
          <select className="h-9 max-w-64 rounded-sm border bg-background px-2 text-body" value={draftAgent} disabled={saving} onChange={(e) => setDraftAgent(e.target.value)}>
            <option value="">{t(($) => $.lweixin.routing.choose_agent)}</option>
            {agentId && !available.some((agent) => agent.id === agentId) && (
              <option value={agentId} disabled>{t(($) => $.lweixin.routing.unavailable_agent)}</option>
            )}
            {available.map((agent) => <option key={agent.id} value={agent.id}>{agent.name}</option>)}
          </select>
        </label>
      )}
      {triggerMode !== undefined && (
        <label className="grid min-w-40 gap-1 text-caption">
          {t(($) => $.lweixin.routing.trigger)}
          <select className="h-9 rounded-sm border bg-background px-2 text-body" value={draftTrigger} disabled={saving} onChange={(e) => setDraftTrigger(e.target.value as LweixinGroupTriggerMode)}>
            <option value="mention">{t(($) => $.lweixin.routing.trigger_mention)}</option>
            <option value="all">{t(($) => $.lweixin.routing.trigger_all)}</option>
          </select>
        </label>
      )}
      <Button size="sm" disabled={!isDirty || saving || (draftMode === "agent" && (!draftAgent || !available.some((agent) => agent.id === draftAgent)))} onClick={save} aria-busy={saving}>
        {t(($) => $.lweixin.routing.save)}
      </Button>
      {triggerMode !== undefined && draftTrigger === "mention" && triggerReason === "mention_metadata_unavailable" && (
        <p role="status" className="w-full text-caption text-muted-foreground">{t(($) => $.lweixin.routing.mention_unavailable)}</p>
      )}
      {triggerMode !== undefined && draftTrigger === "all" && (
        <p className="w-full text-caption text-muted-foreground">{t(($) => $.lweixin.routing.all_warning)}</p>
      )}
      {error && <p role="alert" className="w-full text-caption text-destructive">{error}</p>}
    </div>
  );
}

// LweixinAgentBindButton is the per-agent CTA on the agent detail page.
// LWeixin uses the bring-your-own-gateway model: the admin pastes the gateway
// base URL and an API token; the backend probes /api/status before persisting.
// Visibility mirrors TelegramAgentBindButton (owner/admin only).
export function LweixinAgentBindButton({
  agentId,
  agentName,
  className,
  onShowConnectedDetails,
}: {
  agentId: string;
  agentName?: string;
  className?: string;
  /** Compact read-only connected row that invokes this instead of the full
   * badge — the agent inspector passes a "jump to the Integrations tab"
   * handler so management actions live in one place. */
  onShowConnectedDetails?: () => void;
}) {
  const { t } = useT("settings");
  const wsId = useWorkspaceId();
  const qc = useQueryClient();
  const user = useAuthStore((s) => s.user);

  const [dialogOpen, setDialogOpen] = useState(false);
  const [baseUrl, setBaseUrl] = useState("");
  const [apiToken, setApiToken] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const { data: listing } = useQuery({
    ...lweixinInstallationsOptions(wsId),
    enabled: !!wsId,
  });
  const installSupported = listing?.install_supported === true;

  const { data: members = [] } = useQuery({
    ...memberListOptions(wsId),
    enabled: !!wsId,
  });
  const currentMember = members.find((m) => m.user_id === user?.id) ?? null;
  const canManage =
    currentMember?.role === "owner" || currentMember?.role === "admin";

  if (!canManage) return null;

  const existing = listing?.installations.find(
    (inst) => inst.agent_id === agentId && inst.status === "active",
  );
  if (existing) {
    return onShowConnectedDetails ? (
      <LweixinAgentStatusRow onClick={onShowConnectedDetails} className={className} />
    ) : (
      <LweixinAgentConnectedBadge installation={existing} className={className} />
    );
  }

  if (!installSupported) return null;

  function closeDialog() {
    if (submitting) return;
    setDialogOpen(false);
    setBaseUrl("");
    setApiToken("");
  }

  async function handleSubmit() {
    const base_url = baseUrl.trim();
    const api_token = apiToken.trim();
    if (submitting || !agentId || !base_url || !api_token) return;
    setSubmitting(true);
    try {
      const installation = await api.registerLweixinBot(wsId, agentId, {
        base_url,
        api_token,
      });
      if (!installation.id || installation.status !== "active") {
        throw new Error("LWeixin connection returned an invalid installation");
      }
      // The lweixin_installation realtime event also refreshes this list, but
      // invalidate explicitly so the connected badge appears immediately.
      await qc.invalidateQueries({ queryKey: lweixinKeys.installations(wsId) });
      toast.success(t(($) => $.lweixin.connect_success_toast));
      setDialogOpen(false);
      setBaseUrl("");
      setApiToken("");
    } catch (e) {
      toast.error(
        e instanceof Error ? e.message : t(($) => $.lweixin.connect_failed_toast),
      );
    } finally {
      setSubmitting(false);
    }
  }

  const canSubmit = baseUrl.trim() !== "" && apiToken.trim() !== "" && !submitting;

  return (
    <div
      className={cn("flex flex-wrap items-center gap-2", className)}
      data-testid="lweixin-agent-bind-buttons"
    >
      <Button
        variant="outline"
        size="sm"
        onClick={() => setDialogOpen(true)}
        disabled={!agentId}
        title={
          agentName
            ? t(($) => $.lweixin.bind_button_title, { agent: agentName })
            : undefined
        }
        data-testid="lweixin-agent-connect"
      >
        <LweixinMark className="h-3 w-3" />
        {t(($) => $.lweixin.bind_button)}
      </Button>

      <Dialog
        open={dialogOpen}
        onOpenChange={(v) => (v ? setDialogOpen(true) : closeDialog())}
      >
        <DialogContent className="sm:max-w-lg" data-testid="lweixin-connect-dialog">
          <DialogHeader>
            <DialogTitle>{t(($) => $.lweixin.connect_dialog_title)}</DialogTitle>
          </DialogHeader>

          <p className="text-caption text-muted-foreground">
            {t(($) => $.lweixin.connect_dialog_description)}
          </p>

          <button
            type="button"
            onClick={() => openExternal("https://github.com/bestvcboy/multica/blob/vc/main/docs/lweixin.md")}
            className="inline-flex w-fit items-center gap-2 text-body font-medium text-primary underline-offset-2 hover:underline"
            data-testid="lweixin-docs-link"
          >
            <ExternalLink className="h-4 w-4" />
            {t(($) => $.lweixin.connect_docs_link)}
          </button>

          <div className="space-y-1.5">
            <Label htmlFor="lweixin-base-url">
              {t(($) => $.lweixin.base_url_label)}
            </Label>
            <Input
              id="lweixin-base-url"
              data-testid="lweixin-base-url"
              type="url"
              value={baseUrl}
              onChange={(e) => setBaseUrl(e.target.value)}
              placeholder="https://wchat.example.com"
              autoComplete="off"
              spellCheck={false}
              disabled={submitting}
            />
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="lweixin-api-token">
              {t(($) => $.lweixin.api_token_label)}
            </Label>
            <Input
              id="lweixin-api-token"
              data-testid="lweixin-api-token"
              type="password"
              value={apiToken}
              onChange={(e) => setApiToken(e.target.value)}
              autoComplete="off"
              spellCheck={false}
              disabled={submitting}
            />
          </div>

          <DialogFooter>
            <Button
              variant="outline"
              size="sm"
              onClick={closeDialog}
              disabled={submitting}
            >
              {t(($) => $.lweixin.connect_cancel)}
            </Button>
            <Button
              size="sm"
              onClick={handleSubmit}
              disabled={!canSubmit}
              data-testid="lweixin-connect-submit"
            >
              {submitting
                ? t(($) => $.lweixin.connect_submitting)
                : t(($) => $.lweixin.connect_submit)}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

// LweixinAgentStatusRow is the compact, read-only connected affordance the
// agent inspector renders; it deep-links into the Integrations tab.
function LweixinAgentStatusRow({
  onClick,
  className,
}: {
  onClick: () => void;
  className?: string;
}) {
  const { t } = useT("settings");
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-caption text-muted-foreground transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50",
        className,
      )}
      data-testid="lweixin-agent-status"
    >
      <span className="inline-block h-1.5 w-1.5 shrink-0 rounded-full bg-emerald-500" />
      <span className="truncate">{t(($) => $.lweixin.agent_connected_label)}</span>
      <ChevronRight className="ml-auto h-3.5 w-3.5 shrink-0" />
    </button>
  );
}

// LweixinAgentConnectedBadge is the full "already connected" affordance:
// status + Disconnect, plus the gateway base URL for reference.
function LweixinAgentConnectedBadge({
  installation,
  className,
}: {
  installation: LweixinInstallation;
  className?: string;
}) {
  const { t } = useT("settings");
  const wsId = useWorkspaceId();
  const qc = useQueryClient();

  const [confirmOpen, setConfirmOpen] = useState(false);
  const [disconnecting, setDisconnecting] = useState(false);

  async function handleDisconnect() {
    if (disconnecting) return;
    setDisconnecting(true);
    try {
      await api.deleteLweixinInstallation(wsId, installation.id);
      await qc.invalidateQueries({ queryKey: lweixinKeys.installations(wsId) });
      toast.success(t(($) => $.lweixin.toast_disconnected));
      setConfirmOpen(false);
    } catch (e) {
      toast.error(
        e instanceof Error ? e.message : t(($) => $.lweixin.toast_disconnect_failed),
      );
    } finally {
      setDisconnecting(false);
    }
  }

  return (
    <div
      className={cn("space-y-2", className)}
      data-testid="lweixin-agent-connected"
    >
      <div className="flex items-center justify-between gap-3">
        <span className="inline-flex min-w-0 items-center gap-2 text-caption text-muted-foreground">
          <span className="inline-block h-1.5 w-1.5 shrink-0 rounded-full bg-emerald-500" />
          <span className="truncate">
            {t(($) => $.lweixin.agent_connected_label)}
            {installation.app_id ? ` · ${installation.app_id}` : ""}
          </span>
        </span>
        <Button
          variant="destructive"
          size="sm"
          onClick={() => setConfirmOpen(true)}
          disabled={disconnecting}
          title={t(($) => $.lweixin.agent_disconnect_tooltip)}
          aria-label={t(($) => $.lweixin.disconnect)}
          data-testid="lweixin-agent-disconnect"
        >
          <Trash2 className="h-3 w-3" />
          {disconnecting
            ? t(($) => $.lweixin.disconnecting)
            : t(($) => $.lweixin.disconnect)}
        </Button>
      </div>

      {installation.base_url && (
        <button
          type="button"
          onClick={() => openExternal(installation.base_url)}
          className="inline-flex items-center gap-1 text-caption text-muted-foreground underline-offset-2 transition-colors hover:text-foreground hover:underline"
          title={t(($) => $.lweixin.agent_console_tooltip)}
        >
          <ExternalLink className="h-3 w-3" />
          {installation.base_url}
        </button>
      )}

      <AlertDialog
        open={confirmOpen}
        onOpenChange={(v) => {
          if (!v && !disconnecting) setConfirmOpen(false);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t(($) => $.lweixin.disconnect_confirm_title)}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t(($) => $.lweixin.disconnect_confirm_description)}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={disconnecting}>
              {t(($) => $.lweixin.disconnect_confirm_cancel)}
            </AlertDialogCancel>
            <AlertDialogAction onClick={handleDisconnect} disabled={disconnecting}>
              {disconnecting
                ? t(($) => $.lweixin.disconnecting)
                : t(($) => $.lweixin.disconnect)}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
