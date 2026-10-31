import { useCallback, useEffect, useState } from "react";
import { errorMessage, SteamService, SteamSetup } from "../api";
import { Button, ErrorText, Spinner } from "./ui";

type Props = {
  // onDone is called once the shortcut exists, or the user moves on without it.
  onDone?: () => void;
  // skipLabel turns the second button into a way out of a first-run step.
  skipLabel?: string;
};

// AddToSteam offers to add Hermit itself to Steam as a non-Steam game. On a
// Steam Deck that is the only way to start it from Game Mode.
export default function AddToSteam({ onDone, skipLabel }: Props) {
  const [setup, setSetup] = useState<SteamSetup | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [justAdded, setJustAdded] = useState(false);

  const load = useCallback(() => {
    SteamService.GetSetup()
      .then(setSetup)
      .catch((err) => setError(errorMessage(err)));
  }, []);
  useEffect(load, [load]);

  const add = async () => {
    setError("");
    setBusy(true);
    try {
      const result = await SteamService.AddToSteam();
      setSetup(result);
      setJustAdded(result.added);
    } catch (err) {
      setError(errorMessage(err));
      load();
    } finally {
      setBusy(false);
    }
  };

  if (!setup) {
    return (
      <p className="flex items-center gap-2 text-sm text-zinc-500">
        <Spinner /> Looking for Steam…
      </p>
    );
  }

  return (
    <div className="flex flex-col gap-3">
      {setup.added ? (
        <p className="text-sm text-emerald-400">
          ✓ Hermit is in your Steam library{justAdded && ". Start it from there, or from Game Mode on a Steam Deck"}.
        </p>
      ) : (
        <>
          <p className="text-sm text-zinc-400">
            Steam can start Hermit like a game. On a Steam Deck that is the only way to open it from Game Mode; on a
            desktop it simply puts it in your library.
          </p>
          {setup.supported ? (
            <p className={`text-sm ${setup.running ? "text-amber-400" : "text-zinc-400"}`}>
              {setup.running
                ? "Steam is running and keeps its shortcuts in memory, so it will be closed and started again. Do this while no game is running."
                : "Steam is not running, so the shortcut is written straight away."}
            </p>
          ) : (
            <p className="text-sm text-zinc-400">{setup.reason}</p>
          )}
        </>
      )}

      <p className="truncate text-xs text-zinc-500" title={setup.exe}>
        {setup.exe}
      </p>
      <ErrorText>{error}</ErrorText>

      <div className="flex gap-2">
        {!setup.added && (
          <Button variant="primary" disabled={!setup.supported || busy} onClick={add}>
            {busy ? (setup.running ? "Restarting Steam…" : "Adding…") : "Add to Steam"}
          </Button>
        )}
        {(skipLabel || setup.added) && (
          <Button variant={setup.added ? "primary" : "ghost"} disabled={busy} onClick={() => onDone?.()}>
            {setup.added ? "Continue" : skipLabel}
          </Button>
        )}
      </div>
    </div>
  );
}
