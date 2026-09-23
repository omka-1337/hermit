import { useCallback, useEffect, useState } from "react";
import { confirmDanger, errorMessage, Game, Library, Profile } from "../api";
import CreateProfileModal from "./CreateProfileModal";
import EditableName from "./EditableName";
import GameIcon from "./GameIcon";
import GameSettings from "./GameSettings";
import { onModpackSettled } from "../modpackInstalls";
import ProfileList from "./ProfileList";
import { LaunchSetup, PlayButton } from "./launch";
import { ImportModal } from "./share";
import { BackendBadge, Button, ErrorText, RuntimeBadge } from "./ui";

type Props = {
  game: Game;
  onChanged: (game: Game) => void;
  onRemoved: () => void;
  onOpenProfile: (profileId: string) => void;
};

export default function GameView({ game, onChanged, onRemoved, onOpenProfile }: Props) {
  const [profiles, setProfiles] = useState<Profile[]>([]);
  const [loadingProfiles, setLoadingProfiles] = useState(true);
  const [error, setError] = useState("");
  const [importing, setImporting] = useState(false);
  const [creating, setCreating] = useState(false);

  const run = async (action: () => Promise<void>) => {
    setError("");
    try {
      await action();
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  const loadProfiles = useCallback(async () => {
    try {
      setProfiles((await Library.ListProfiles(game.id)) ?? []);
    } finally {
      setLoadingProfiles(false);
    }
  }, [game.id]);

  useEffect(() => {
    loadProfiles().catch((err) => setError(errorMessage(err)));
  }, [loadProfiles]);

  // A modpack keeps installing after its dialog is closed; its profile shows
  // up here once it is done.
  useEffect(
    () =>
      onModpackSettled((settled) => {
        if (settled.gameId === game.id) loadProfiles().catch((err) => setError(errorMessage(err)));
      }),
    [game.id, loadProfiles],
  );

  const refreshGame = async () => onChanged(await Library.GetGame(game.id));

  const removeGame = () =>
    run(async () => {
      const ok = await confirmDanger(
        "Remove game",
        `Remove "${game.name}" with all its profiles and installed mods? The game itself will not be touched.`,
        "Remove",
      );
      if (!ok) return;
      await Library.RemoveGame(game.id);
      onRemoved();
    });


  return (
    <div className="mx-auto flex max-w-3xl flex-col gap-6 p-8">
      <header className="flex items-start justify-between gap-4">
        <GameIcon name={game.name} steamAppId={game.steamAppId} size={56} />
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <EditableName
            value={game.name}
            className="text-2xl font-semibold"
            onSave={(name) => run(async () => onChanged(await Library.RenameGame(game.id, name)))}
          />
          <div className="flex items-center gap-2 text-sm text-zinc-500">
            <RuntimeBadge runtime={game.runtime} />
            <BackendBadge backend={game.backend} />
            <span className="min-w-0 truncate select-text">{game.path}</span>
          </div>
        </div>
        <div className="flex items-start gap-2">
          {/* The gear matches the Play button's height. */}
          <div className="flex self-stretch">
            <GameSettings game={game} onRemove={removeGame} />
          </div>
          <PlayButton game={game} profileId={game.activeProfile} onPlayed={refreshGame} />
        </div>
      </header>

      <ErrorText>{error}</ErrorText>

      <section className="flex flex-col gap-3">
        <h2 className="text-sm font-semibold tracking-wide text-zinc-400 uppercase">Launch</h2>
        <LaunchSetup game={game} />
      </section>

      <section className="flex flex-col gap-3">
        <h2 className="text-sm font-semibold tracking-wide text-zinc-400 uppercase">Profiles</h2>
        <ProfileList
          game={game}
          profiles={profiles}
          loading={loadingProfiles}
          onOpenProfile={onOpenProfile}
          onGameChanged={onChanged}
          reload={loadProfiles}
          onError={setError}
        />
        <div className="flex gap-2">
          <Button variant="primary" onClick={() => setCreating(true)}>
            Create a new profile
          </Button>
          <Button variant="ghost" onClick={() => setImporting(true)}>
            Import…
          </Button>
        </div>
      </section>

      {creating && (
        <CreateProfileModal
          game={game}
          onClose={() => setCreating(false)}
          onCreated={(profileId, open) => {
            setCreating(false);
            if (open) onOpenProfile(profileId);
            else loadProfiles();
          }}
        />
      )}

      {importing && (
        <ImportModal
          game={game}
          onClose={() => {
            setImporting(false);
            loadProfiles();
          }}
          onImported={(profileId) => {
            setImporting(false);
            onOpenProfile(profileId);
          }}
        />
      )}
    </div>
  );
}
