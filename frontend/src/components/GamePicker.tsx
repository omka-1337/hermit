import { useCallback, useEffect, useState } from "react";
import { errorMessage, Game, GameCandidate, Library } from "../api";
import AddGameForm from "./AddGameForm";
import GameIcon from "./GameIcon";
import { BackendBadge, Button, EngineBadge, ErrorText, RuntimeBadge } from "./ui";

type Props = {
  onAdded: (game: Game) => void;
  // Rendered at the bottom right, e.g. Skip / Continue / Close.
  actions?: React.ReactNode;
};

// GamePicker lists Unity games found in Steam libraries, with a manual fallback.
export default function GamePicker({ onAdded, actions }: Props) {
  const [games, setGames] = useState<GameCandidate[] | null>(null);
  const [manual, setManual] = useState(false);
  const [adding, setAdding] = useState<string | null>(null);
  const [error, setError] = useState("");

  const discover = useCallback(async () => {
    setError("");
    try {
      setGames((await Library.DiscoverGames()) ?? []);
    } catch (err) {
      setGames([]);
      setError(errorMessage(err));
    }
  }, []);

  useEffect(() => {
    discover();
  }, [discover]);

  const add = async (c: GameCandidate) => {
    setAdding(c.path);
    setError("");
    try {
      const game = await Library.AddGame(c.name, c.path);
      setGames((list) => list?.map((g) => (g.path === c.path ? { ...g, alreadyAdded: true } : g)) ?? null);
      onAdded(game);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setAdding(null);
    }
  };

  if (manual) {
    return (
      <AddGameForm
        onAdded={(g) => {
          setManual(false);
          discover();
          onAdded(g);
        }}
        extraActions={
          <Button type="button" variant="ghost" onClick={() => setManual(false)}>
            Back
          </Button>
        }
      />
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <span className="text-sm text-zinc-400">Unity games found in Steam</span>
        <Button variant="ghost" onClick={() => discover()} disabled={games === null}>
          Refresh
        </Button>
      </div>

      <div className="max-h-80 overflow-y-auto rounded-lg border border-zinc-800">
        {games === null ? (
          <p className="px-4 py-6 text-center text-sm text-zinc-500">Scanning Steam libraries…</p>
        ) : games.length === 0 ? (
          <p className="px-4 py-6 text-center text-sm text-zinc-500">No compatible games found.</p>
        ) : (
          <ul className="divide-y divide-zinc-800">
            {games.map((c, i) => (
              <li key={c.path} className="flex items-center gap-3 px-4 py-2.5">
                <GameIcon name={c.name} steamAppId={c.steamAppId} size={36} />
                <div className="flex min-w-0 flex-1 flex-col gap-1">
                  <span className="truncate text-sm font-medium">{c.name}</span>
                  <div className="flex items-center gap-1.5">
                    <EngineBadge engine={c.detection.engine} version={c.detection.engineVersion} />
                    <RuntimeBadge runtime={c.detection.runtime} />
                    <BackendBadge backend={c.detection.backend} />
                    <span className="min-w-0 truncate text-xs text-zinc-500" title={c.path}>
                      {c.path}
                    </span>
                  </div>
                </div>
                {c.alreadyAdded ? (
                  <span className="w-16 text-center text-xs text-zinc-500">Added</span>
                ) : (
                  <Button
                    variant="primary"
                    className="w-16"
                    data-focus-first={i === 0 || undefined}
                    disabled={adding !== null}
                    onClick={() => add(c)}
                  >
                    Add
                  </Button>
                )}
              </li>
            ))}
          </ul>
        )}
      </div>

      <ErrorText>{error}</ErrorText>

      <div className="flex items-center justify-between gap-2">
        <Button variant="ghost" onClick={() => setManual(true)}>
          Add folder manually…
        </Button>
        <div className="flex gap-2">{actions}</div>
      </div>
    </div>
  );
}
