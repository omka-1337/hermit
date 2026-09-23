import { useState } from "react";
import { confirmDanger, Game, Library, Profile } from "../api";
import { packageLabel } from "../format";
import ContextMenu, { MenuAnchor, MenuItem } from "./ContextMenu";
import { inputClass } from "./ui";

type Props = {
  game: Game;
  profiles: Profile[];
  loading: boolean;
  onOpenProfile: (profileId: string) => void;
  onGameChanged: (game: Game) => void;
  // reload re-reads the profiles after one was added, renamed or removed.
  reload: () => Promise<void>;
  onError: (message: string) => void;
};

// ProfileList shows the profiles of a game as cards: a card opens its profile,
// and everything else about it — which one is active, renaming, copying,
// removing — waits in the menu a right click opens, so the card itself stays
// one plain thing to click.
export default function ProfileList({
  game,
  profiles,
  loading,
  onOpenProfile,
  onGameChanged,
  reload,
  onError,
}: Props) {
  const [menu, setMenu] = useState<{ profile: Profile; at: MenuAnchor } | null>(null);
  const [renaming, setRenaming] = useState<string | null>(null);

  const run = async (action: () => Promise<void>) => {
    try {
      await action();
    } catch (err) {
      onError(err instanceof Error ? err.message : String(err));
    }
  };

  const open = (profile: Profile, e: { clientX: number; clientY: number }) =>
    setMenu({ profile, at: { x: e.clientX, y: e.clientY } });

  const setActive = (p: Profile) =>
    run(async () => onGameChanged(await Library.SetActiveProfile(game.id, p.id)));

  const copy = (p: Profile) =>
    run(async () => {
      await Library.CopyProfile(game.id, p.id, `${p.name} copy`);
      await reload();
    });

  const rename = (p: Profile, name: string) =>
    run(async () => {
      setRenaming(null);
      if (!name.trim() || name === p.name) return;
      await Library.RenameProfile(game.id, p.id, name.trim());
      await reload();
    });

  const remove = (p: Profile) =>
    run(async () => {
      const ok = await confirmDanger(
        "Remove profile",
        `Remove profile "${p.name}" with all its installed mods and configs?`,
        "Remove",
      );
      if (!ok) return;
      await Library.RemoveProfile(game.id, p.id);
      await Promise.all([reload(), (async () => onGameChanged(await Library.GetGame(game.id)))()]);
    });

  return (
    <>
      <ul className="flex flex-col gap-2">
        {loading && profiles.length === 0 && (
          <li className="h-[58px] animate-pulse rounded-lg border border-zinc-800 bg-zinc-800/40" />
        )}
        {profiles.map((p) => {
          const active = p.id === game.activeProfile;
          const modCount = p.mods?.length ?? 0;
          return (
            <li key={p.id} className="relative">
              {/* The active profile shows a strip sticking out from under its card. */}
              {active && (
                <span
                  aria-hidden
                  title="Active profile"
                  className="absolute inset-y-2 -left-1.5 w-3 rounded-md bg-indigo-500"
                />
              )}
              <div
                className="relative flex items-stretch overflow-hidden rounded-lg border border-zinc-800 bg-zinc-900 transition-colors hover:border-zinc-700"
                onContextMenu={(e) => {
                  e.preventDefault();
                  open(p, e);
                }}
              >
                {renaming === p.id ? (
                  <input
                    autoFocus
                    defaultValue={p.name}
                    className={`${inputClass} m-2`}
                    onBlur={(e) => rename(p, e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") e.currentTarget.blur();
                      if (e.key === "Escape") setRenaming(null);
                    }}
                  />
                ) : (
                  <button
                    data-focus-first={active || undefined}
                    onClick={() => onOpenProfile(p.id)}
                    className="flex min-w-0 flex-1 items-center gap-3 px-4 py-3 text-left hover:bg-zinc-800/50"
                  >
                    <span className="flex min-w-0 flex-col">
                      <span className="truncate text-sm font-medium">{p.name}</span>
                      {p.modpack && (
                        <span className="truncate text-xs text-indigo-400">Modpack {packageLabel(p.modpack)}</span>
                      )}
                    </span>
                    <span className="ml-auto shrink-0 text-xs text-zinc-500">
                      {modCount} {modCount === 1 ? "mod" : "mods"}
                    </span>
                  </button>
                )}
                {/* Right click has no touch or controller equivalent, so the
                    same menu has a button of its own. */}
                <button
                  aria-label={`Options for ${p.name}`}
                  title="Profile options"
                  onClick={(e) => open(p, e)}
                  className="px-2 text-zinc-500 hover:bg-zinc-800/50 hover:text-zinc-100"
                >
                  <DotsIcon />
                </button>
                <span aria-hidden className="flex items-center pr-3 pl-1 text-zinc-600">
                  <ChevronIcon />
                </span>
              </div>
            </li>
          );
        })}
      </ul>

      {menu && (
        <ContextMenu at={menu.at} onClose={() => setMenu(null)}>
          {menu.profile.id !== game.activeProfile && (
            <MenuItem
              onClick={() => {
                setActive(menu.profile);
                setMenu(null);
              }}
            >
              Set active
            </MenuItem>
          )}
          <MenuItem
            onClick={() => {
              setRenaming(menu.profile.id);
              setMenu(null);
            }}
          >
            Rename
          </MenuItem>
          <MenuItem
            title="Creates a profile with the same mods and configs"
            onClick={() => {
              copy(menu.profile);
              setMenu(null);
            }}
          >
            Duplicate
          </MenuItem>
          <MenuItem
            danger
            disabled={profiles.length <= 1}
            title={profiles.length <= 1 ? "A game keeps at least one profile" : undefined}
            onClick={() => {
              remove(menu.profile);
              setMenu(null);
            }}
          >
            Remove
          </MenuItem>
        </ContextMenu>
      )}
    </>
  );
}

function ChevronIcon() {
  return (
    <svg viewBox="0 0 24 24" className="h-5 w-5" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="m9 18 6-6-6-6" />
    </svg>
  );
}

function DotsIcon() {
  return (
    <svg viewBox="0 0 24 24" className="h-5 w-5" fill="currentColor" aria-hidden>
      <circle cx="12" cy="5" r="1.6" />
      <circle cx="12" cy="12" r="1.6" />
      <circle cx="12" cy="19" r="1.6" />
    </svg>
  );
}
