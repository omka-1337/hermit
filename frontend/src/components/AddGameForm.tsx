import { useState } from "react";
import { Dialogs } from "@wailsio/runtime";
import { Engine, errorMessage, Game, GameCandidate, Library } from "../api";
import { BackendBadge, Button, EngineBadge, ErrorText, inputClass, RuntimeBadge } from "./ui";

type Props = {
  onAdded: (game: Game) => void;
  // Rendered next to the submit button, e.g. Back or Cancel.
  extraActions?: React.ReactNode;
};

// AddGameForm adds a game from a manually picked folder.
export default function AddGameForm({ onAdded, extraActions }: Props) {
  const [candidate, setCandidate] = useState<GameCandidate | null>(null);
  const [name, setName] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const pickFolder = async () => {
    setError("");
    try {
      const path = await Dialogs.OpenFile({
        Title: "Select game folder",
        CanChooseDirectories: true,
        CanChooseFiles: false,
      });
      if (!path) return;
      const c = await Library.InspectGamePath(path);
      setCandidate(c);
      setName(c.name);
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!candidate) return;
    setBusy(true);
    setError("");
    try {
      onAdded(await Library.AddGame(name.trim(), candidate.path));
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <form onSubmit={submit} className="flex flex-col gap-4">
      <div className="flex flex-col gap-1.5">
        <span className="text-sm text-zinc-400">Game folder</span>
        <div className="flex gap-2">
          <div className="flex-1 truncate rounded-md border border-zinc-700 bg-zinc-900 px-3 py-1.5 text-sm text-zinc-300 select-text">
            {candidate?.path ?? <span className="text-zinc-500">Not selected</span>}
          </div>
          <Button type="button" onClick={pickFolder}>
            Browse…
          </Button>
        </div>
      </div>

      {candidate && (
        <>
          <label className="flex flex-col gap-1.5">
            <span className="text-sm text-zinc-400">Name</span>
            <input className={inputClass} value={name} onChange={(e) => setName(e.target.value)} autoFocus />
          </label>
          {candidate.detection.engine !== Engine.EngineUnknown ? (
            <div className="flex items-center gap-2 text-sm text-zinc-400">
              Detected: <EngineBadge engine={candidate.detection.engine} version={candidate.detection.engineVersion} />
              <RuntimeBadge runtime={candidate.detection.runtime} />
              <BackendBadge backend={candidate.detection.backend} />
            </div>
          ) : (
            <p className="text-sm text-amber-400">
              No game Hermit can mod was found in this folder: it mods Unity games through BepInEx, and Bethesda's
              Creation Engine games such as Skyrim. Make sure you picked the folder
              containing the game executable.
            </p>
          )}
          {candidate.alreadyAdded && <p className="text-sm text-amber-400">This game is already added.</p>}
        </>
      )}

      <ErrorText>{error}</ErrorText>

      <div className="flex items-center justify-end gap-2">
        {extraActions}
        <Button
          type="submit"
          variant="primary"
          disabled={!candidate || candidate.alreadyAdded || !name.trim() || busy}
        >
          Add game
        </Button>
      </div>
    </form>
  );
}
