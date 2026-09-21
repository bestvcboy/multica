import { describe, expect, it } from "vitest";

import { prepareDesktopAgentEnvironment } from "./agent-path";

describe("prepareDesktopAgentEnvironment", () => {
  it("adds Windows Codex install directories when the desktop PATH is stale", () => {
    const env = {
      APPDATA: "C:\\Users\\alice\\AppData\\Roaming",
      LOCALAPPDATA: "C:\\Users\\alice\\AppData\\Local",
      PATH: "C:\\Windows\\System32",
    };

    prepareDesktopAgentEnvironment("win32", env);

    expect(env.PATH?.split(";")).toEqual([
      "C:\\Users\\alice\\AppData\\Roaming\\npm",
      "C:\\Users\\alice\\AppData\\Local\\OpenAI\\Codex\\bin",
      "C:\\Windows\\System32",
    ]);
  });

  it("does not duplicate paths already present", () => {
    const env = {
      APPDATA: "C:\\Users\\alice\\AppData\\Roaming",
      LOCALAPPDATA: "C:\\Users\\alice\\AppData\\Local",
      PATH: [
        "C:\\Users\\alice\\AppData\\Roaming\\npm",
        "C:\\Windows\\System32",
        "C:\\Users\\alice\\AppData\\Local\\OpenAI\\Codex\\bin",
      ].join(";"),
    };

    prepareDesktopAgentEnvironment("win32", env);

    expect(env.PATH?.split(";")).toEqual([
      "C:\\Users\\alice\\AppData\\Roaming\\npm",
      "C:\\Windows\\System32",
      "C:\\Users\\alice\\AppData\\Local\\OpenAI\\Codex\\bin",
    ]);
  });

  it("keeps non-Windows environments unchanged", () => {
    const env = { PATH: "/usr/bin" };

    prepareDesktopAgentEnvironment("linux", env);

    expect(env.PATH).toBe("/usr/bin");
  });
});
