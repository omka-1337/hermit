import { useCallback, useEffect, useState } from "react";
import { Dialogs, Events } from "@wailsio/runtime";
import { BethesdaService, confirmDanger, Engine, errorMessage, Game, Library, Profile, UnpackProgress } from "../api";
import { installFromFileEvent } from "../components/TitleMenus";
import GamePicker from "../components/GamePicker";
import { LaunchSetup } from "../components/launch";
import { onModpackSettled } from "../modpackInstalls";
import ProfileView, { profileTabs, Tab } from "../components/profile/ProfileView";
import GameSwitcher from "../components/sidebar/GameSwitcher";
import ProfileSwitcher from "../components/sidebar/ProfileSwitcher";
import { TabIcon } from "../components/sidebar/icons";
import { Button, ErrorText, Modal } from "../components/ui";
import { formatBytes } from "../format";
import SettingsView from "./SettingsView";
import { GearIcon } from "./shell";

type Props = {
  games: Game[];
  selectedId: string | null;
  onSelect: (id: string | null) => void;
  onGamesChanged: (selectId?: string | null) => Promise<void>;
};

// DesktopWorkspace is the mouse-and-keyboard layout. The sidebar picks the
// game, then its profile, which is the active one, then a page of that
// profile, top to bottom; the page fills the rest of the window.
export default function DesktopWorkspace({ games, selectedId, onSelect, onGamesChanged }: Props) {
  const [settings, setSettings] = useState(false);
  const [adding, setAdding] = useState(false);
  const [tab, setTab] = useState<Tab>("installed");
  const [profiles, setProfiles] = useState<Profile[]>([]);
  const [error, setError] = useState("");
  // unpacking is an archive being unpacked for File → Install from file.
  const [unpacking, setUnpacking] = useState<UnpackProgress | null>(null);
  const [addRequest, setAddRequest] = useState(0);
  const game = games.find((g) => g.id === selectedId) ?? null;
  const profile = profiles.find((p) => p.id === game?.activeProfile) ?? null;
  const tabs = game ? profileTabs(game, profile, true) : [];

  const gameId = game?.id;
  const loadProfiles = useCallback(async () => {
    if (!gameId) return setProfiles([]);
    setProfiles((await Library.ListProfiles(gameId)) ?? []);
  }, [gameId]);

  useEffect(() => {
    setError("");
    loadProfiles().catch((err) => setError(errorMessage(err)));
  }, [loadProfiles]);

  // A modpack keeps installing after its dialog is closed; its profile shows
  // up once it is done.
  useEffect(
    () =>
      onModpackSettled((settled) => {
        if (settled.gameId === gameId) loadProfiles().catch((err) => setError(errorMessage(err)));
      }),
    [gameId, loadProfiles],
  );

  // A game without the page that was open, a Creation Engine game has no
  // Browse, starts on its first one.
  const tabIds = tabs.map((t) => t.id).join(" ");
  useEffect(() => {
    const ids = tabIds.split(" ") as Tab[];
    if (tabIds && !ids.includes(tab)) setTab(ids[0]);
  }, [tabIds, tab]);

  const changed = (g: Game) => onGamesChanged(g.id);

  // File → Install from file: a Bethesda game's archive is unpacked here and
  // installed, through its FOMOD installer's window when it has one; other
  // games add the file in the dialog the Installed page has for it.
  useEffect(() => {
    const install = async () => {
      if (!game) return;
      setSettings(false);
      if (game.engine !== Engine.EngineCreation) {
        setTab("installed");
        setAddRequest((n) => n + 1);
        return;
      }
      const file = await Dialogs.OpenFile({
        Title: "Choose a mod archive",
        CanChooseFiles: true,
        Filters: [{ DisplayName: "Mod archives (zip, 7z, rar)", Pattern: "*.zip;*.7z;*.rar" }],
      });
      if (!file) return;
      setError("");
      setUnpacking({ file: file.split("/").pop() ?? file, done: 0, total: 0 });
      try {
        const prepared = await BethesdaService.PrepareInstall(game.id, game.activeProfile, file);
        if (prepared.fomod) await BethesdaService.OpenFomod(prepared.session);
        else await BethesdaService.FinishInstall(prepared.session, null);
        setTab("installed");
      } catch (err) {
        setError(errorMessage(err));
      } finally {
        setUnpacking(null);
      }
    };
    window.addEventListener(installFromFileEvent, install);
    return () => window.removeEventListener(installFromFileEvent, install);
  }, [game]);

  useEffect(() => Events.On("bethesda:unpack", (ev) => setUnpacking((u) => (u ? ev.data : u))), []);

  // A mod installed from a FOMOD installer's window changes the profile's count.
  useEffect(
    () =>
      Events.On("bethesda:installed", (ev) => {
        if (ev.data?.gameId === gameId) loadProfiles().catch(console.error);
      }),
    [gameId, loadProfiles],
  );

  const removeGame = async () => {
    if (!game) return;
    const ok = await confirmDanger(
      "Remove game",
      `Remove "${game.name}" with all its profiles and installed mods? The game itself will not be touched.`,
      "Remove",
    );
    if (!ok) return;
    try {
      await Library.RemoveGame(game.id);
      await onGamesChanged(null);
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  // The page's view of the profile is newer than the sidebar's list.
  const profileChanged = (p: Profile) => setProfiles((list) => list.map((q) => (q.id === p.id ? p : q)));

  return (
    <div className="flex h-full">
      <aside className="flex w-64 shrink-0 flex-col gap-4 border-r border-zinc-800 bg-zinc-900 p-3">
        <GameSwitcher
          games={games}
          selected={game}
          onSelect={(id) => {
            setSettings(false);
            onSelect(id);
          }}
          onAdd={() => setAdding(true)}
          onChanged={changed}
          onRemove={removeGame}
        />
        {game && (
          <ProfileSwitcher
            game={game}
            profiles={profiles}
            onGameChanged={changed}
            reload={loadProfiles}
            onError={setError}
          />
        )}
        {game && (
          <nav className="flex flex-col gap-0.5">
            {tabs.map((t) => (
              <button
                key={t.id}
                onClick={() => {
                  setSettings(false);
                  setTab(t.id);
                }}
                className={`flex items-center gap-2.5 rounded-md px-3 py-2 text-left text-sm ${
                  !settings && tab === t.id ? "bg-zinc-800 font-medium text-zinc-100" : "text-zinc-400 hover:bg-zinc-800/60 hover:text-zinc-200"
                }`}
              >
                <TabIcon tab={t.id} />
                {t.label}
              </button>
            ))}
          </nav>
        )}
        <div className="flex-1" />
        <button
          onClick={() => setSettings(true)}
          className={`flex items-center gap-2 rounded-md px-3 py-2 text-left text-sm ${
            settings ? "bg-zinc-800 text-zinc-100" : "text-zinc-400 hover:bg-zinc-800/60 hover:text-zinc-200"
          }`}
        >
          <GearIcon />
          Settings
        </button>
      </aside>

      <main className="flex min-w-0 flex-1 flex-col overflow-hidden">
        {error && (
          <div className="px-6 pt-4">
            <ErrorText>{error}</ErrorText>
          </div>
        )}
        {settings ? (
          <div className="min-h-0 flex-1 overflow-auto">
            <SettingsView />
          </div>
        ) : game && profile ? (
          <>
            <div className="shrink-0 px-6 empty:hidden [&>section]:pt-4">
              <LaunchSetup game={game} />
            </div>
            <div className="min-h-0 flex-1">
              <ProfileView
                key={`${game.id}/${profile.id}`}
                game={game}
                profileId={profile.id}
                tab={tab}
                onBack={() => {}}
                onGameChanged={() => onGamesChanged(game.id)}
                onOpenProfile={async (profileId) => changed(await Library.SetActiveProfile(game.id, profileId))}
                onProfileChange={profileChanged}
                addRequest={addRequest}
              />
            </div>
          </>
        ) : (
          !game && (
            <div className="flex h-full flex-col items-center justify-center gap-3 text-center">
              <p className="text-zinc-400">{games.length ? "Choose a game in the top left" : "No games added yet"}</p>
              {!games.length && (
                <Button variant="primary" onClick={() => setAdding(true)}>
                  Add game
                </Button>
              )}
            </div>
          )
        )}
      </main>

      {unpacking && (
        <Modal title="Installing" onClose={() => {}}>
          <div className="flex flex-col gap-3">
            <span className="truncate text-sm">Unpacking {unpacking.file}…</span>
            <div className="h-2 overflow-hidden rounded-full bg-zinc-800">
              <div
                className="h-full bg-indigo-500 transition-[width]"
                style={{ width: unpacking.total ? `${Math.round((unpacking.done / unpacking.total) * 100)}%` : "0%" }}
              />
            </div>
            {unpacking.total > 0 && (
              <span className="text-xs text-zinc-500">
                {formatBytes(unpacking.done)} of {formatBytes(unpacking.total)}
              </span>
            )}
          </div>
        </Modal>
      )}

      {adding && (
        <Modal title="Add game" size="lg" onClose={() => setAdding(false)}>
          <GamePicker
            onAdded={async (g) => {
              setAdding(false);
              setSettings(false);
              onSelect(g.id);
              await onGamesChanged(g.id);
            }}
            actions={
              <Button variant="ghost" onClick={() => setAdding(false)}>
                Close
              </Button>
            }
          />
        </Modal>
      )}
    </div>
  );
}
