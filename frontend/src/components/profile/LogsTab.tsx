import { useCallback, useEffect, useState } from "react";
import { CreationService, errorMessage, LogFile } from "../../api";
import { formatAgo, formatBytes } from "../../format";
import { Button, ErrorText } from "../ui";
import { useRefresh } from "../../refresh";
import { useProfile } from "./ProfileContext";

// Lines that read as trouble stand out from the rest of a log.
const errorLine = /\b(error|errors|fail|failed|failure|exception|fatal|critical|crash)\b/i;
const warningLine = /\b(warn|warning|warnings)\b/i;

// LogsTab shows the logs the script extender and its plugins write, in
// Documents\My Games\<game>\SKSE of the game's Proton prefix: the list on the
// left, newest first, and the one picked on the right.
export default function LogsTab() {
  const { game } = useProfile();
  const [logs, setLogs] = useState<LogFile[] | null>(null);
  const [chosen, setChosen] = useState<string | null>(null);
  const [text, setText] = useState("");
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    setError("");
    try {
      const list = (await CreationService.Logs(game.id)) ?? [];
      setLogs(list);
      setChosen((c) => (c && list.some((l) => l.name === c) ? c : (list[0]?.name ?? null)));
    } catch (err) {
      setLogs([]);
      setError(errorMessage(err));
    }
  }, [game.id]);

  const read = useCallback(async () => {
    if (!chosen) return setText("");
    try {
      setText(await CreationService.ReadLog(game.id, chosen));
    } catch (err) {
      setText("");
      setError(errorMessage(err));
    }
  }, [game.id, chosen]);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    read();
  }, [read]);

  const refresh = async () => {
    await load();
    await read();
  };
  useRefresh(refresh);

  return (
    <div className="flex h-full">
      <ul className="w-64 shrink-0 overflow-y-auto border-r border-zinc-800 py-1">
        {logs?.map((l) => (
          <li key={l.name}>
            <button
              onClick={() => setChosen(l.name)}
              className={`flex w-full flex-col px-4 py-2 text-left ${
                l.name === chosen ? "bg-zinc-800 text-zinc-100" : "text-zinc-300 hover:bg-zinc-800/60"
              }`}
            >
              <span className="truncate text-sm font-medium">{l.name}</span>
              <span className="text-xs text-zinc-500">
                {formatAgo(l.modified)} · {formatBytes(l.size)}
              </span>
            </button>
          </li>
        ))}
        {logs?.length === 0 && !error && (
          <li className="px-4 py-3 text-sm text-zinc-500">No logs yet. They appear once the game has run with SKSE.</li>
        )}
      </ul>
      <div className="flex min-w-0 flex-1 flex-col">
        <div className="flex items-center gap-2 border-b border-zinc-800 px-4 py-2">
          <span className="min-w-0 flex-1 truncate text-sm font-medium">{chosen ?? ""}</span>
          <Button variant="ghost" onClick={refresh}>
            Refresh
          </Button>
          <Button variant="ghost" onClick={() => CreationService.OpenLogs(game.id).catch((err) => setError(errorMessage(err)))}>
            Open folder
          </Button>
        </div>
        {error && (
          <div className="px-4 pt-3">
            <ErrorText>{error}</ErrorText>
          </div>
        )}
        <pre className="min-h-0 flex-1 overflow-auto px-4 py-3 font-mono text-xs leading-relaxed text-zinc-300 select-text">
          {text.split("\n").map((line, i) => (
            <div key={i} className={errorLine.test(line) ? "text-red-400" : warningLine.test(line) ? "text-amber-400" : undefined}>
              {line || " "}
            </div>
          ))}
        </pre>
      </div>
    </div>
  );
}
