import { useEffect } from "react";
import { Game, PackageDetail } from "../../api";
import { formatAgo, progressText } from "../../format";
import { cancelModpackInstall, onModpackSettled, startModpackInstall, useModpackInstall } from "../../modpackInstalls";
import { Button, ErrorText } from "../ui";

type Props = {
  game: Game;
  pkg: PackageDetail;
  onInstalled: (profileId: string) => void;
};

// A modpack pins the exact mod versions it was put together with, so one that
// has not been updated for a while installs mods from back then. Games like
// these update often, and old mod versions are the usual reason a modpack no
// longer starts; past this age the button says so before installing.
const staleAfterDays = 180;

function modpackAge(created: string): number {
  const time = new Date(created).getTime();
  return Number.isNaN(time) ? 0 : (Date.now() - time) / 86_400_000;
}

// ModpackInstallButton creates a new profile from a modpack. The install runs
// on when this page is left; coming back shows it instead of starting another.
export default function ModpackInstallButton({ game, pkg, onInstalled }: Props) {
  const id = `${pkg.namespace}-${pkg.name}`;
  const install = useModpackInstall(game.id, id);
  const running = install?.status === "running";

  // Open the new profile when it is done — as long as this page is still up.
  useEffect(
    () =>
      onModpackSettled((settled) => {
        if (settled.profileId && settled.gameId === game.id && settled.packageId === id) onInstalled(settled.profileId);
      }),
    [game.id, id, onInstalled],
  );

  const start = () =>
    startModpackInstall(game.id, pkg.namespace, pkg.name, pkg.latest_version_number, pkg.name.replace(/_/g, " "));

  const stale = modpackAge(pkg.version_created) > staleAfterDays;

  return (
    <div className="flex flex-col gap-2">
      {stale && !running && (
        <div className="rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-300">
          This modpack was last updated {formatAgo(pkg.version_created)}. It installs its mods in the versions from
          back then, and those may no longer work with the current game — if it does not start, try Update all in the
          new profile, which moves every mod to its latest version.
        </div>
      )}
      <div className="flex gap-2">
        <Button variant="primary" disabled={running} onClick={start}>
          {running ? "Creating profile…" : "Create profile from modpack"}
        </Button>
        {running && (
          <Button variant="ghost" onClick={() => cancelModpackInstall(game.id, id)}>
            Cancel
          </Button>
        )}
      </div>
      <p className="text-xs text-zinc-500">
        {pkg.dependencies?.length ?? 0} mods in their exact versions, with the modpack&apos;s configs.
      </p>
      {running && install.progress && <p className="text-xs text-zinc-400">{progressText(install.progress)}</p>}
      {running && !install.progress && <p className="text-xs text-zinc-400">Starting…</p>}
      {install?.status === "failed" && <ErrorText>{install.error}</ErrorText>}
    </div>
  );
}
