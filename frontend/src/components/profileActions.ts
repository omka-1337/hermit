import { useState } from "react";
import { Dialogs } from "@wailsio/runtime";
import { confirmDanger, Game, Library, Profile } from "../api";

// useProfileActions does what can be done to a profile of a game: making it
// active, copying, renaming, moving it to another folder and removing it.
// reload re-reads the profiles once one has changed; failures go to onError.
export function useProfileActions(
  game: Game,
  reload: () => Promise<void>,
  onGameChanged: (game: Game) => void,
  onError: (message: string) => void,
) {
  // moving is the profile being moved; across disks that copies every mod.
  const [moving, setMoving] = useState<string | null>(null);

  const run = async (action: () => Promise<void>) => {
    try {
      await action();
    } catch (err) {
      onError(err instanceof Error ? err.message : String(err));
    }
  };

  const refreshGame = async () => onGameChanged(await Library.GetGame(game.id));

  const setActive = (p: Profile) => run(async () => onGameChanged(await Library.SetActiveProfile(game.id, p.id)));

  const copy = (p: Profile) =>
    run(async () => {
      await Library.CopyProfile(game.id, p.id, `${p.name} copy`);
      await reload();
    });

  const rename = (p: Profile, name: string) =>
    run(async () => {
      if (!name.trim() || name === p.name) return;
      await Library.RenameProfile(game.id, p.id, name.trim());
      await reload();
    });

  // move puts a profile into a folder of its own inside location, or back
  // into Hermit's data when location is empty.
  const move = (p: Profile, location: string) =>
    run(async () => {
      setMoving(p.id);
      try {
        await Library.MoveProfile(game.id, p.id, location);
      } finally {
        setMoving(null);
        await Promise.all([reload(), refreshGame()]);
      }
    });

  const moveTo = (p: Profile) =>
    run(async () => {
      const location = await Dialogs.OpenFile({
        Title: `Choose where to keep "${p.name}"`,
        CanChooseDirectories: true,
        CanChooseFiles: false,
        CanCreateDirectories: true,
      });
      if (location) await move(p, location);
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
      await Promise.all([reload(), refreshGame()]);
    });

  return { moving, setActive, copy, rename, move, moveTo, remove };
}
