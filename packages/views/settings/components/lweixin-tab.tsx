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
import { useActorName } from "@multica/core/workspace/hooks";
import { lweixinInstallationsOptions, lweixinKeys } from "@multica/core/lweixin";
import { api } from "@multica/core/api";
import type { LweixinInstallation } from "@multica/core/types";
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

  async function handleDisconnect() {
    if (!disconnectTarget || disconnecting) return;
    setDisconnecting(true);
    try {
      // Await the server before touching cache/UI (repo rule: no optimistic
      // removal on flows that confirm/destroy).
      await api.deleteLweixinInstallation(wsId, disconnectTarget);
      await qc.invalidateQueries({ queryKey: lweixinKeys.installations(wsId) });
      toast.success(t(($) => $.lweixin.toast_disconnected));
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
                  />
                ))}
              </CardContent>
            </Card>
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
}: {
  installation: LweixinInstallation;
  canManage: boolean;
  onDisconnect: () => void;
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
      {canManage && isActive && (
        <Button variant="outline" size="sm" onClick={onDisconnect}>
          <Trash2 className="h-3 w-3" />
          {t(($) => $.lweixin.disconnect)}
        </Button>
      )}
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
