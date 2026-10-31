import { useState } from "react";
import { Issue, IssueKind, Report } from "../../api";
import { formatAgo } from "../../format";

export function issueText(issue: Issue): string {
  switch (issue.kind) {
    case IssueKind.IssueIncompatible:
      return `Not loaded: incompatible with ${issue.detail}`;
    case IssueKind.IssueMissingDependencies:
      return `Not loaded: missing ${issue.detail}`;
    case IssueKind.IssueDependencyNotLoaded:
      return "Not loaded: a dependency failed to load";
    case IssueKind.IssueNewerVersionExists:
      return `Skipped: a newer copy is loaded (${issue.detail})`;
    case IssueKind.IssueProcessFilter:
      return "Skipped: not meant for this game process";
    default:
      return `Failed to load: ${issue.detail}`;
  }
}

// LaunchReport summarises the last game session of the profile.
export default function LaunchReport({ report }: { report: Report }) {
  const [expanded, setExpanded] = useState(false);
  const issues = report.issues ?? [];
  const when = formatAgo(report.finishedAt);

  if (!report.bepinexStarted) {
    return (
      <div className="mb-3 rounded-lg border border-amber-500/30 bg-amber-500/5 px-4 py-3 text-sm">
        <p className="text-amber-400">BepInEx did not start during the last launch ({when}).</p>
        <p className="mt-1 text-xs text-zinc-400">
          Check that the Steam launch options are set and that the game was started after they were saved.
        </p>
      </div>
    );
  }

  const loaded = report.loaded ?? [];
  const notStarted = report.notStarted ?? [];
  // Reports saved by older builds lack these fields: unknown is not "stuck".
  const known = typeof report.errors === "number";
  const stuck = known && report.complete === false;
  // A few errors are normal for a modded game; hundreds are not.
  const manyErrors = known && report.errors >= 100;
  const trouble = issues.length > 0 || notStarted.length > 0 || stuck || manyErrors;
  const tone = trouble ? "border-amber-500/30 bg-amber-500/5" : "border-zinc-800";

  return (
    <div className={`mb-3 rounded-lg border px-4 py-3 text-sm ${tone}`}>
      <div className="flex items-center justify-between gap-3">
        <span>
          Last launch {when}
          <span className="text-zinc-500"> · BepInEx {report.bepinexVersion}</span>
        </span>
        {(issues.length > 0 || notStarted.length > 0) && (
          <button className="text-xs text-zinc-400 hover:text-zinc-200" onClick={() => setExpanded(!expanded)}>
            {expanded ? "Hide" : "Details"}
          </button>
        )}
      </div>

      {/* The numbers to compare when the game only shows a black screen. */}
      <div className="mt-1 flex flex-wrap gap-x-4 gap-y-1 text-xs">
        {(report.pluginMods ?? 0) > 0 && <span className="text-zinc-400">Mods: {report.pluginMods}</span>}
        <span className="text-zinc-400">Loaded: {loaded.length}</span>
        {notStarted.length > 0 && <span className="text-amber-400">Not loaded: {notStarted.length}</span>}
        {issues.length > 0 && <span className="text-amber-400">Failed: {issues.length}</span>}
        {known && (
          <span className={manyErrors ? "text-amber-400" : "text-zinc-400"}>Errors in the log: {report.errors}</span>
        )}
      </div>

      {stuck && (
        <p className="mt-2 text-xs text-amber-300">
          BepInEx never finished loading mods
          {loaded.length > 0 && (
            <>
              : the last one it started was <span className="text-zinc-100">{loaded[loaded.length - 1]}</span>
            </>
          )}
          . The game most likely hung or crashed while that mod or the next one was starting.
        </p>
      )}
      {!stuck && manyErrors && (
        <p className="mt-2 text-xs text-amber-300">
          The mods loaded, but the log is full of errors. That is usually mods made for another version of the game —
          try Update all.
        </p>
      )}

      {expanded && (
        <ul className="mt-2 flex flex-col gap-1 text-xs select-text">
          {issues.map((issue, i) => (
            <li key={`issue-${i}`}>
              <span className="text-zinc-200">{issue.plugin}</span>{" "}
              <span className="text-zinc-400">{issueText(issue)}</span>
            </li>
          ))}
          {notStarted.map((id) => (
            <li key={`missing-${id}`}>
              <span className="text-zinc-200">{id}</span>{" "}
              <span className="text-zinc-400">Not loaded: BepInEx never got to it</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
