import { useEffect, useState } from "react";
import { BepInExBuild, errorMessage, InstallService, LoaderStatus } from "../../api";
import { formatBytes, progressText } from "../../format";
import { Button, ErrorText } from "../ui";
import { useProfile } from "./ProfileContext";

// Without a loader Hermit links nothing into the game and it starts vanilla,
// and nothing else in the profile says so. Games on Thunderstore get BepInEx
// as a dependency of their first mod; the rest — mods from Nexus Mods, a file
// on disk, a game with no Thunderstore community at all — need it installed
// from BepInEx's own releases.
export default function LoaderNotice() {
  const { game, profile, progress, applyProfile } = useProfile();
  const [loader, setLoader] = useState<LoaderStatus | null>(null);
  const [build, setBuild] = useState<BepInExBuild | null>(null);
  const [installing, setInstalling] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    setLoader(null);
    InstallService.LoaderStatus(game.id, profile.id).then(setLoader).catch(console.error);
  }, [game.id, profile.id, profile.mods]);

  // Which build the game needs is a GitHub lookup, so it waits until it is
  // clear one is missing.
  useEffect(() => {
    if (loader?.installed !== false) return;
    InstallService.BepInExBuild(game.id)
      .then(setBuild)
      .catch((err) => setError(errorMessage(err)));
  }, [loader?.installed, game.id]);

  const install = async () => {
    setError("");
    setInstalling(true);
    try {
      applyProfile(await InstallService.InstallBepInEx(game.id, profile.id));
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setInstalling(false);
    }
  };

  if (!loader) return null;

  if (loader.installed) {
    if (loader.enabled) return null;
    return (
      <Notice>
        <span>
          <strong>{loader.name}</strong> is disabled, so the game starts without mods.
        </span>
      </Notice>
    );
  }

  return (
    <Notice>
      <div className="flex min-w-0 flex-col gap-1">
        <span>BepInEx is not installed in this profile, so mods will not load.</span>
        <span className="text-xs text-amber-200/70">
          {installing && progress
            ? progressText(progress)
            : build
              ? `${build.asset} — BepInEx ${build.version}${build.prerelease ? " (pre-release)" : ""}, ${formatBytes(build.size)} from GitHub`
              : "Looking up the build this game needs…"}
        </span>
        <ErrorText>{error}</ErrorText>
      </div>
      <Button variant="primary" className="ml-auto shrink-0" disabled={!build || installing} onClick={install}>
        {installing ? "Installing…" : "Install BepInEx"}
      </Button>
    </Notice>
  );
}

function Notice({ children }: { children: React.ReactNode }) {
  return (
    <div className="mx-6 mt-3 flex items-center gap-3 rounded-md border border-amber-500/40 bg-amber-500/10 px-4 py-3 text-sm text-amber-100">
      {children}
    </div>
  );
}
