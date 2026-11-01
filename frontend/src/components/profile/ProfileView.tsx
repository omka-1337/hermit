import { ReactNode, useEffect, useState } from "react";
import { Events } from "@wailsio/runtime";
import { deployed, Engine, errorMessage, FolderService, Game, InstallService, ModListService, Profile } from "../../api";
import { requestRefresh, useRefresh } from "../../refresh";
import { packageLabel } from "../../format";
import { PlayButton } from "../launch";
import PlayBar from "../PlayBar";
import { ExportModal } from "../share";
import { Button, ErrorText, useModalOpen } from "../ui";
import { useGamepad } from "../../gamepad";
import { usePageHints } from "../../screens/shell";
import { useLayout } from "../../uimode";
import DataTab from "./DataTab";
import NexusBrowseTab from "./NexusBrowseTab";
import ModListTab from "./ModListTab";
import PluginsPanel from "./PluginsPanel";
import BrowseTab from "./BrowseTab";
import ConfigTab from "./ConfigTab";
import InstalledTab from "./InstalledTab";
import LoaderNotice from "./LoaderNotice";
import LogsTab from "./LogsTab";
import ToolsTab from "./ToolsTab";
import { ProfileProvider } from "./ProfileContext";
import { ResizeHandle, usePanelWidth } from "../../panelWidth";
import { GearIcon } from "../../screens/shell";
import { installFromFileEvent } from "../TitleMenus";
import ProfileOptionsModal from "./ProfileOptionsModal";

export type Tab = "installed" | "browse" | "config" | "data" | "tools" | "logs";

// profileTabs are the pages of a profile. A game whose mods are deployed has
// neither Thunderstore nor BepInEx: its profile is the list of its mods, and
// in the desktop layout the game folder as its mods will make it, the tools
// its mods bring and, for a Creation Engine game, the logs its script
// extender's plugins wrote. Its load order of
// plugins is in a panel beside the list of mods.
export function profileTabs(game: Game, profile: Profile | null, desktop = false): { id: Tab; label: string }[] {
  // Separators head groups of mods and are not mods themselves. The desktop
  // layout counts mods in its status bar instead.
  const modCount = desktop ? 0 : (profile?.mods?.filter((m) => !m.separator).length ?? 0);
  return deployed(game.engine)
    ? [
        { id: "installed", label: `Mods${modCount ? ` (${modCount})` : ""}` },
        ...(desktop
          ? [
              { id: "browse" as const, label: "Browse" },
              { id: "data" as const, label: game.engine === Engine.EngineCreation ? "Data" : "Game folder" },
              { id: "tools" as const, label: "Tools" },
            ]
          : []),
        ...(desktop && game.engine === Engine.EngineCreation ? [{ id: "logs" as const, label: "Logs" }] : []),
      ]
    : [
        { id: "installed", label: `Installed${modCount ? ` (${modCount})` : ""}` },
        { id: "browse", label: "Browse" },
        { id: "config", label: "Config" },
      ];
}

type Props = {
  game: Game;
  profileId: string;
  onBack: () => void;
  onGameChanged: () => void;
  onOpenProfile: (profileId: string) => void;
  // tab, when given, is picked in the desktop sidebar, which also holds the
  // profile's name and what is done to the profile as a whole; the page then
  // keeps only the tab's content and Play.
  tab?: Tab;
  onProfileChange?: (profile: Profile) => void;
  // addRequest opens the dialog for adding a mod from a file each time it
  // goes up, for File → Install from file.
  addRequest?: number;
};

export default function ProfileView({
  game,
  profileId,
  onBack,
  onGameChanged,
  onOpenProfile,
  tab: chosenTab,
  onProfileChange,
  addRequest = 0,
}: Props) {
  const deck = useLayout() === "deck";
  const modalOpen = useModalOpen();
  const [profile, setProfileState] = useState<Profile | null>(null);
  const [ownTab, setTab] = useState<Tab>("installed");
  const embedded = chosenTab !== undefined;
  const tab = chosenTab ?? ownTab;
  const setProfile = (p: Profile) => {
    setProfileState(p);
    onProfileChange?.(p);
  };
  const [error, setError] = useState("");
  const [exporting, setExporting] = useState(false);
  const [optionsOpen, setOptionsOpen] = useState(false);

  // A refresh reads the profile again; the list of a game whose mods are
  // deployed also takes in mod folders put in by hand and lets go of those
  // removed.
  useRefresh(() => {
    const read =
      deployed(game.engine)
        ? ModListService.Rescan(game.id, profileId)
        : InstallService.OpenProfile(game.id, profileId);
    read.then(setProfile).catch((err) => setError(errorMessage(err)));
  });

  // A mod installed from a FOMOD installer's window, or anywhere else.
  useEffect(
    () =>
      Events.On("modlist:installed", (ev) => {
        if (ev.data?.gameId === game.id && ev.data?.profileId === profileId) {
          InstallService.OpenProfile(game.id, profileId).then(setProfile).catch(console.error);
        }
      }),
    [game.id, profileId],
  );

  useEffect(() => {
    InstallService.OpenProfile(game.id, profileId)
      .then(setProfile)
      .catch((err) => setError(errorMessage(err)));
  }, [game.id, profileId]);

  const modList = deployed(game.engine);
  // A Creation Engine game's list of mods has the profile's load order of
  // plugins beside it in the desktop layout, as wide as it was dragged to:
  // 22rem at first.
  const panel = embedded && game.engine === Engine.EngineCreation;
  const plugins = usePanelWidth("plugins", 352, 260, 720);
  const tabs = profileTabs(game, profile, embedded);

  const step = (by: number) =>
    setTab((current) => {
      const at = tabs.findIndex((t) => t.id === current);
      return tabs[(at + by + tabs.length) % tabs.length].id;
    });

  useGamepad({ onTabPrev: () => step(-1), onTabNext: () => step(1) }, deck && !modalOpen);
  usePageHints(deck ? ["LT / RT — tabs"] : []);

  return (
    <div className="flex h-full flex-col">
      {embedded ? (
        <header className="flex min-h-16 items-center gap-3 border-b border-zinc-800 px-6 py-3">
          {embedded && modList && tab === "installed" ? (
            // The list of mods and the plugins head themselves; what is done to
            // the profile as a whole takes the page's title.
            <div className="flex items-center gap-2">
              <IconButton
                accent
                title="Install mods from archives…"
                onClick={() => window.dispatchEvent(new CustomEvent(installFromFileEvent))}
              >
                <InstallIcon />
              </IconButton>
              <IconButton title="Refresh: read the mods and plugins again, as F5 does" onClick={requestRefresh}>
                <RefreshIcon />
              </IconButton>
              {game.engine === Engine.EngineCreation && (
                <IconButton title="Profile settings: INI files and saves of the profile's own" onClick={() => setOptionsOpen(true)}>
                  <GearIcon />
                </IconButton>
              )}
            </div>
          ) : (
            <span className="text-lg font-semibold">{tabs.find((t) => t.id === tab)?.label}</span>
          )}
          <div className="ml-auto">
            <PlayBar game={game} profileId={profileId} onGameChanged={onGameChanged} />
          </div>
        </header>
      ) : (
        <header className="flex items-center gap-3 border-b border-zinc-800 px-6 pt-4">
          <Button variant="ghost" onClick={onBack} aria-label="Back to game">
            ←
          </Button>
          <div className="flex min-w-0 flex-col pb-3">
            <span className="text-xs text-zinc-500">{game.name}</span>
            <span className="truncate text-lg font-semibold">{profile?.name ?? "…"}</span>
            {profile?.modpack && (
              <span className="truncate text-xs text-accent-400">Modpack {packageLabel(profile.modpack)}</span>
            )}
          </div>
          <nav className="ml-6 flex gap-1 self-end" title={deck ? "LT / RT switch tabs" : undefined}>
            {tabs.map((t) => (
              <button
                key={t.id}
                onClick={() => setTab(t.id)}
                className={`border-b-2 px-3 pb-3 text-sm ${
                  tab === t.id
                    ? "border-accent-500 text-zinc-100"
                    : "border-transparent text-zinc-400 hover:text-zinc-200"
                }`}
              >
                {t.label}
              </button>
            ))}
          </nav>
          <div className="ml-auto flex items-center gap-3 self-center pb-3">
            {game.activeProfile === profileId && (
              <span className="text-xs font-medium text-accent-400">Active profile</span>
            )}
            {profile && (
              <Button
                variant="ghost"
                title="Open the profile's folder in the file manager"
                onClick={() => FolderService.OpenProfile(game.id, profileId).catch((err) => setError(errorMessage(err)))}
              >
                Open folder
              </Button>
            )}
            {profile && !modList && (
              <Button variant="ghost" onClick={() => setExporting(true)}>
                Export
              </Button>
            )}
            <PlayButton game={game} profileId={profileId} onPlayed={onGameChanged} />
          </div>
        </header>
      )}

      {exporting && profile && <ExportModal game={game} profile={profile} onClose={() => setExporting(false)} />}
      {optionsOpen && profile && (
        <ProfileOptionsModal game={game} profileId={profileId} profileName={profile.name} onClose={() => setOptionsOpen(false)} />
      )}

      <div className="flex min-h-0 flex-1 flex-col">
        {error && (
          <div className="p-6">
            <ErrorText>{error}</ErrorText>
          </div>
        )}
        {profile && (
          <ProfileProvider game={game} profile={profile} onProfileChange={setProfile} onOpenProfile={onOpenProfile}>
            {modList ? (
              <div className="min-h-0 flex-1">
                {tab === "logs" ? (
                  <LogsTab />
                ) : tab === "tools" ? (
                  <ToolsTab />
                ) : tab === "data" ? (
                  <DataTab />
                ) : tab === "browse" ? (
                  <NexusBrowseTab />
                ) : panel ? (
                  <div className="flex h-full">
                    <div className="min-w-0 flex-1">
                      <ModListTab />
                    </div>
                    <aside style={{ width: plugins.width }} className="relative shrink-0 border-l border-zinc-800">
                      <ResizeHandle edge="left" panel={plugins} label="Plugins panel width" />
                      <PluginsPanel />
                    </aside>
                  </div>
                ) : (
                  <ModListTab />
                )}
              </div>
            ) : (
              <>
                {tab !== "config" && <LoaderNotice />}
                <div className="min-h-0 flex-1">
                  {tab === "installed" && <InstalledTab onBrowse={() => setTab("browse")} addRequest={addRequest} />}
                  {tab === "config" && <ConfigTab />}
                  {/* Browse stays mounted so search and scroll survive tab switches. */}
                  <div className={tab === "browse" ? "h-full" : "hidden"}>
                    <BrowseTab />
                  </div>
                </div>
              </>
            )}
          </ProfileProvider>
        )}
      </div>
    </div>
  );
}

function InstallIcon() {
  return (
    <svg viewBox="0 0 24 24" className="h-5 w-5 shrink-0" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M12 3v12" />
      <path d="m7 10 5 5 5-5" />
      <path d="M5 21h14" />
    </svg>
  );
}

function RefreshIcon() {
  return (
    <svg viewBox="0 0 24 24" className="h-5 w-5 shrink-0" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M20 11a8 8 0 1 0-2.3 5.7" />
      <path d="M20 4v7h-7" />
    </svg>
  );
}

// IconButton is a tool of the page, framed so it stands out as MO2's
// toolbar does; accent marks the one most used.
function IconButton({
  title,
  onClick,
  accent = false,
  children,
}: {
  title: string;
  onClick: () => void;
  accent?: boolean;
  children: ReactNode;
}) {
  return (
    <button
      title={title}
      aria-label={title}
      onClick={onClick}
      className={`flex h-10 w-10 items-center justify-center rounded-lg border transition-colors ${
        accent
          ? "border-accent-500/50 bg-accent-500/15 text-accent-300 hover:border-accent-400 hover:bg-accent-500/25"
          : "border-zinc-700 bg-zinc-800/80 text-zinc-100 hover:border-zinc-500 hover:bg-zinc-700"
      }`}
    >
      {children}
    </button>
  );
}
