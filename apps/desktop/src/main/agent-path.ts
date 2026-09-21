import { win32 as win32Path } from "path";

function pathKey(env: NodeJS.ProcessEnv): string {
  return (
    Object.keys(env).find((key) => key.toLowerCase() === "path") ?? "PATH"
  );
}

function pathIdentity(value: string): string {
  return value.replace(/[\\/]+$/, "").toLowerCase();
}

/**
 * Make agent CLIs installed for the current Windows user visible to the
 * bundled daemon. Desktop apps do not always inherit the latest interactive
 * shell PATH, especially when they are launched from Explorer or start with
 * Windows. Keep this pure and injectable so the Windows-specific path contract
 * is covered without booting Electron in tests.
 */
export function prepareDesktopAgentEnvironment(
  platform: NodeJS.Platform,
  env: NodeJS.ProcessEnv,
): void {
  if (platform !== "win32") return;

  const userProfile = env.USERPROFILE?.trim();
  const appData =
    env.APPDATA?.trim() ||
    (userProfile
      ? win32Path.join(userProfile, "AppData", "Roaming")
      : undefined);
  const localAppData =
    env.LOCALAPPDATA?.trim() ||
    (userProfile
      ? win32Path.join(userProfile, "AppData", "Local")
      : undefined);

  const additions = [
    appData && win32Path.join(appData, "npm"),
    localAppData && win32Path.join(localAppData, "OpenAI", "Codex", "bin"),
    env.ProgramFiles?.trim() &&
      win32Path.join(env.ProgramFiles.trim(), "nodejs"),
    env.ProgramW6432?.trim() &&
      win32Path.join(env.ProgramW6432.trim(), "nodejs"),
  ].filter((value): value is string => Boolean(value));

  const key = pathKey(env);
  const current = (env[key] ?? "").split(win32Path.delimiter).filter(Boolean);
  const seen = new Set(current.map(pathIdentity));
  const prepend = additions.filter((value) => {
    const identity = pathIdentity(value);
    if (seen.has(identity)) return false;
    seen.add(identity);
    return true;
  });

  if (prepend.length > 0) {
    env[key] = [...prepend, ...current].join(win32Path.delimiter);
  }
}
