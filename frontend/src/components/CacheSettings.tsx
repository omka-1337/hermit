import { useCallback, useEffect, useState } from "react";
import { CacheService, confirm, errorMessage } from "../api";
import { formatBytes } from "../format";
import { useAnyModpackInstalling } from "../modpackInstalls";
import { Button, ErrorText } from "./ui";

// CacheSettings shows how much the download cache takes and empties it. The
// cache holds every mod archive ever downloaded, so it grows to gigabytes.
export default function CacheSettings() {
  const [size, setSize] = useState<number | null>(null);
  const [busy, setBusy] = useState(false);
  const [freed, setFreed] = useState<number | null>(null);
  const [error, setError] = useState("");
  const installing = useAnyModpackInstalling();

  const measure = useCallback(() => {
    CacheService.Size()
      .then(setSize)
      .catch((err) => setError(errorMessage(err)));
  }, []);
  useEffect(measure, [measure]);

  const clear = async () => {
    setError("");
    const ok = await confirm(
      "Clear cache",
      "Delete the downloaded mod archives and Thunderstore data? Installed mods stay in their profiles; anything needed later is downloaded again.",
      "Clear",
    );
    if (!ok) return;
    setBusy(true);
    try {
      setFreed(await CacheService.Clear());
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
      measure();
    }
  };

  return (
    <div className="flex flex-col gap-2 rounded-lg border border-zinc-800 px-4 py-3">
      <div className="flex items-center justify-between gap-3">
        <span className="flex flex-col gap-0.5">
          <span className="text-sm font-medium">Download cache</span>
          <span className="text-xs text-zinc-500">
            Mod archives kept after installing them, so reinstalling or making another profile does not download
            them again.
          </span>
        </span>
        <span className="shrink-0 text-sm text-zinc-300">{size === null ? "…" : formatBytes(size)}</span>
      </div>
      <div className="flex items-center gap-3">
        <Button disabled={busy || installing || !size} onClick={clear}>
          {busy ? "Clearing…" : "Clear cache"}
        </Button>
        {installing && <span className="text-xs text-zinc-500">Available once the modpack install finishes.</span>}
        {freed !== null && !busy && !installing && (
          <span className="text-xs text-emerald-400">✓ Freed {formatBytes(freed)}</span>
        )}
      </div>
      <ErrorText>{error}</ErrorText>
    </div>
  );
}
