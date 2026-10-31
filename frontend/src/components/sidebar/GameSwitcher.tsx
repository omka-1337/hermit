import { useState } from "react";
import { Engine, Game } from "../../api";
import ContextMenu, { anchorBelow, MenuAnchor, MenuDivider, MenuItem } from "../ContextMenu";
import GameIcon from "../GameIcon";
import GameSettings from "../GameSettings";
import { PlusIcon } from "../../screens/shell";
import { ChevronDownIcon } from "./icons";

type Props = {
  games: Game[];
  selected: Game | null;
  onSelect: (id: string) => void;
  onAdd: () => void;
  onChanged: (game: Game) => void;
  onRemove: () => void;
};

// GameSwitcher heads the desktop sidebar: the game being modded, with its
// icon, a list to switch to another one or add one, and the gear for the
// game's own settings.
export default function GameSwitcher({ games, selected, onSelect, onAdd, onChanged, onRemove }: Props) {
  const [menu, setMenu] = useState<MenuAnchor | null>(null);

  return (
    <div className="flex gap-2">
      <button
        onClick={(e) => setMenu(anchorBelow(e.currentTarget))}
        className="flex min-w-0 flex-1 items-center gap-2.5 rounded-lg border border-zinc-800 p-2 text-left hover:bg-zinc-800"
      >
        {selected ? (
          <>
            <GameIcon name={selected.name} steamAppId={selected.steamAppId} size={32} />
            <span className="flex min-w-0 flex-1 flex-col">
              <span className="truncate text-sm font-semibold">{selected.name}</span>
              <span className="truncate text-xs text-zinc-500">{engineName(selected)}</span>
            </span>
          </>
        ) : (
          <span className="flex-1 px-1 text-sm text-zinc-400">{games.length ? "Choose a game" : "No games yet"}</span>
        )}
        <ChevronDownIcon />
      </button>
      {selected && <GameSettings game={selected} onRemove={onRemove} onRenamed={onChanged} align="left" />}

      {menu && (
        <ContextMenu at={menu} onClose={() => setMenu(null)} width="w-64">
          {games.map((g) => (
            <MenuItem
              key={g.id}
              selected={g.id === selected?.id}
              onClick={() => {
                setMenu(null);
                onSelect(g.id);
              }}
            >
              <GameIcon name={g.name} steamAppId={g.steamAppId} size={24} />
              <span className="truncate">{g.name}</span>
            </MenuItem>
          ))}
          {games.length > 0 && <MenuDivider />}
          <MenuItem
            onClick={() => {
              setMenu(null);
              onAdd();
            }}
          >
            <PlusIcon />
            Add game…
          </MenuItem>
        </ContextMenu>
      )}
    </div>
  );
}

function engineName(game: Game): string {
  switch (game.engine) {
    case Engine.EngineUnity:
      return "Unity · BepInEx";
    case Engine.EngineCreation:
      return "Creation Engine";
  }
  return "";
}
