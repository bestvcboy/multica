// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { I18nProvider } from "@multica/core/i18n/react";
import enCommon from "../../locales/en/common.json";
import enSettings from "../../locales/en/settings.json";

const mocks = vi.hoisted(() => ({
  register: vi.fn(), invalidate: vi.fn(), error: vi.fn(), role: "owner",
  queried: [] as string[], installations: [] as unknown[],
  conversations: [] as unknown[],
  updateDefault: vi.fn(), updateRoute: vi.fn(),
}));
vi.mock("@tanstack/react-query", () => ({
  queryOptions: <T,>(options: T) => options,
  useQuery: (options: { queryKey: (string | number)[]; enabled?: boolean }) => {
    if (options.enabled === false) return { data: undefined };
    const key = JSON.stringify(options.queryKey);
    mocks.queried.push(key);
    if (key.includes("members")) return { data: [{ user_id: "user-1", role: mocks.role }] };
    if (key.includes("agents")) return { data: [{ id: "agent-2", name: "Agent Two" }] };
    if (key.includes("routing")) return { data: { privateDefault: { mode: "silent", agentId: null }, privateRevision: 0 } };
    if (key.includes("conversations")) return { data: { conversations: mocks.conversations } };
    if (key.includes("messages")) return { data: { messages: [{ messageId: "m-1", senderId: "friend-1", text: "Hello", receivedAt: "2026-09-23" }] } };
    return { data: { installations: mocks.installations, configured: true, install_supported: true } };
  },
  useQueryClient: () => ({ invalidateQueries: mocks.invalidate }),
}));
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "workspace-1" }));
vi.mock("@multica/core/workspace/queries", () => ({
  memberListOptions: () => ({ queryKey: ["members"] }),
  agentListOptions: () => ({ queryKey: ["agents"] }),
}));
vi.mock("@multica/core/auth", () => {
  const state = { user: { id: "user-1" } };
  return { useAuthStore: Object.assign((select: (s: typeof state) => unknown) => select(state), {
    getState: () => state,
  }) };
});
vi.mock("@multica/core/api", () => ({ api: {
  registerLweixinBot: mocks.register,
  updateLweixinRouting: mocks.updateDefault,
  updateLweixinConversationRoute: mocks.updateRoute,
} }));
vi.mock("@multica/core/workspace/hooks", () => ({ useActorName: () => ({ getAgentName: () => "Agent" }) }));
vi.mock("../../common/actor-avatar", () => ({ ActorAvatar: () => null }));
vi.mock("../../platform", () => ({ openExternal: vi.fn() }));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: mocks.error } }));

import { LweixinAgentBindButton, LweixinTab } from "./lweixin-tab";

function renderButton() {
  render(<I18nProvider locale="en" resources={{ en: { common: enCommon, settings: enSettings } }}>
    <LweixinAgentBindButton agentId="agent-1" />
  </I18nProvider>);
}

function renderTab() {
  render(<I18nProvider locale="en" resources={{ en: { common: enCommon, settings: enSettings } }}>
    <LweixinTab />
  </I18nProvider>);
}

describe("LweixinAgentBindButton", () => {
  beforeEach(() => { cleanup(); vi.clearAllMocks(); mocks.role = "owner"; });

  it("preserves the gateway path and binds the account to the selected agent", async () => {
    mocks.register.mockResolvedValue({ id: "installation-1", status: "active" });
    renderButton();
    await userEvent.click(screen.getByTestId("lweixin-agent-connect"));
    await userEvent.type(screen.getByTestId("lweixin-base-url"), "https://wchat.example.com/a/b/c");
    const token = screen.getByTestId("lweixin-api-token");
    expect(token.getAttribute("type")).toBe("password");
    await userEvent.type(token, "test-token");
    await userEvent.click(screen.getByTestId("lweixin-connect-submit"));
    await waitFor(() => expect(mocks.register).toHaveBeenCalledWith("workspace-1", "agent-1", {
      base_url: "https://wchat.example.com/a/b/c", api_token: "test-token",
    }));
    await waitFor(() => expect(mocks.invalidate).toHaveBeenCalledWith({
      queryKey: ["lweixin", "workspace-1", "installations"],
    }));
  });

  it("hides connection management from members", () => {
    mocks.role = "member";
    renderButton();
    expect(screen.queryByTestId("lweixin-agent-connect")).toBeNull();
  });
});

describe("LweixinTab history", () => {
  beforeEach(() => {
    cleanup();
    vi.clearAllMocks();
    mocks.role = "owner";
    mocks.queried = [];
    mocks.installations = [{ id: "inst-1", agent_id: "agent-1", status: "active", installed_at: "2026-09-23" }];
    mocks.conversations = [{ id: "c-1", chatId: "friend-1", chatType: "p2p", routeMode: "inherit", routeAgentId: null, routeRevision: 0 }];
  });

  it("keeps private history queries and controls hidden from members", () => {
    mocks.role = "member";
    renderTab();
    expect(screen.queryByRole("button", { name: "Conversations" })).toBeNull();
    expect(mocks.queried.some((key) => key.includes("conversations") || key.includes("messages"))).toBe(false);
  });

  it("lets an admin open a conversation and read text history", async () => {
    mocks.role = "admin";
    renderTab();
    await userEvent.click(screen.getByRole("button", { name: "Conversations" }));
    await userEvent.click(screen.getByText("friend-1"));
    expect(screen.getByText("Hello")).toBeTruthy();
    expect(mocks.queried.some((key) => key.includes('"messages"'))).toBe(true);
  });

  it("saves the private default and an explicit silent friend rule", async () => {
    mocks.updateDefault.mockResolvedValue({});
    mocks.updateRoute.mockResolvedValue({});
    renderTab();
    await userEvent.click(screen.getByRole("button", { name: "Conversations" }));
    await userEvent.selectOptions(screen.getByLabelText("Routing"), "agent");
    await userEvent.selectOptions(screen.getByLabelText("Agent"), "agent-2");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(mocks.updateDefault).toHaveBeenCalledWith("workspace-1", "inst-1", { mode: "agent", agentId: "agent-2" }));
    await userEvent.click(screen.getByText("friend-1"));
    await userEvent.selectOptions(screen.getAllByLabelText("Routing")[1]!, "silent");
    await userEvent.click(screen.getAllByRole("button", { name: "Save" })[1]!);
    await waitFor(() => expect(mocks.updateRoute).toHaveBeenCalledWith("workspace-1", "inst-1", "c-1", "silent", null));
  });

  it("pages conversations without retaining the previous selection", async () => {
    mocks.conversations = Array.from({ length: 50 }, (_, i) => ({
      id: `c-${i}`, chatId: `friend-${i}`, chatType: "p2p", routeMode: "inherit", routeAgentId: null, routeRevision: 0,
    }));
    renderTab();
    await userEvent.click(screen.getByRole("button", { name: "Conversations" }));
    await userEvent.click(screen.getByText("friend-0"));
    expect(screen.getByText("Hello")).toBeTruthy();
    await userEvent.click(screen.getAllByRole("button", { name: "Next" })[0]!);
    expect(screen.queryByText("Hello")).toBeNull();
    expect(mocks.queried.some((key) => key.includes('"conversations",50'))).toBe(true);
  });
});
