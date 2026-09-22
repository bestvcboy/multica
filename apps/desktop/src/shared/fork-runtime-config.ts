import type { RuntimeConfig } from "./runtime-config";

// Self-hosted fork defaults, kept separate from upstream endpoint logic.
export const FORK_RUNTIME_DEFAULTS: Partial<RuntimeConfig> = {
  apiUrl: "https://paperclip.cheyishang.com",
  wsUrl: "wss://paperclip.cheyishang.com/ws",
  appUrl: "https://paperclip.cheyishang.com",
};
