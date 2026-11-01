import { ReactNode, useState } from "react";
import { deployed, Engine, errorMessage, Game, Library } from "../api";
import ModpackInstallButton from "./browse/ModpackInstallButton";
import PackageBrowser from "./browse/PackageBrowser";
import ImportMO2Modal from "./ImportMO2Modal";
import { ImportModal } from "./share";
import { Button, ErrorText, inputClass, Modal } from "./ui";

type Way = "empty" | "modpack" | "import" | "code";

type Props = {
  game: Game;
  onClose: () => void;
  // onCreated is called with the new profile; open is set when it should be shown right away.
  onCreated: (profileId: string, open: boolean) => void;
};

// CreateProfileModal offers the ways a profile comes about, as cards with
// what each does: an empty profile, one from a Thunderstore modpack, one
// imported from a file (from Mod Organizer 2 for a Creation Engine game),
// or one downloaded from a profile code. Picking one goes on in the same
// dialog, with a way back to the cards. A game whose mods are deployed has
// neither modpacks nor codes.
export default function CreateProfileModal({ game, onClose, onCreated }: Props) {
  const [way, setWay] = useState<Way | null>(null);
  const back = () => setWay(null);

  const thunderstore = !deployed(game.engine);
  const creation = game.engine === Engine.EngineCreation;
  const ways: { id: Way; title: string; text: string; icon: ReactNode }[] = [
    {
      id: "empty",
      title: "Create new profile",
      text: "An empty profile, to fill with mods yourself.",
      icon: <PlusIcon />,
    },
    ...(thunderstore
      ? [
          {
            id: "modpack" as const,
            title: "Download modpack",
            text: "A profile with every mod of a modpack from Thunderstore.",
            icon: <ModpackIcon />,
          },
        ]
      : []),
    ...(thunderstore || creation
      ? [
          {
            id: "import" as const,
            title: "Import profile",
            text: creation
              ? "The mods, their order, the plugins and INI files of a Mod Organizer 2 profile."
              : "From an .r2z file, r2modman's format, which Hermit's Export saves too.",
            icon: <ImportIcon />,
          },
        ]
      : []),
    ...(thunderstore
      ? [
          {
            id: "code" as const,
            title: "Download from code",
            text: "From a profile code someone shared from r2modman or Hermit.",
            icon: <CodeIcon />,
          },
        ]
      : []),
  ];

  if (way === "import" && creation) {
    return <ImportMO2Modal game={game} onBack={back} onClose={onClose} onImported={(p) => onCreated(p.id, true)} />;
  }
  if (way === "import" || way === "code") {
    return (
      <ImportModal
        game={game}
        from={way === "import" ? "file" : "code"}
        onBack={back}
        onClose={onClose}
        onImported={(id) => onCreated(id, true)}
      />
    );
  }

  return (
    <Modal title="Create a new profile" size={way === "modpack" ? "xl" : "md"} onClose={onClose}>
      {way === null ? (
        <div className="grid grid-cols-2 gap-3">
          {ways.map((w, i) => (
            <button
              key={w.id}
              data-focus-first={i === 0 || undefined}
              onClick={() => setWay(w.id)}
              className="flex flex-col items-start gap-2 rounded-lg border border-zinc-800 bg-zinc-900/60 p-4 text-left transition-colors hover:border-accent-500/60 hover:bg-zinc-800/60"
            >
              <span className="flex h-10 w-10 items-center justify-center rounded-lg bg-accent-500/15 text-accent-300">{w.icon}</span>
              <span className="text-sm font-semibold">{w.title}</span>
              <span className="text-xs text-zinc-400">{w.text}</span>
            </button>
          ))}
        </div>
      ) : way === "empty" ? (
        <EmptyProfile game={game} onBack={back} onCreated={(id) => onCreated(id, false)} />
      ) : (
        <div className="flex min-h-0 flex-1 flex-col gap-3">
          <div className="min-h-0 flex-1 overflow-hidden rounded-lg border border-zinc-800 bg-zinc-950">
            <PackageBrowser
              game={game}
              modpacksOnly
              renderActions={(pkg) => <ModpackInstallButton game={game} pkg={pkg} onInstalled={(id) => onCreated(id, true)} />}
            />
          </div>
          <div className="flex justify-end">
            <Button variant="ghost" onClick={back}>
              Back
            </Button>
          </div>
        </div>
      )}
    </Modal>
  );
}

function EmptyProfile({ game, onBack, onCreated }: { game: Game; onBack: () => void; onCreated: (id: string) => void }) {
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const create = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setBusy(true);
    try {
      onCreated((await Library.CreateProfile(game.id, name.trim())).id);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <form onSubmit={create} className="flex flex-col gap-4">
      <label className="flex flex-col gap-1.5">
        <span className="text-sm text-zinc-400">Profile name</span>
        <input className={inputClass} value={name} autoFocus onChange={(e) => setName(e.target.value)} />
      </label>
      <ErrorText>{error}</ErrorText>
      <div className="flex justify-end gap-2">
        <Button type="button" variant="ghost" onClick={onBack}>
          Back
        </Button>
        <Button type="submit" variant="primary" disabled={busy || !name.trim()}>
          Create
        </Button>
      </div>
    </form>
  );
}

function Icon({ children }: { children: ReactNode }) {
  return (
    <svg viewBox="0 0 24 24" className="h-5 w-5" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      {children}
    </svg>
  );
}

function PlusIcon() {
  return (
    <Icon>
      <path d="M12 5v14M5 12h14" />
    </Icon>
  );
}

// A box of parts with an arrow into it: a modpack downloaded.
function ModpackIcon() {
  return (
    <Icon>
      <path d="M21 8 12 3 3 8v8l9 5 9-5V8Z" />
      <path d="m3 8 9 5 9-5" />
      <path d="M12 13v8" />
    </Icon>
  );
}

// A file with an arrow coming in.
function ImportIcon() {
  return (
    <Icon>
      <path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8Z" />
      <path d="M14 3v5h5" />
      <path d="M12 11v6" />
      <path d="m9 14 3 3 3-3" />
    </Icon>
  );
}

// Brackets around a dash: a code.
function CodeIcon() {
  return (
    <Icon>
      <path d="m8 7-5 5 5 5" />
      <path d="m16 7 5 5-5 5" />
      <path d="M10 12h4" />
    </Icon>
  );
}
