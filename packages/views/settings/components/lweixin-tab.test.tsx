// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { I18nProvider } from "@multica/core/i18n/react";
import enCommon from "../../locales/en/common.json";
import enSettings from "../../locales/en/settings.json";

const mocks = vi.hoisted(() => ({
  register: vi.fn(), invalidate: vi.fn(), error: vi.fn(), role: "owner",
}));
vi.mock("@tanstack/react-query", () => ({
  queryOptions: <T,>(options: T) => options,
  useQuery: (options: { queryKey: string[] }) => ({
    data: options.queryKey.includes("members")
      ? [{ user_id: "user-1", role: mocks.role }]
      : { installations: [], configured: true, install_supported: true },
  }),
  useQueryClient: () => ({ invalidateQueries: mocks.invalidate }),
}));
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "workspace-1" }));
vi.mock("@multica/core/workspace/queries", () => ({
  memberListOptions: () => ({ queryKey: ["members"] }),
}));
vi.mock("@multica/core/auth", () => {
  const state = { user: { id: "user-1" } };
  return { useAuthStore: Object.assign((select: (s: typeof state) => unknown) => select(state), {
    getState: () => state,
  }) };
});
vi.mock("@multica/core/api", () => ({ api: { registerLweixinBot: mocks.register } }));
vi.mock("@multica/core/workspace/hooks", () => ({ useActorName: () => ({}) }));
vi.mock("../../common/actor-avatar", () => ({ ActorAvatar: () => null }));
vi.mock("../../platform", () => ({ openExternal: vi.fn() }));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: mocks.error } }));

import { LweixinAgentBindButton } from "./lweixin-tab";

function renderButton() {
  render(<I18nProvider locale="en" resources={{ en: { common: enCommon, settings: enSettings } }}>
    <LweixinAgentBindButton agentId="agent-1" />
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
