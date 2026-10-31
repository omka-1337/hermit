import { useState } from "react";
import { Dialogs } from "@wailsio/runtime";
import { BethesdaService, errorMessage, Game, MO2Instance, Profile } from "../api";
import { Button, ErrorText, inputClass, Modal, Spinner } from "./ui";

type Props = {
  game: Game;
  // profileId is the empty profile to import into; without it the import
  // makes a profile of its own.
  profileId?: string;
  onClose: () => void;
  onImported: (profile: Profile) => void;
};

// ImportMO2Modal brings a profile of a Mod Organizer 2 instance over into
// Hermit: its mods as MO2 installed them, their order, their separators, the
// options picked in their installers, the plugin list and the profile's own
// INI files. MO2 itself is left as it is.
export default function ImportMO2Modal({ game, profileId, onClose, onImported }: Props) {
  const [instance, setInstance] = useState<MO2Instance | null>(null);
  const [chosen, setChosen] = useState("");
  const [name, setName] = useState("");
  const [location, setLocation] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const pickInstance = async () => {
    setError("");
    try {
      const dir = await Dialogs.OpenFile({
        Title: "Choose the Mod Organizer 2 folder, the one with ModOrganizer.ini",
        CanChooseDirectories: true,
        CanChooseFiles: false,
      });
      if (!dir) return;
      const found = await BethesdaService.InspectMO2(dir);
      const profile = found.profiles?.includes(found.selected) ? found.selected : (found.profiles?.[0] ?? "");
      setInstance(found);
      setChosen(profile);
      if (!name) setName(profile);
      // Next to MO2 is on its disk, where the files are linked, not copied.
      if (!location) setLocation(found.dir.replace(/\/[^/]+\/?$/, "") || "/");
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  const pickLocation = async () => {
    const dir = await Dialogs.OpenFile({
      Title: "Choose where to keep the profile",
      CanChooseDirectories: true,
      CanChooseFiles: false,
      CanCreateDirectories: true,
    });
    if (dir) setLocation(dir);
  };

  const run = async () => {
    if (!instance) return;
    setBusy(true);
    setError("");
    try {
      onImported(
        profileId
          ? await BethesdaService.ImportMO2(game.id, profileId, instance.dir, chosen)
          : await BethesdaService.ImportMO2AsProfile(game.id, instance.dir, chosen, name.trim(), location),
      );
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
            <Button onClick={pickInstance} disabled={busy}>
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
        {!profileId && instance && (
          <>
            <label className="flex flex-col gap-1.5 text-sm">
              <span className="text-zinc-400">Profile name</span>
              <input className={inputClass} value={name} disabled={busy} onChange={(e) => setName(e.target.value)} />
            </label>
            <div className="flex flex-col gap-1.5">
              <span className="text-sm text-zinc-400">Keep the profile in</span>
              <div className="flex gap-2">
                <div className="flex-1 truncate rounded-md border border-zinc-700 bg-zinc-900 px-3 py-1.5 text-sm text-zinc-300 select-text">
                  {location || <span className="text-zinc-500">Hermit&apos;s own data</span>}
                </div>
                <Button onClick={pickLocation} disabled={busy}>
                  Browse…
                </Button>
                {location && (
                  <Button variant="ghost" onClick={() => setLocation("")} disabled={busy}>
                    Reset
                  </Button>
                )}
              </div>
            </div>
          </>
        )}
        <p className="text-xs text-zinc-500">
          {profileId
            ? "On the disk MO2 is on, files are linked rather than copied: the import takes seconds and no space. Move this profile to that disk first (right click on it, Move to folder…) if it is not there yet."
            : "On the disk MO2 is on, files are linked rather than copied: the import takes seconds and no space."}
        </p>
        <ErrorText>{error}</ErrorText>
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={onClose} disabled={busy}>
            Cancel
          </Button>
          <Button variant="primary" onClick={run} disabled={!instance || !chosen || busy || (!profileId && !name.trim())}>
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
