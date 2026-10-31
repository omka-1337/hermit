import { useEffect, useRef, useState } from "react";
import { Engine, errorMessage, Game, LaunchService, Library, Runtime } from "../api";
import EditableName from "./EditableName";
import { useLaunchInfo } from "./launch";
import { Button, ErrorText, inputClass } from "./ui";

type Props = {
  game: Game;
  onRemove: () => void;
  // onRenamed, when given, puts the game's name in the panel: the desktop
  // sidebar has no header to rename it in.
  onRenamed?: (game: Game) => void;
  // align is the side of the gear the panel lines up with.
  align?: "left" | "right";
  // pickExecutable is false where the file the game starts from is picked
  // next to Play instead.
  pickExecutable?: boolean;
};

// GameSettings is the gear next to Play on a game's page. It keeps the rarely
// used and the destructive out of the header: which executable the game
// starts from, and removing the game from Hermit.
export default function GameSettings({ game, onRemove, onRenamed, align = "right", pickExecutable: allowPick = true }: Props) {
  const [open, setOpen] = useState(false);
  const box = useRef<HTMLDivElement>(null);
  const { info } = useLaunchInfo(game.id);
  const [executables, setExecutables] = useState<string[]>([]);
  // A Creation Engine game starts through its script extender unless a file
  // is picked, so its default is not its own executable.
  const creation = game.engine === Engine.EngineCreation;
  const fallback = creation ? "" : game.executable;
  const [chosen, setChosen] = useState(game.launchExecutable || fallback);
  const [error, setError] = useState("");

  // A game that runs through Wine can be started from another file, whether
  // Hermit starts it or Steam does through the launch options.
  const choosable = allowPick && !!info?.supported && game.runtime === Runtime.RuntimeProton;

  useEffect(() => {
    if (!choosable) return;
    LaunchService.ListExecutables(game.id)
      .then((list) => setExecutables(list ?? []))
      .catch(() => setExecutables([]));
  }, [game.id, choosable]);

  useEffect(() => setChosen(game.launchExecutable || fallback), [game.launchExecutable, fallback]);

  // Close on a click anywhere else, or Escape.
  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (!box.current?.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    document.addEventListener("mousedown", onDown);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDown);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  const choose = async (exe: string) => {
    setError("");
    const previous = chosen;
    setChosen(exe);
    try {
      await Library.SetLaunchExecutable(game.id, exe);
    } catch (err) {
      setChosen(previous);
      setError(errorMessage(err));
    }
  };

  const pickExecutable = choosable && executables.length > (creation ? 0 : 1);
  const custom = pickExecutable && chosen !== fallback;

  return (
    <div ref={box} className="relative flex">
      <button
        aria-label="Game settings"
        title="Game settings"
        onClick={() => setOpen((o) => !o)}
        className={`relative flex w-9 items-center justify-center rounded-md border transition-colors ${
          open ? "border-zinc-600 bg-zinc-800 text-zinc-100" : "border-zinc-700 text-zinc-400 hover:bg-zinc-800 hover:text-zinc-100"
        }`}
      >
        <GearIcon />
        {/* A dot says the game does not start from its own executable. */}
        {custom && <span className="absolute top-1 right-1 h-1.5 w-1.5 rounded-full bg-accent-400" />}
      </button>
      {open && (
        <div
          className={`absolute top-full z-20 mt-2 flex w-80 flex-col gap-3 rounded-lg border border-zinc-700 bg-zinc-900 p-3 shadow-xl ${
            align === "left" ? "left-0" : "right-0"
          }`}
        >
          {onRenamed && (
            <div className="flex flex-col gap-1 text-sm">
              <EditableName
                value={game.name}
                className="font-medium"
                onSave={async (name) => onRenamed(await Library.RenameGame(game.id, name))}
              />
              <span className="truncate text-xs text-zinc-500 select-text" title={game.path}>
                {game.path}
              </span>
            </div>
          )}
          {pickExecutable && (
            <label className="flex flex-col gap-1.5 text-sm">
              <span className="text-zinc-400">Start the game from</span>
              <select className={inputClass} value={chosen} onChange={(e) => choose(e.target.value)}>
                {creation && <option value="">Automatic: the script extender when it is installed</option>}
                {executables.map((exe) => (
                  <option key={exe} value={exe}>
                    {exe === game.executable ? `${exe} (the game)` : exe}
                  </option>
                ))}
              </select>
              <span className="text-xs text-zinc-500">
                {creation
                  ? "Automatic starts SKSE, F4SE or SFSE when the game folder or a mod has it, and the game as Steam starts it otherwise."
                  : "Some mods ship a launcher the game has to be started through; pick it here."}
                {!info?.external && " Steam starts it too, through the launch options."}
              </span>
            </label>
          )}
          <ErrorText>{error}</ErrorText>
          <div className={`flex flex-col gap-1.5 ${pickExecutable ? "border-t border-zinc-800 pt-3" : ""}`}>
            <Button
              variant="danger"
              className="self-start"
              onClick={() => {
                setOpen(false);
                onRemove();
              }}
            >
              Remove game
            </Button>
            <span className="text-xs text-zinc-500">
              Deletes its profiles and installed mods from Hermit. The game itself is not touched.
            </span>
          </div>
        </div>
      )}
    </div>
  );
}

function GearIcon() {
  return (
    <svg
      viewBox="0 0 24 24"
      className="h-4 w-4"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z" />
      <circle cx="12" cy="12" r="3" />
    </svg>
  );
}
