import { Download, DownloadService, errorMessage } from "../api";
import { useDownloads, useNexusSetup } from "../downloads";
import { formatBytes } from "../format";
import { Button } from "../components/ui";

const statusText: Record<string, string> = {
  queued: "Waiting",
  downloading: "Downloading",
  done: "Downloaded",
  failed: "Failed",
  cancelled: "Cancelled",
};

// DownloadsView lists the downloads from Nexus Mods: the one under way, the
// ones waiting for it, and those that are over.
export default function DownloadsView({ onSetUp }: { onSetUp: () => void }) {
  const downloads = useDownloads();
  const { ready } = useNexusSetup();
  const over = downloads.some((d) => d.status !== "queued" && d.status !== "downloading");

  return (
    <div className="mx-auto flex max-w-3xl flex-col gap-4 p-8">
      <div className="flex items-center gap-3">
        <h1 className="flex-1 text-2xl font-semibold">Downloads</h1>
        {over && (
          <Button variant="ghost" onClick={() => DownloadService.ClearFinished().catch(console.error)}>
            Clear finished
          </Button>
        )}
      </div>
      {!ready && (
        <div className="flex items-center gap-3 rounded-lg border border-amber-500/30 bg-amber-500/5 px-4 py-3 text-sm text-amber-300">
          <span className="flex-1">Downloads from Nexus Mods are not set up yet.</span>
          <Button onClick={onSetUp}>Set up</Button>
        </div>
      )}
      {downloads.length === 0 ? (
        <p className="text-sm text-zinc-500">
          Nothing downloaded yet. Mod Manager Download on Nexus Mods downloads a mod here and installs it into the
          active profile of its game.
        </p>
      ) : (
        <ul className="divide-y divide-zinc-800">
          {[...downloads].reverse().map((d) => (
            <DownloadRow key={d.id} d={d} />
          ))}
        </ul>
      )}
    </div>
  );
}

function DownloadRow({ d }: { d: Download }) {
  const active = d.status === "queued" || d.status === "downloading";
  const share = d.size ? d.done / d.size : 0;
  return (
    <li className="flex items-center gap-4 py-3">
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <span className="truncate text-sm font-medium">
          {d.name}
          {d.version && <span className="ml-2 font-normal text-zinc-500">{d.version}</span>}
        </span>
        <span className="truncate text-xs text-zinc-500">
          {d.gameName}
          {d.fileName && ` · ${d.fileName}`}
        </span>
        {d.status === "downloading" && (
          <span className="mt-1 h-1.5 overflow-hidden rounded-full bg-zinc-800">
            <span className="block h-full bg-indigo-500 transition-[width]" style={{ width: `${Math.round(share * 100)}%` }} />
          </span>
        )}
        {d.error && <span className="text-xs text-red-400">{d.error}</span>}
      </div>
      <span className="w-40 shrink-0 text-right text-xs text-zinc-400 tabular-nums">
        {d.status === "downloading"
          ? `${formatBytes(d.done)} of ${formatBytes(d.size)}${d.speed ? ` · ${formatBytes(d.speed)}/s` : ""}`
          : d.size
            ? formatBytes(d.size)
            : ""}
      </span>
      <span
        className={`w-24 shrink-0 text-xs ${
          d.status === "failed" ? "text-red-400" : d.status === "done" ? "text-emerald-400" : "text-zinc-400"
        }`}
      >
        {statusText[d.status] ?? d.status}
      </span>
      {active ? (
        <Button variant="ghost" onClick={() => DownloadService.Cancel(d.id).catch((err) => console.error(errorMessage(err)))}>
          Cancel
        </Button>
      ) : (
        <span className="w-[70px]" />
      )}
    </li>
  );
}
