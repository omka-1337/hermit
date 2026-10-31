import { useEffect, useState } from "react";
import { Engine, errorMessage, Game, LaunchService, Library, Runtime } from "../api";
import ContextMenu, { anchorBelow, MenuAnchor, MenuItem } from "./ContextMenu";
import { usePlay, usePortProtonIcon } from "./launch";
import { ChevronDownIcon } from "./sidebar/icons";

type Props = {
  game: Game;
  profileId: string;
  // onGameChanged runs when the game was started, or the file it starts from
  // was changed.
  onGameChanged: () => void;
};

// loader matches the script extenders' loaders, named the way their mods
// call them.
const loader = /^(skse64|skse|sksevr|f4se|f4sevr|sfse)_loader\.exe$/i;

function exeLabel(exe: string): string {
  const name = exe.split("/").pop() ?? exe;
  const m = loader.exec(name);
  return m ? `${m[1].toUpperCase()} (Loader)` : name.replace(/\.exe$/i, "");
}

// PlayBar is Play in the desktop layout, with the file the game starts from
// picked right next to it.
export default function PlayBar({ game, profileId, onGameChanged }: Props) {
  const { info, play, starting, error, viaPortProton } = usePlay(game, profileId, onGameChanged);
  const portProtonIcon = usePortProtonIcon();
  const [executables, setExecutables] = useState<string[]>([]);
  const [menu, setMenu] = useState<MenuAnchor | null>(null);
  const [pickError, setPickError] = useState("");
  const creation = game.engine === Engine.EngineCreation;

  // A game that runs through Wine can be started from another file, whether
  // Hermit starts it or Steam does through the launch options.
  const choosable = !!info?.supported && game.runtime === Runtime.RuntimeProton;
  useEffect(() => {
    if (!choosable) return;
    LaunchService.ListExecutables(game.id)
      .then((list) => setExecutables(list ?? []))
      .catch(() => setExecutables([]));
  }, [game.id, choosable]);

  // A Creation Engine game left to choose by itself starts through its
  // script extender when it has one.
  const chosen =
    game.launchExecutable ||
    (creation ? (executables.find((e) => loader.test(e.split("/").pop() ?? "")) ?? game.executable) : game.executable);
  const pickable = choosable && executables.length > 1;

  const pick = async (exe: string) => {
    setMenu(null);
    setPickError("");
    try {
      await Library.SetLaunchExecutable(game.id, exe);
      onGameChanged();
    } catch (err) {
      setPickError(errorMessage(err));
    }
  };

  // A game Steam does not start and PortProton cannot either is started by
  // whatever runs it; all there is to show is that it runs.
  if (info?.external && !viaPortProton) {
    return info.running ? <span className="text-sm font-medium text-emerald-300">Running</span> : null;
  }

  const disabled = !info?.supported || info.running || starting;
  return (
    <div className="flex flex-col items-end gap-1">
      <div className="flex items-center gap-1 rounded-xl border border-emerald-500/20 bg-emerald-950/60 p-1">
        {pickable && (
          <button
            title="The file the game starts from"
            onClick={(e) => setMenu(anchorBelow(e.currentTarget))}
            className="flex items-center gap-2 rounded-lg px-3 py-2 text-sm font-semibold text-emerald-300 hover:bg-emerald-500/10"
          >
            {exeLabel(chosen)}
            <ChevronDownIcon className="text-emerald-400/70" />
          </button>
        )}
        <button
          disabled={disabled}
          onClick={play}
          title={info?.supported ? "" : info?.reason}
          className="flex items-center justify-center gap-2 rounded-lg bg-emerald-500 px-5 py-2 text-sm font-bold tracking-wide text-zinc-950 uppercase transition-colors hover:bg-emerald-400 disabled:pointer-events-none disabled:opacity-50"
        >
          <svg viewBox="0 0 24 24" className="h-4 w-4" fill="currentColor" aria-hidden>
            <path d="M7 4.5v15a1 1 0 0 0 1.5.86l12.5-7.5a1 1 0 0 0 0-1.72L8.5 3.64A1 1 0 0 0 7 4.5Z" />
          </svg>
          {info?.running ? "Running" : starting ? "Starting…" : "Play"}
          {viaPortProton && portProtonIcon && <img src={portProtonIcon} alt="PortProton" className="h-4 w-4 shrink-0" />}
        </button>
      </div>
      {(error || pickError) && <span className="text-xs text-red-400">{error || pickError}</span>}

      {menu && (
        <ContextMenu at={menu} onClose={() => setMenu(null)} width="w-60">
          {executables.map((exe) => (
            <MenuItem key={exe} selected={exe === chosen} title={exe} onClick={() => pick(exe)}>
              <span className="flex-1 truncate">{exeLabel(exe)}</span>
              {exe !== exeLabel(exe) + ".exe" && <span className="truncate text-xs text-zinc-500">{exe}</span>}
            </MenuItem>
          ))}
        </ContextMenu>
      )}
    </div>
  );
}
