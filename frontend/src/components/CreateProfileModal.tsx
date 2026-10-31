import { useState } from "react";
import { Engine, errorMessage, Game, Library } from "../api";
import ModpackInstallButton from "./browse/ModpackInstallButton";
import PackageBrowser from "./browse/PackageBrowser";
import { Button, ErrorText, inputClass, Modal } from "./ui";

type Tab = "empty" | "modpack";

type Props = {
  game: Game;
  onClose: () => void;
  // onCreated is called with the new profile; open is set when it should be shown right away.
  onCreated: (profileId: string, open: boolean) => void;
};

// CreateProfileModal creates an empty profile or one from a Thunderstore
// modpack; Creation Engine games have no modpacks there.
export default function CreateProfileModal({ game, onClose, onCreated }: Props) {
  const [tab, setTab] = useState<Tab>("empty");
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const createEmpty = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setBusy(true);
    try {
      const profile = await Library.CreateProfile(game.id, name.trim());
      onCreated(profile.id, false);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  const tabs: { id: Tab; label: string }[] = [
    { id: "empty", label: "Empty profile" },
    ...(game.engine === Engine.EngineCreation ? [] : [{ id: "modpack" as const, label: "From modpack" }]),
  ];

  return (
    <Modal title="Create a new profile" size={tab === "modpack" ? "xl" : "md"} onClose={onClose}>
      <nav className={`mb-4 flex gap-1 border-b border-zinc-800 ${tabs.length > 1 ? "" : "hidden"}`}>
        {tabs.map((t) => (
          <button
            key={t.id}
            onClick={() => setTab(t.id)}
            className={`-mb-px border-b-2 px-3 pb-2 text-sm ${
              tab === t.id ? "border-indigo-500 text-zinc-100" : "border-transparent text-zinc-400 hover:text-zinc-200"
            }`}
          >
            {t.label}
          </button>
        ))}
      </nav>

      {tab === "empty" ? (
        <form onSubmit={createEmpty} className="flex flex-col gap-4">
          <label className="flex flex-col gap-1.5">
            <span className="text-sm text-zinc-400">Profile name</span>
            <input className={inputClass} value={name} autoFocus onChange={(e) => setName(e.target.value)} />
          </label>
          <ErrorText>{error}</ErrorText>
          <div className="flex justify-end gap-2">
            <Button type="button" variant="ghost" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" variant="primary" disabled={busy || !name.trim()}>
              Create
            </Button>
          </div>
        </form>
      ) : (
        <div className="min-h-0 flex-1 overflow-hidden rounded-lg border border-zinc-800 bg-zinc-950">
          <PackageBrowser
            game={game}
            modpacksOnly
            renderActions={(pkg) => (
              <ModpackInstallButton game={game} pkg={pkg} onInstalled={(id) => onCreated(id, true)} />
            )}
          />
        </div>
      )}
    </Modal>
  );
}
