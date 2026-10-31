import { useState } from "react";
import { Browser } from "@wailsio/runtime";
import { DownloadService, errorMessage } from "../api";
import { useNexusSetup } from "../downloads";
import { Button, ErrorText, inputClass } from "./ui";

// The page of a Nexus Mods account where its personal API key is.
const apiKeysPage = "https://www.nexusmods.com/users/myaccount?tab=api%20access";

// NexusSettings sets up downloads from Nexus Mods: the account they are made
// with, by its personal API key, and Hermit opening the links of "Mod
// Manager Download" buttons.
export default function NexusSettings() {
  const { account, handler, setAccount, setHandler } = useNexusSetup();
  const [key, setKey] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const act = async (action: () => Promise<void>) => {
    setBusy(true);
    setError("");
    try {
      await action();
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-sm font-semibold tracking-wide text-zinc-400 uppercase">Nexus Mods</h2>
      <div className="flex flex-col gap-2 rounded-lg border border-zinc-800 px-4 py-3">
        <span className="text-sm font-medium">Account</span>
        {account?.hasKey ? (
          <div className="flex items-center gap-3">
            <span className="flex-1 text-sm text-zinc-300">
              Logged in as <span className="font-semibold">{account.name}</span>
              <span className="text-zinc-500">{account.premium ? " · Premium" : ""}</span>
            </span>
            <Button variant="ghost" disabled={busy} onClick={() => act(async () => setAccount(await DownloadService.SetAPIKey("")))}>
              Log out
            </Button>
          </div>
        ) : (
          <>
            <span className="text-xs text-zinc-500">
              Hermit downloads from Nexus Mods with your account's personal API key: open your account's API keys,
              copy the Personal API Key at the bottom and paste it here.
            </span>
            <div className="flex gap-2">
              <input
                type="password"
                placeholder="Personal API key"
                className={inputClass}
                value={key}
                disabled={busy}
                onChange={(e) => setKey(e.target.value)}
              />
              <Button variant="ghost" onClick={() => Browser.OpenURL(apiKeysPage)}>
                Get the key
              </Button>
              <Button
                variant="primary"
                disabled={busy || !key.trim()}
                onClick={() =>
                  act(async () => {
                    setAccount(await DownloadService.SetAPIKey(key));
                    setKey("");
                    // Logged in to download: the buttons open in Hermit unless
                    // another mod manager opens them, which is asked first.
                    if (!handler?.hermit && !handler?.entry) setHandler(await DownloadService.RegisterNXM());
                  })
                }
              >
                Log in
              </Button>
            </div>
          </>
        )}
      </div>
      <div className="flex flex-col gap-2 rounded-lg border border-zinc-800 px-4 py-3">
        <span className="text-sm font-medium">Mod Manager Download</span>
        <div className="flex items-center gap-3">
          <span className="flex-1 text-xs text-zinc-500">
            {handler?.hermit
              ? "The links of the Mod Manager Download buttons on Nexus Mods open in Hermit: the mod goes into the active profile of its game."
              : handler?.entry
                ? `The links of the Mod Manager Download buttons on Nexus Mods open in ${handler.entry} now.`
                : "Nothing opens the links of the Mod Manager Download buttons on Nexus Mods yet."}
          </span>
          {!handler?.hermit && (
            <Button disabled={busy} onClick={() => act(async () => setHandler(await DownloadService.RegisterNXM()))}>
              Open them in Hermit
            </Button>
          )}
        </div>
      </div>
      <ErrorText>{error}</ErrorText>
    </section>
  );
}
