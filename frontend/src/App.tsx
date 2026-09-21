import { useEffect, useState } from "react";
import { errorMessage, Game, InfoService, Library, Settings, SettingsStore } from "./api";
import ConfirmHost from "./confirm";
import SteamSetupWindow from "./screens/SteamSetupWindow";
import { UIModeProvider } from "./uimode";
import WindowFrame from "./components/WindowFrame";
import Main from "./screens/Main";
import Setup from "./screens/Setup";

type State =
  | { status: "loading" }
  | { status: "error"; message: string }
  | { status: "ready"; setup: boolean; settings: Settings; steamDeck: boolean };

// The first-run window is a window of its own, opened by the Go side at
// /#steam-setup; it shares this bundle but none of the manager's chrome.
const isSteamSetupWindow = window.location.hash === "#steam-setup";

function App() {
  if (isSteamSetupWindow) {
    return <SteamSetupWindow />;
  }

  const [state, setState] = useState<State>({ status: "loading" });
  const [games, setGames] = useState<Game[]>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);

  // reloadGames refreshes the list; selectId overrides the selection (null clears it).
  const reloadGames = async (selectId?: string | null) => {
    const list = (await Library.ListGames()) ?? [];
    setGames(list);
    setSelectedId((current) => {
      const want = selectId === undefined ? current : selectId;
      return list.some((g) => g.id === want) ? want : (list[0]?.id ?? null);
    });
  };

  useEffect(() => {
    Promise.all([SettingsStore.Get(), InfoService.GetInfo(), reloadGames()])
      .then(([settings, info]) =>
        setState({ status: "ready", setup: !settings.setupCompleted, settings, steamDeck: info.steamDeck }),
      )
      .catch((err) => setState({ status: "error", message: errorMessage(err) }));
  }, []);

  const finishSetup = async (game?: Game) => {
    try {
      const settings = await SettingsStore.CompleteSetup();
      await reloadGames(game?.id);
      setState((prev) => ({
        status: "ready",
        setup: false,
        settings,
        steamDeck: prev.status === "ready" ? prev.steamDeck : false,
      }));
    } catch (err) {
      setState({ status: "error", message: errorMessage(err) });
    }
  };

  let screen: React.ReactNode = null;
  switch (state.status) {
    case "error":
      screen = <div className="p-8 text-red-400">Failed to load: {state.message}</div>;
      break;
    case "ready":
      screen = state.setup ? (
        <Setup onFinish={finishSetup} />
      ) : (
        <Main games={games} selectedId={selectedId} onSelect={setSelectedId} onGamesChanged={reloadGames} />
      );
  }
  if (state.status !== "ready") {
    return <WindowFrame>{screen}</WindowFrame>;
  }
  return (
    <UIModeProvider settings={state.settings} steamDeck={state.steamDeck}>
      <WindowFrame>{screen}</WindowFrame>
      <ConfirmHost />
    </UIModeProvider>
  );
}

export default App;
