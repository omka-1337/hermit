import { useState } from "react";
import { Game } from "../api";
import GamePicker from "../components/GamePicker";
import GameView from "../components/GameView";
import ProfileView from "../components/profile/ProfileView";
import SettingsView from "./SettingsView";
import ConsoleShell from "./ConsoleShell";
import DesktopWorkspace from "./DesktopWorkspace";
import { Page } from "./shell";
import { useLayout } from "../uimode";

type Props = {
  games: Game[];
  selectedId: string | null;
  onSelect: (id: string | null) => void;
  onGamesChanged: (selectId?: string | null) => Promise<void>;
};

// Main picks the layout. The desktop one, DesktopWorkspace, is laid out on its
// own; for the deck Main owns the navigation state and builds the page
// content, with the chrome around it from ConsoleShell.
export default function Main({ games, selectedId, onSelect, onGamesChanged }: Props) {
  const deck = useLayout() === "deck";
  const [page, setPage] = useState<Page>("game");
  const [openProfile, setOpenProfile] = useState<{ gameId: string; profileId: string } | null>(null);
  const selected = games.find((g) => g.id === selectedId) ?? null;

  const openGame = (id: string) => {
    setOpenProfile(null);
    setPage("game");
    onSelect(id);
  };

  // A game added from either picker becomes the selected one.
  const addedGame = async (id: string) => {
    openGame(id);
    await onGamesChanged(id);
  };

  if (!deck) {
    return <DesktopWorkspace games={games} selectedId={selectedId} onSelect={onSelect} onGamesChanged={onGamesChanged} />;
  }

  const shellProps = {
    games,
    selectedId,
    page,
    canGoBack: openProfile !== null || (page !== "game" && selected !== null),
    onOpenGame: openGame,
    onAdd: () => {
      setOpenProfile(null);
      setPage("add");
    },
    onSettings: () => {
      setOpenProfile(null);
      setPage("settings");
    },
    onBack: () => {
      if (openProfile) setOpenProfile(null);
      else if (page !== "game" && selected) openGame(selected.id);
    },
  };

  // The console layout adds games on a page of its own.
  const addGamePage = (
    <div className="mx-auto flex max-w-3xl flex-col gap-4 p-6">
      <h2 className="text-xl font-semibold">Add game</h2>
      <GamePicker onAdded={(g) => addedGame(g.id)} />
    </div>
  );

  const content =
    page === "settings" ? (
      <SettingsView />
    ) : page === "add" || !selected ? (
      // With no games at all the bar highlights Add game, so show its page.
      addGamePage
    ) : openProfile?.gameId === selected.id ? (
      <ProfileView
        key={openProfile.profileId}
        game={selected}
        profileId={openProfile.profileId}
        onBack={() => setOpenProfile(null)}
        onGameChanged={() => onGamesChanged(selected.id)}
        onOpenProfile={(profileId) => setOpenProfile({ gameId: selected.id, profileId })}
      />
    ) : (
      <GameView
        key={selected.id}
        game={selected}
        onChanged={(g) => onGamesChanged(g.id)}
        onRemoved={() => onGamesChanged(null)}
        onOpenProfile={(profileId) => setOpenProfile({ gameId: selected.id, profileId })}
      />
    );

  return <ConsoleShell {...shellProps}>{content}</ConsoleShell>;
}
