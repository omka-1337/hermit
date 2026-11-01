import { useState } from "react";
import { deployed, errorMessage, FolderService, Game, Library, Profile } from "../../api";
import { packageLabel } from "../../format";
import ContextMenu, { anchorBelow, MenuAnchor, MenuDivider, MenuItem } from "../ContextMenu";
import CreateProfileModal from "../CreateProfileModal";
import { useProfileActions } from "../profileActions";
import { ExportModal } from "../share";
import { inputClass } from "../ui";
import { PlusIcon } from "../../screens/shell";
import { ChevronDownIcon, DotsIcon } from "./icons";

type Props = {
  game: Game;
  profiles: Profile[];
  onGameChanged: (game: Game) => void;
  // reload re-reads the profiles after one was added, renamed or removed.
  reload: () => Promise<void>;
  onError: (message: string) => void;
};

// ProfileSwitcher sits under the game in the desktop sidebar: the profile
// picked there is the active one, the one Play and Steam start. New and
// imported profiles are added from its list, and the "⋯" next to it does
// what is done to the current profile as a whole.
export default function ProfileSwitcher({ game, profiles, onGameChanged, reload, onError }: Props) {
  const [list, setList] = useState<MenuAnchor | null>(null);
  const [actions, setActions] = useState<MenuAnchor | null>(null);
  const [renaming, setRenaming] = useState(false);
  const [creating, setCreating] = useState(false);
  const [exporting, setExporting] = useState(false);
  const { moving, setActive, copy, rename, move, moveTo, remove } = useProfileActions(game, reload, onGameChanged, onError);
  // Profiles of games whose mods are deployed are not exported yet.
  const modList = deployed(game.engine);
  const current = profiles.find((p) => p.id === game.activeProfile) ?? null;
  const folder = current ? game.profileDirs?.[current.id] : undefined;

  // A profile made or brought in here becomes the one in use.
  const added = async (profileId: string) => {
    try {
      onGameChanged(await Library.SetActiveProfile(game.id, profileId));
      await reload();
    } catch (err) {
      onError(errorMessage(err));
    }
  };

  const pick = (action: () => void) => () => {
    setList(null);
    setActions(null);
    action();
  };

  return (
    <div className="flex flex-col gap-1">
      <span className="px-1 text-xs font-medium text-zinc-500">Profile</span>
      <div className="flex gap-2">
        {renaming && current ? (
          <input
            autoFocus
            defaultValue={current.name}
            className={inputClass}
            onBlur={(e) => {
              setRenaming(false);
              rename(current, e.target.value);
            }}
            onKeyDown={(e) => {
              if (e.key === "Enter") e.currentTarget.blur();
              if (e.key === "Escape") setRenaming(false);
            }}
          />
        ) : (
          <button
            onClick={(e) => setList(anchorBelow(e.currentTarget))}
            className="flex min-w-0 flex-1 items-center gap-2 rounded-lg border border-zinc-800 px-3 py-2 text-left hover:bg-zinc-800"
          >
            <span className="flex min-w-0 flex-1 flex-col">
              <span className="truncate text-sm font-medium">{current?.name ?? "…"}</span>
              {moving === current?.id ? (
                <span className="text-xs text-zinc-500">Moving…</span>
              ) : current?.modpack ? (
                <span className="truncate text-xs text-accent-400">Modpack {packageLabel(current.modpack)}</span>
              ) : (
                folder && (
                  <span className="truncate text-xs text-zinc-500" title={folder}>
                    {folder}
                  </span>
                )
              )}
            </span>
            <ChevronDownIcon />
          </button>
        )}
        <button
          aria-label="Profile actions"
          title="Profile actions"
          disabled={!current}
          onClick={(e) => setActions(anchorBelow(e.currentTarget))}
          className="flex w-9 shrink-0 items-center justify-center rounded-md border border-zinc-700 text-zinc-400 hover:bg-zinc-800 hover:text-zinc-100 disabled:opacity-40"
        >
          <DotsIcon />
        </button>
      </div>

      {list && (
        <ContextMenu at={list} onClose={() => setList(null)} width="w-64">
          {profiles.map((p) => {
            const count = p.mods?.filter((m) => !m.separator).length ?? 0;
            return (
              <MenuItem key={p.id} selected={p.id === game.activeProfile} onClick={pick(() => setActive(p))}>
                <span className="flex-1 truncate">{p.name}</span>
                <span className="shrink-0 text-xs text-zinc-500">
                  {count} {count === 1 ? "mod" : "mods"}
                </span>
              </MenuItem>
            );
          })}
          <MenuDivider />
          <MenuItem onClick={pick(() => setCreating(true))}>
            <PlusIcon />
            New profile…
          </MenuItem>
        </ContextMenu>
      )}

      {actions && current && (
        <ContextMenu at={actions} onClose={() => setActions(null)} width="w-56">
          <MenuItem onClick={pick(() => setRenaming(true))}>Rename</MenuItem>
          <MenuItem title="Creates a profile with the same mods and configs" onClick={pick(() => copy(current))}>
            Duplicate
          </MenuItem>
          <MenuItem
            title="Keeps the profile and its mods in a folder you choose, e.g. on the disk the game is on"
            disabled={moving !== null}
            onClick={pick(() => moveTo(current))}
          >
            Move to folder…
          </MenuItem>
          {folder && (
            <MenuItem disabled={moving !== null} onClick={pick(() => move(current, ""))}>
              Move back into Hermit
            </MenuItem>
          )}
          <MenuItem
            onClick={pick(() => FolderService.OpenProfile(game.id, current.id).catch((err) => onError(errorMessage(err))))}
          >
            Open folder
          </MenuItem>
          {!modList && <MenuItem onClick={pick(() => setExporting(true))}>Export…</MenuItem>}
          <MenuDivider />
          <MenuItem
            danger
            disabled={profiles.length <= 1}
            title={profiles.length <= 1 ? "A game keeps at least one profile" : undefined}
            onClick={pick(() => remove(current))}
          >
            Remove
          </MenuItem>
        </ContextMenu>
      )}

      {creating && (
        <CreateProfileModal
          game={game}
          onClose={() => setCreating(false)}
          onCreated={(profileId) => {
            setCreating(false);
            added(profileId);
          }}
        />
      )}
      {exporting && current && <ExportModal game={game} profile={current} onClose={() => setExporting(false)} />}
    </div>
  );
}
