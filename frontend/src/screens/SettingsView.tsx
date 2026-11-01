import { useEffect, useState } from "react";
import { AppInfo, errorMessage, InfoService, Settings, SettingsStore, UIMode, Update } from "../api";
import { Browser } from "@wailsio/runtime";
import AddToSteam from "../components/AddToSteam";
import CacheSettings from "../components/CacheSettings";
import NexusSettings from "../components/NexusSettings";
import { Button, ErrorText, inputClass, Spinner } from "../components/ui";
import { useUIMode } from "../uimode";

export default function SettingsView() {
  const [settings, setSettings] = useState<Settings | null>(null);
  const [error, setError] = useState("");
  const [info, setInfo] = useState<AppInfo | null>(null);
  const [release, setRelease] = useState<Update | null>(null);
  const [updateError, setUpdateError] = useState("");
  const { preference, layout, steamDeck, setPreference } = useUIMode();

  useEffect(() => {
    InfoService.GetInfo().then(setInfo).catch(console.error);
    InfoService.CheckUpdate()
      .then(setRelease)
      .catch((err) => setUpdateError(errorMessage(err)));
    SettingsStore.Get()
      .then(setSettings)
      .catch((err) => setError(errorMessage(err)));
  }, []);

  const act = async (action: () => Promise<void>) => {
    setError("");
    try {
      await action();
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  const update = (patch: Partial<Settings>) =>
    act(async () => {
      if (settings) setSettings(await SettingsStore.Update({ ...settings, ...patch }));
    });

  return (
    <div className="mx-auto flex max-w-3xl flex-col gap-6 p-8">
      <h1 className="text-2xl font-semibold">Settings</h1>
      <ErrorText>{error}</ErrorText>

      <section className="flex flex-col gap-3">
        <h2 className="text-sm font-semibold tracking-wide text-zinc-400 uppercase">Interface</h2>
        <div className="flex flex-col gap-2 rounded-lg border border-zinc-800 px-4 py-3">
          <span className="text-sm font-medium">Layout</span>
          <span className="text-xs text-zinc-500">
            The Steam Deck layout scales everything up for the handheld screen and shows package details over the
            list instead of beside it. Some changes need a restart to resize the window.
          </span>
          <div className="mt-1 flex gap-2">
            {[
              { mode: UIMode.UIModeAuto, label: `Automatic (${steamDeck ? "Steam Deck" : "desktop"})` },
              { mode: UIMode.UIModeDesktop, label: "Desktop" },
              { mode: UIMode.UIModeDeck, label: "Steam Deck" },
            ].map((option) => (
              <Button
                key={option.mode}
                data-focus-first={preference === option.mode || undefined}
                variant={preference === option.mode ? "primary" : "secondary"}
                onClick={() => act(() => setPreference(option.mode))}
              >
                {option.label}
              </Button>
            ))}
          </div>
          <span className="text-xs text-zinc-500">Now using the {layout === "deck" ? "Steam Deck" : "desktop"} layout.</span>
        </div>
      </section>

      {/* Downloads from Nexus Mods are for the games whose mods are deployed, in the desktop layout. */}
      {layout === "desktop" && <NexusSettings />}
      {settings && (
        <section className="flex flex-col gap-3">
          <h2 className="text-sm font-semibold tracking-wide text-zinc-400 uppercase">Mod browser</h2>
          <label className="flex cursor-pointer items-start gap-3 rounded-lg border border-zinc-800 px-4 py-3">
            <input
              type="checkbox"
              className="mt-1 accent-accent-500"
              checked={settings.allowNsfw}
              onChange={(e) => update({ allowNsfw: e.target.checked })}
            />
            <span className="flex flex-col gap-0.5">
              <span className="text-sm font-medium">Allow NSFW content</span>
              <span className="text-xs text-zinc-500">Show packages marked as NSFW when browsing Thunderstore.</span>
            </span>
          </label>
        </section>
      )}
      {settings && <DownloadLimit limit={settings.downloadLimit ?? 0} onChange={(kb) => update({ downloadLimit: kb })} />}
      <section className="flex flex-col gap-3">
        <h2 className="text-sm font-semibold tracking-wide text-zinc-400 uppercase">Storage</h2>
        <CacheSettings />
      </section>
      <section className="flex flex-col gap-3">
        <h2 className="text-sm font-semibold tracking-wide text-zinc-400 uppercase">Steam</h2>
        <div className="rounded-lg border border-zinc-800 px-4 py-3">
          <AddToSteam />
        </div>
      </section>
      {info && (
        <section className="flex flex-col gap-1 text-sm">
          <h2 className="text-sm font-semibold tracking-wide text-zinc-400 uppercase">About</h2>
          <p>{info.name}</p>
          <p className="text-zinc-500">
            v{info.version} · {info.os}/{info.arch}
          </p>
          {updateError ? (
            <p className="text-xs text-zinc-500">Could not check for updates: {updateError}</p>
          ) : !release ? (
            <p className="flex items-center gap-2 text-xs text-zinc-500">
              <Spinner /> Checking for a newer version…
            </p>
          ) : release.available ? (
            <p className="flex items-center gap-2 text-sm text-amber-400">
              Hermit {release.latest} is available
              <Button variant="secondary" onClick={() => Browser.OpenURL(release.url)}>
                Open release page
              </Button>
            </p>
          ) : (
            <p className="text-xs text-emerald-400">✓ You are on the latest version</p>
          )}
          <p className="text-xs text-zinc-500">
            Copyright (C) 2026 Omka. Free software under the{" "}
            <button
              className="text-accent-400 hover:underline"
              onClick={() => Browser.OpenURL("https://www.gnu.org/licenses/gpl-3.0.html")}
            >
              GNU GPL v3
            </button>{" "}
            or later, with no warranty. Source:{" "}
            <button className="text-accent-400 hover:underline" onClick={() => Browser.OpenURL(info.repository)}>
              {info.repository.replace("https://", "")}
            </button>
          </p>
        </section>
      )}
    </div>
  );
}

// limits are the speeds offered, in MB a second; 0 is none.
const limits = [0, 2, 5, 10, 20];

// DownloadLimit caps the speed all downloads share: a few of them at full
// speed can fill a home connection until calls, streams and games on it drop.
function DownloadLimit({ limit, onChange }: { limit: number; onChange: (kb: number) => void }) {
  const [custom, setCustom] = useState("");
  const mb = limit / 1024;
  const save = () => {
    const value = Number(custom.replace(",", "."));
    if (custom.trim() !== "" && value >= 0) onChange(Math.round(value * 1024));
    setCustom("");
  };
  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-sm font-semibold tracking-wide text-zinc-400 uppercase">Downloads</h2>
      <div className="flex flex-col gap-2 rounded-lg border border-zinc-800 px-4 py-3">
        <span className="text-sm font-medium">Speed limit</span>
        <span className="text-xs text-zinc-500">
          Mods from Thunderstore and Nexus Mods share it. A modpack downloads several mods at once, and at full speed
          they can fill the connection until calls, streams and games on it drop.
        </span>
        <div className="mt-1 flex flex-wrap items-center gap-2">
          {limits.map((l) => (
            <Button key={l} variant={mb === l ? "primary" : "secondary"} onClick={() => onChange(l * 1024)}>
              {l === 0 ? "No limit" : `${l} MB/s`}
            </Button>
          ))}
          <input
            className={`${inputClass} w-24`}
            inputMode="decimal"
            placeholder={limit && !limits.includes(mb) ? `${+mb.toFixed(1)}` : "Other"}
            value={custom}
            onChange={(e) => setCustom(e.target.value)}
            onBlur={save}
            onKeyDown={(e) => e.key === "Enter" && save()}
          />
          <span className="text-xs text-zinc-500">MB/s</span>
        </div>
        {limit > 0 && !limits.includes(mb) && <span className="text-xs text-zinc-500">Now {+mb.toFixed(1)} MB/s.</span>}
      </div>
    </section>
  );
}
