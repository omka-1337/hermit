import { useCallback, useEffect, useRef, useState } from "react";
import { Dialogs, Events } from "@wailsio/runtime";
import { confirmDanger, deployed, Download, DownloadService, errorMessage, Game, Library, Profile } from "../api";
import { stageColors, useDownloads, useNexusSetup } from "../downloads";
import { QueueStatus, useInstallQueue } from "../installQueue";
import { installFromFileEvent } from "../components/TitleMenus";
import GamePicker from "../components/GamePicker";
import { LaunchSetup } from "../components/launch";
import { onModpackSettled } from "../modpackInstalls";
import ProfileView, { profileTabs, Tab } from "../components/profile/ProfileView";
import GameSwitcher from "../components/sidebar/GameSwitcher";
import ProfileSwitcher from "../components/sidebar/ProfileSwitcher";
import { TabIcon } from "../components/sidebar/icons";
import { Button, ErrorText, Modal } from "../components/ui";
import { ResizeHandle, usePanelWidth } from "../panelWidth";
import { requestRefresh, useRefresh } from "../refresh";
import DownloadsView from "./DownloadsView";
import BrowserPage, { openBrowserEvent } from "./BrowserPage";
import SettingsView from "./SettingsView";
import { GearIcon } from "./shell";
import nexusLogo from "../assets/nexus.svg";
import thunderstoreLogo from "../assets/thunderstore.svg";

type Page = "profile" | "settings" | "downloads" | "browser";

type Props = {
  games: Game[];
  selectedId: string | null;
  onSelect: (id: string | null) => void;
  onGamesChanged: (selectId?: string | null) => Promise<void>;
};

// DesktopWorkspace is the mouse-and-keyboard layout. The sidebar picks the
// game, then its profile, which is the active one, then a page of that
// profile, top to bottom; the page fills the rest of the window, Play in its
// top right corner. Browse, beside the mark of the site its mods come from,
// and then Downloads and Settings, pages of their own, are at the bottom of
// the sidebar. A status bar along the bottom counts the profile's
// mods and follows downloads and installs.
export default function DesktopWorkspace({ games, selectedId, onSelect, onGamesChanged }: Props) {
  // page is what fills the window beside the sidebar: the profile's page
  // picked in the sidebar, Settings, Downloads, or a page of Nexus Mods in
  // the browser, which goes back to returnTo when it is done.
  const [page, setPage] = useState<Page>("profile");
  const [browserURL, setBrowserURL] = useState("");
  const [returnTo, setReturnTo] = useState<Page>("profile");
  // The page and game as the event handlers see them, between renders.
  const pageRef = useRef(page);
  pageRef.current = page;
  const gameRef = useRef(selectedId);
  gameRef.current = selectedId;
  const elsewhere = page !== "profile";
  const [adding, setAdding] = useState(false);
  const [tab, setTab] = useState<Tab>("installed");
  const [profiles, setProfiles] = useState<Profile[]>([]);
  const [error, setError] = useState("");
  const [addRequest, setAddRequest] = useState(0);
  const queue = useInstallQueue(setError);
  const downloads = useDownloads();
  const nexus = useNexusSetup();
  // The sidebar is as wide as it was dragged to: 16rem at first.
  const sidebar = usePanelWidth("sidebar", 256, 200, 480);
  const game = games.find((g) => g.id === selectedId) ?? null;
  const profile = profiles.find((p) => p.id === game?.activeProfile) ?? null;
  const tabs = game ? profileTabs(game, profile, true) : [];
  const browse = tabs.some((t) => t.id === "browse");

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

  // A game without the page that was open, one whose mods are deployed has
  // no Browse, starts on its first one.
  const tabIds = tabs.map((t) => t.id).join(" ");
  useEffect(() => {
    const ids = tabIds.split(" ") as Tab[];
    if (tabIds && !ids.includes(tab)) setTab(ids[0]);
  }, [tabIds, tab]);

  const changed = (g: Game) => onGamesChanged(g.id);

  // File → Install from file: the archives of a game whose mods are
  // deployed, as many as are picked, join the queue of installs; other games
  // add the file in the dialog the Installed page has for it.
  useEffect(() => {
    const install = async () => {
      if (!game) return;
      setPage("profile");
      if (!deployed(game.engine)) {
        setTab("installed");
        setAddRequest((n) => n + 1);
        return;
      }
      const files = await Dialogs.OpenFile({
        Title: "Choose mod archives",
        CanChooseFiles: true,
        AllowsMultipleSelection: true,
        Filters: [{ DisplayName: "Mod archives (zip, 7z, rar)", Pattern: "*.zip;*.7z;*.rar" }],
      });
      if (!files?.length) return;
      setError("");
      setTab("installed");
      queue.add(game.id, game.activeProfile, files).catch((err) => setError(errorMessage(err)));
    };
    window.addEventListener(installFromFileEvent, install);
    return () => window.removeEventListener(installFromFileEvent, install);
  }, [game, queue]);

  // A page of Nexus Mods opens in the browser; once it hands a file over,
  // or is closed, the user is back where they opened it from.
  useEffect(() => {
    const open = (e: Event) => {
      setBrowserURL((e as CustomEvent<string>).detail);
      if (pageRef.current !== "browser") setReturnTo(pageRef.current);
      setPage("browser");
    };
    window.addEventListener(openBrowserEvent, open);
    const offDownload = Events.On("browser:download", () => {
      if (pageRef.current === "browser") setPage(returnTo);
    });
    return () => {
      window.removeEventListener(openBrowserEvent, open);
      offDownload();
    };
  }, [returnTo]);

  // F5 reads everything on screen again, as in MO2; the web view would
  // otherwise reload the whole interface.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "F5") return;
      e.preventDefault();
      requestRefresh();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);
  useRefresh(() => loadProfiles().catch((err) => setError(errorMessage(err))));

  // A mod installed from a FOMOD installer's window changes the profile's count.
  useEffect(
    () =>
      Events.On("modlist:installed", (ev) => {
        if (ev.data?.gameId === gameId) loadProfiles().catch(console.error);
      }),
    [gameId, loadProfiles],
  );

  const active = downloads.filter((d) => d.status === "queued" || d.status === "downloading");

  // Coming back from Settings, downloads may be set up.
  useEffect(() => {
    if (page !== "settings") nexus.reload();
  }, [page]);

  // A finished download goes into the queue of installs of the profile it is
  // for, which comes up; so do downloads that finished before this window was
  // there to see them.
  const queueAdd = queue.add;
  useEffect(() => {
    const take = async () => {
      for (const d of (await DownloadService.TakeFinished()) ?? []) {
        // A download for the game open installs where the user is, in the
        // background, as one started from Browse keeps them browsing; one for
        // another game opens that game's mods.
        if (d.gameId !== gameRef.current) {
          onSelect(d.gameId);
          setPage("profile");
          setTab("installed");
        }
        queueAdd(d.gameId, d.profileId, [d.path]).catch((err) => setError(errorMessage(err)));
      }
    };
    take().catch(console.error);
    const offFinished = Events.On("downloads:finished", () => take().catch(console.error));
    // Why a link from Nexus Mods was not downloaded.
    const offError = Events.On("downloads:error", (ev) => setError(String(ev.data)));
    DownloadService.TakeErrors()
      .then((errs) => errs?.length && setError(errs.join("\n")))
      .catch(console.error);
    return () => {
      offFinished();
      offError();
    };
  }, []);

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
    <div className="flex h-full flex-col">
      <div className="flex min-h-0 flex-1">
        <aside
          style={{ width: sidebar.width }}
          className="relative flex shrink-0 flex-col gap-4 border-r border-zinc-800 bg-zinc-900 p-3"
        >
          <ResizeHandle edge="right" panel={sidebar} label="Sidebar width" />
          <GameSwitcher
            games={games}
            selected={game}
            onSelect={(id) => {
              setPage("profile");
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
              {tabs.filter((t) => t.id !== "browse").map((t) => (
                <button
                  key={t.id}
                  onClick={() => {
                    setPage("profile");
                    setTab(t.id);
                  }}
                  className={`flex items-center gap-2.5 rounded-md px-3 py-2 text-left text-sm ${
                    !elsewhere && tab === t.id ? "bg-zinc-800 font-medium text-zinc-100" : "text-zinc-400 hover:bg-zinc-800/60 hover:text-zinc-200"
                  }`}
                >
                  <TabIcon tab={t.id} />
                  {t.label}
                </button>
              ))}
            </nav>
          )}
          <div className="flex-1" />
          {game && browse && (
            <button
              onClick={() => {
                setPage("profile");
                setTab("browse");
              }}
              className={`flex items-center gap-2.5 rounded-md px-3 py-2 text-left text-sm ${
                !elsewhere && tab === "browse" ? "bg-zinc-800 font-medium text-zinc-100" : "text-zinc-400 hover:bg-zinc-800/60 hover:text-zinc-200"
              }`}
            >
              {deployed(game.engine) ? (
                <img src={nexusLogo} alt="Nexus Mods" className="h-4 w-4 shrink-0" />
              ) : (
                <img src={thunderstoreLogo} alt="Thunderstore" className="h-4 w-4 shrink-0" />
              )}
              Browse
            </button>
          )}
          <div className={`grid grid-cols-2 gap-2 ${browse && game ? "-mt-2" : ""}`}>
            <button
              onClick={() => setPage("downloads")}
              title="Downloads"
              aria-label="Downloads"
              className={`relative flex h-9 items-center justify-center rounded-md border ${
                page === "downloads"
                  ? "border-zinc-600 bg-zinc-800 text-zinc-100"
                  : "border-zinc-700 text-zinc-400 hover:bg-zinc-800/60 hover:text-zinc-200"
              }`}
            >
              <DownloadIcon />
              {active.length > 0 && (
                <span className="absolute -right-1.5 -top-1.5 min-w-4 rounded-full bg-accent-500 px-1 text-center text-[10px] leading-4 font-semibold text-zinc-950 tabular-nums">
                  {active.length}
                </span>
              )}
            </button>
            <button
              onClick={() => setPage("settings")}
              title="Settings"
              aria-label="Settings"
              className={`flex h-9 items-center justify-center rounded-md border ${
                page === "settings"
                  ? "border-zinc-600 bg-zinc-800 text-zinc-100"
                  : "border-zinc-700 text-zinc-400 hover:bg-zinc-800/60 hover:text-zinc-200"
              }`}
            >
              <GearIcon />
            </button>
          </div>
        </aside>

        <main className="flex min-w-0 flex-1 flex-col overflow-hidden">
          {error && (
            <div className="px-6 pt-4">
              <ErrorText>{error}</ErrorText>
            </div>
          )}
          {page === "settings" && (
            <div className="min-h-0 flex-1 overflow-auto">
              <SettingsView />
            </div>
          )}
          {page === "downloads" && (
            <div className="min-h-0 flex-1 overflow-auto">
              <DownloadsView onSetUp={() => setPage("settings")} />
            </div>
          )}
          {page === "browser" && (
            <div className="min-h-0 flex-1">
              <BrowserPage url={browserURL} onClose={() => setPage(returnTo)} />
            </div>
          )}
          {/* The profile's page stays while another is shown, so that it is
              as it was when the user comes back: the mod picked, the search. */}
          {game && profile ? (
            <div className={page === "profile" ? "contents" : "hidden"}>
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
            </div>
          ) : (
            page === "profile" &&
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
      </div>
      {game && deployed(game.engine) && !elsewhere && nexus.account && nexus.handler && !nexus.ready && (
        <div className="flex shrink-0 items-center gap-3 border-t border-amber-500/30 bg-amber-500/5 px-3 py-1.5 text-xs text-amber-300">
          <span className="flex-1">
            Nexus Mods downloads are not set up:{" "}
            {!nexus.account.hasKey
              ? "Hermit is not logged in to Nexus Mods."
              : "the Mod Manager Download buttons do not open in Hermit."}
          </span>
          {nexus.account.hasKey ? (
            <button
              className="font-medium hover:underline"
              onClick={() =>
                DownloadService.RegisterNXM()
                  .then(nexus.reload)
                  .catch((err) => setError(errorMessage(err)))
              }
            >
              Open them in Hermit
            </button>
          ) : (
            <button className="font-medium hover:underline" onClick={() => setPage("settings")}>
              Log in
            </button>
          )}
        </div>
      )}
      <StatusBar profile={game && !elsewhere ? profile : null} queue={queue.status} downloads={active} />

      {queue.dialog}

      {adding && (
        <Modal title="Add game" size="lg" onClose={() => setAdding(false)}>
          <GamePicker
            onAdded={async (g) => {
              setAdding(false);
              setPage("profile");
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

// StatusBar runs along the bottom of the window: how many mods the profile
// has and, in its middle, the work under way: how many downloads and
// archives are left in the queue, a bar in the colour of what is being done,
// downloading or unpacking, and how far it is.
function StatusBar({ profile, queue, downloads }: { profile: Profile | null; queue: QueueStatus | null; downloads: Download[] }) {
  const mods = profile?.mods?.filter((m) => !m.separator) ?? [];
  const on = mods.filter((m) => m.enabled).length;

  const running = downloads.find((d) => d.status === "downloading");
  const waiting = downloads.filter((d) => d.status === "queued").length;
  const archives = queue ? queue.total - queue.finished : 0;
  // A download under way first, then the archive being unpacked; a queue
  // waiting on an installer's window counts how many archives are through.
  let stage: "download" | "unpack" | null = null;
  let share = 0;
  if (running) {
    stage = running.stage === "unpack" ? "unpack" : "download";
    share = running.progress || 0;
  } else if (queue) {
    stage = "unpack";
    share = queue.unpack?.total ? queue.unpack.done / queue.unpack.total : queue.total ? queue.finished / queue.total : 0;
  }
  const count = (running ? 1 : 0) + waiting + archives;
  const percent = Math.round(share * 100);

  return (
    <footer className="relative flex h-7 shrink-0 items-center gap-4 border-t border-zinc-800 bg-zinc-900 px-3 text-xs text-zinc-500">
      {profile && (
        <span>
          {mods.length} {mods.length === 1 ? "mod" : "mods"}, {on} on
        </span>
      )}
      {stage && (
        <div
          className="absolute left-1/2 flex -translate-x-1/2 items-center gap-2"
          title={`${count} in the queue · ${stage === "download" ? "downloading" : "unpacking"} ${percent}%`}
        >
          <span className="w-6 text-right font-medium text-zinc-300 tabular-nums">{count}</span>
          <span className="h-1.5 w-64 overflow-hidden rounded-full bg-zinc-800">
            <span className={`block h-full transition-[width] ${stageColors[stage]}`} style={{ width: `${percent}%` }} />
          </span>
          <span className="w-9 font-medium text-zinc-300 tabular-nums">{percent}%</span>
        </div>
      )}
    </footer>
  );
}

function DownloadIcon() {
  return (
    <svg viewBox="0 0 24 24" className="h-5 w-5 shrink-0" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M12 3v12" />
      <path d="m7 10 5 5 5-5" />
      <path d="M5 21h14" />
    </svg>
  );
}
