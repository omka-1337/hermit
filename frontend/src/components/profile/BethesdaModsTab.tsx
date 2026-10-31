import { useState } from "react";
import { Browser, Dialogs } from "@wailsio/runtime";
import { BethesdaService, confirmDanger, errorMessage, MO2Instance, Mod } from "../../api";
import { Button, ErrorText, inputClass, Modal, Spinner, Toggle } from "../ui";
import { useProfile } from "./ProfileContext";

// BethesdaModsTab lists the mods of a Creation Engine profile in the order
// they are deployed into Data: where two mods have the same file, the one
// further down wins, as in Mod Organizer 2.
export default function BethesdaModsTab() {
  const { game, profile, applyProfile, setEnabled } = useProfile();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState<string | null>(null);
  const [importing, setImporting] = useState(false);
  const mods = profile.mods ?? [];

  const act = async (id: string, action: () => Promise<void>) => {
    setError("");
    setBusy(id);
    try {
      await action();
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(null);
    }
  };

  const move = (m: Mod, to: number) =>
    act(m.id, async () => applyProfile(await BethesdaService.MoveMod(game.id, profile.id, m.id, to)));

  const remove = (m: Mod) =>
    act(m.id, async () => {
      if (await confirmDanger("Remove mod", `Remove ${m.name} and its files from this profile?`, "Remove")) {
        applyProfile(await BethesdaService.RemoveMod(game.id, profile.id, m.id));
      }
    });

  const importModal = importing && <ImportMO2Modal onClose={() => setImporting(false)} />;

  if (mods.length === 0) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3">
        <p className="text-zinc-400">No mods in this profile yet.</p>
        <Button variant="primary" onClick={() => setImporting(true)}>
          Import from Mod Organizer 2…
        </Button>
        <p className="max-w-md text-center text-xs text-zinc-500">
          Brings over the mods of an MO2 profile as they are installed there, with their order, the options picked in
          their installers and the plugin list. MO2 itself is left as it is.
        </p>
        {importModal}
      </div>
    );
  }

  return (
    <div className="h-full overflow-y-auto px-6 py-3">
      {importModal}
      <ErrorText>{error}</ErrorText>
      <p className="mb-2 text-xs text-zinc-500">
        Mods further down win when two have the same file. Files the game and tools write into Data while it runs are
        kept in the profile's overwrite folder, which comes after every mod.
      </p>
      <ul className="divide-y divide-zinc-800">
        {mods.map((m, i) => (
          <li key={m.id} className="flex items-center gap-3 py-2">
            <span className="w-7 shrink-0 text-right text-xs text-zinc-600 tabular-nums">{i + 1}</span>
            <Toggle
              first={i === 0}
              checked={m.enabled}
              disabled={busy !== null}
              title={m.enabled ? "Disable" : "Enable"}
              onChange={(enabled) => act(m.id, () => setEnabled(m.id, enabled))}
            />
            <div className={`flex min-w-0 flex-1 flex-col ${m.enabled ? "" : "opacity-50"}`}>
              <span className="truncate text-sm font-medium">
                {m.name}
                {m.version && <span className="ml-2 font-normal text-zinc-500">{m.version}</span>}
              </span>
              {m.notes && (
                <span className="truncate text-xs text-zinc-500" title={m.notes}>
                  {m.notes}
                </span>
              )}
            </div>
            {busy === m.id && <Spinner />}
            {m.source.type === "nexus" && (
              <Button variant="ghost" title={m.source.url} onClick={() => Browser.OpenURL(m.source.url!)}>
                Nexus
              </Button>
            )}
            <Button variant="ghost" title="Earlier: lose to more mods" disabled={busy !== null || i === 0} onClick={() => move(m, i - 1)}>
              ↑
            </Button>
            <Button
              variant="ghost"
              title="Later: win over more mods"
              disabled={busy !== null || i === mods.length - 1}
              onClick={() => move(m, i + 1)}
            >
              ↓
            </Button>
            <Button variant="danger" disabled={busy !== null} onClick={() => remove(m)}>
              Remove
            </Button>
          </li>
        ))}
      </ul>
    </div>
  );
}

function ImportMO2Modal({ onClose }: { onClose: () => void }) {
  const { game, profile, applyProfile } = useProfile();
  const [instance, setInstance] = useState<MO2Instance | null>(null);
  const [chosen, setChosen] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const pick = async () => {
    setError("");
    try {
      const dir = await Dialogs.OpenFile({
        Title: "Choose the Mod Organizer 2 folder, the one with ModOrganizer.ini",
        CanChooseDirectories: true,
        CanChooseFiles: false,
      });
      if (!dir) return;
      const found = await BethesdaService.InspectMO2(dir);
      setInstance(found);
      setChosen(found.profiles?.includes(found.selected) ? found.selected : (found.profiles?.[0] ?? ""));
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  const run = async () => {
    if (!instance) return;
    setBusy(true);
    setError("");
    try {
      applyProfile(await BethesdaService.ImportMO2(game.id, profile.id, instance.dir, chosen));
      onClose();
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal title="Import from Mod Organizer 2" onClose={busy ? () => {} : onClose}>
      <div className="flex flex-col gap-4">
        <div className="flex flex-col gap-1.5">
          <span className="text-sm text-zinc-400">MO2 folder</span>
          <div className="flex gap-2">
            <div className="flex-1 truncate rounded-md border border-zinc-700 bg-zinc-900 px-3 py-1.5 text-sm text-zinc-300 select-text">
              {instance?.dir ?? <span className="text-zinc-500">Not selected</span>}
            </div>
            <Button onClick={pick} disabled={busy}>
              Browse…
            </Button>
          </div>
          {instance && <span className="text-xs text-zinc-500">Set up for {instance.gameName}</span>}
        </div>
        {instance && (instance.profiles?.length ?? 0) > 1 && (
          <label className="flex flex-col gap-1.5 text-sm">
            <span className="text-zinc-400">MO2 profile</span>
            <select className={inputClass} value={chosen} disabled={busy} onChange={(e) => setChosen(e.target.value)}>
              {instance.profiles!.map((p) => (
                <option key={p} value={p}>
                  {p}
                </option>
              ))}
            </select>
          </label>
        )}
        <p className="text-xs text-zinc-500">
          On the disk MO2 is on, files are linked rather than copied: the import takes seconds and no space. Move this
          profile to that disk first (right click on it, Move to folder…) if it is not there yet.
        </p>
        <ErrorText>{error}</ErrorText>
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={onClose} disabled={busy}>
            Cancel
          </Button>
          <Button variant="primary" onClick={run} disabled={!instance || !chosen || busy}>
            {busy ? (
              <span className="flex items-center gap-2">
                <Spinner /> Importing…
              </span>
            ) : (
              "Import"
            )}
          </Button>
        </div>
      </div>
    </Modal>
  );
}
