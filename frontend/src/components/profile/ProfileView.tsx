import { useEffect, useState } from "react";
import { errorMessage, FolderService, Game, InstallService, Profile } from "../../api";
import { packageLabel } from "../../format";
import { PlayButton } from "../launch";
import { ExportModal } from "../share";
import { Button, ErrorText, useModalOpen } from "../ui";
import { useGamepad } from "../../gamepad";
import { usePageHints } from "../../screens/shell";
import { useLayout } from "../../uimode";
import BrowseTab from "./BrowseTab";
import ConfigTab from "./ConfigTab";
import InstalledTab from "./InstalledTab";
import LoaderNotice from "./LoaderNotice";
import { ProfileProvider } from "./ProfileContext";

type Tab = "installed" | "browse" | "config";

type Props = {
  game: Game;
  profileId: string;
  onBack: () => void;
  onGameChanged: () => void;
  onOpenProfile: (profileId: string) => void;
};

export default function ProfileView({ game, profileId, onBack, onGameChanged, onOpenProfile }: Props) {
  const deck = useLayout() === "deck";
  const modalOpen = useModalOpen();
  const [profile, setProfile] = useState<Profile | null>(null);
  const [tab, setTab] = useState<Tab>("installed");
  const [error, setError] = useState("");
  const [exporting, setExporting] = useState(false);

  useEffect(() => {
    InstallService.OpenProfile(game.id, profileId)
      .then(setProfile)
      .catch((err) => setError(errorMessage(err)));
  }, [game.id, profileId]);

  const modCount = profile?.mods?.length ?? 0;
  const tabs: { id: Tab; label: string }[] = [
    { id: "installed", label: `Installed${modCount ? ` (${modCount})` : ""}` },
    { id: "browse", label: "Browse" },
    { id: "config", label: "Config" },
  ];

  const step = (by: number) =>
    setTab((current) => {
      const at = tabs.findIndex((t) => t.id === current);
      return tabs[(at + by + tabs.length) % tabs.length].id;
    });

  useGamepad({ onTabPrev: () => step(-1), onTabNext: () => step(1) }, deck && !modalOpen);
  usePageHints(deck ? ["LT / RT — tabs"] : []);

  return (
    <div className="flex h-full flex-col">
      <header className="flex items-center gap-3 border-b border-zinc-800 px-6 pt-4">
        <Button variant="ghost" onClick={onBack} aria-label="Back to game">
          ←
        </Button>
        <div className="flex min-w-0 flex-col pb-3">
          <span className="text-xs text-zinc-500">{game.name}</span>
          <span className="truncate text-lg font-semibold">{profile?.name ?? "…"}</span>
          {profile?.modpack && (
            <span className="truncate text-xs text-indigo-400">Modpack {packageLabel(profile.modpack)}</span>
          )}
        </div>
        <nav className="ml-6 flex gap-1 self-end" title={deck ? "LT / RT switch tabs" : undefined}>
          {tabs.map((t) => (
            <button
              key={t.id}
              onClick={() => setTab(t.id)}
              className={`border-b-2 px-3 pb-3 text-sm ${
                tab === t.id
                  ? "border-indigo-500 text-zinc-100"
                  : "border-transparent text-zinc-400 hover:text-zinc-200"
              }`}
            >
              {t.label}
            </button>
          ))}
        </nav>
        <div className="ml-auto flex items-center gap-3 self-center pb-3">
          {game.activeProfile === profileId && (
            <span className="text-xs font-medium text-indigo-400">Active profile</span>
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
          {profile && (
            <Button variant="ghost" onClick={() => setExporting(true)}>
              Export
            </Button>
          )}
          <PlayButton game={game} profileId={profileId} onPlayed={onGameChanged} />
        </div>
      </header>

      {exporting && profile && <ExportModal game={game} profile={profile} onClose={() => setExporting(false)} />}

      <div className="flex min-h-0 flex-1 flex-col">
        {error && (
          <div className="p-6">
            <ErrorText>{error}</ErrorText>
          </div>
        )}
        {profile && (
          <ProfileProvider game={game} profile={profile} onProfileChange={setProfile} onOpenProfile={onOpenProfile}>
            {tab !== "config" && <LoaderNotice />}
            <div className="min-h-0 flex-1">
              {tab === "installed" && <InstalledTab onBrowse={() => setTab("browse")} />}
              {tab === "config" && <ConfigTab />}
              {/* Browse stays mounted so search and scroll survive tab switches. */}
              <div className={tab === "browse" ? "h-full" : "hidden"}>
                <BrowseTab />
              </div>
            </div>
          </ProfileProvider>
        )}
      </div>
    </div>
  );
}
