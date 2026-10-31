import { useCallback, useEffect, useState } from "react";
import { Clipboard } from "@wailsio/runtime";
import { confirm, errorMessage, Game, IconService, LaunchInfo, LaunchService } from "../api";
import { Button, ErrorText } from "./ui";

// Both the running game and the Steam launch options change behind our back:
// the game is started and closed outside the manager, and Steam writes its
// config with a delay. Polling keeps the page honest without a refresh.
const launchPollMs = 3000;

export function useLaunchInfo(gameId: string) {
  const [info, setInfo] = useState<LaunchInfo | null>(null);
  const reload = useCallback(() => {
    LaunchService.GetLaunchInfo(gameId).then(setInfo).catch(console.error);
  }, [gameId]);

  useEffect(() => {
    reload();
    const timer = setInterval(() => {
      // Nothing to show while the window is hidden.
      if (!document.hidden) reload();
    }, launchPollMs);
    const onVisible = () => {
      if (!document.hidden) reload();
    };
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      clearInterval(timer);
      document.removeEventListener("visibilitychange", onVisible);
    };
  }, [reload]);

  return { info, reload };
}

type PlayProps = {
  game: Game;
  profileId: string;
  label?: string;
  // onPlayed runs after the game was started; the active profile may have changed.
  onPlayed?: () => void;
};

// usePlay starts a game with a profile: through Steam, which runs it through
// the wrapper set in its launch options, or, for a game Steam does not
// start, through PortProton when it is there.
export function usePlay(game: Game, profileId: string, onPlayed?: () => void) {
  const { info, reload } = useLaunchInfo(game.id);
  const [error, setError] = useState("");
  const [starting, setStarting] = useState(false);
  const viaPortProton = !!info?.external && !!info.portProton;

  const play = async () => {
    setError("");
    try {
      if (!viaPortProton) {
        const fresh = await LaunchService.GetLaunchInfo(game.id);
        if (!fresh.configured) {
          const ok = await confirm(
            "Launch options not found",
            "Mods load only when Steam runs the game through the mod manager. Set the launch options shown on the game page.\n\n" +
              "Steam may not have saved them to disk yet. Launch anyway?",
            "Launch",
          );
          if (!ok) return;
        }
      }
      setStarting(true);
      if (viaPortProton) await LaunchService.PlayViaPortProton(game.id, profileId);
      else await LaunchService.Play(game.id, profileId);
      onPlayed?.();
      // Steam takes a few seconds to start the game, PortProton a while to
      // bring Wine up; "Running" shows once the session is there.
      setTimeout(() => {
        setStarting(false);
        reload();
      }, 8000);
    } catch (err) {
      setStarting(false);
      setError(errorMessage(err));
    }
  };

  return { info, play, starting, error, viaPortProton };
}

export function PlayButton({ game, profileId, label = "Play", onPlayed }: PlayProps) {
  const { info, play, starting, error, viaPortProton } = usePlay(game, profileId, onPlayed);

  if (info?.external) {
    if (viaPortProton) {
      return <PortProtonButton running={info.running} starting={starting} error={error} onPlay={play} />;
    }
    return info.running ? (
      <span className="rounded-md bg-zinc-800 px-3 py-1.5 text-sm font-medium text-zinc-300">Running</span>
    ) : null;
  }

  const disabled = !info?.supported || info.running || starting;
  return (
    <div className="flex flex-col items-end gap-1">
      <Button variant="primary" disabled={disabled} onClick={play} title={info?.supported ? "" : info?.reason}>
        {info?.running ? "Running" : starting ? "Starting…" : label}
      </Button>
      <ErrorText>{error}</ErrorText>
    </div>
  );
}

// LaunchSetup is the Launch section of a game's page: how to get the game to
// load its profile. Once Steam has the launch options there is nothing left
// to do there, and the section is not shown.
export function LaunchSetup({ game }: { game: Game }) {
  const { info, reload } = useLaunchInfo(game.id);
  if (!info || (info.supported && !info.external && info.configured)) return null;
  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-sm font-semibold tracking-wide text-zinc-400 uppercase">Launch</h2>
      {!info.supported ? (
        <p className="text-sm text-zinc-400">{info.reason}</p>
      ) : info.external ? (
        <ExternalLaunchSetup info={info} />
      ) : (
        <SteamLaunchSetup info={info} reload={reload} />
      )}
    </section>
  );
}

function SteamLaunchSetup({ info, reload }: { info: LaunchInfo; reload: () => void }) {
  const [copied, setCopied] = useState(false);

  const copy = async () => {
    await Clipboard.SetText(info.launchOptions);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="flex flex-col gap-3 rounded-lg border border-zinc-800 p-4">
      <div className="flex items-center justify-between gap-3">
        <span className="text-sm font-medium">Steam launch options</span>
        <span className="text-xs text-amber-400">Not detected</span>
      </div>
      <p className="text-xs text-zinc-400">
        In Steam open the game&apos;s Properties → General → Launch Options and paste this. The active profile is then
        loaded whenever the game starts, including from Steam directly. The game folder is left untouched: Proton
        games get links to the profile only while running, native Linux games load BepInEx straight from the
        profile.
      </p>
      <div className="flex gap-2">
        <code className="flex-1 truncate rounded-md bg-zinc-900 px-3 py-1.5 font-mono text-xs text-zinc-300 select-text">
          {info.launchOptions}
        </code>
        <Button onClick={copy}>{copied ? "Copied" : "Copy"}</Button>
        <Button variant="ghost" onClick={reload}>
          Check again
        </Button>
      </div>
      <p className="text-xs text-zinc-500">Steam saves launch options with a delay, sometimes only when it exits.</p>
    </div>
  );
}

// ExternalLaunchSetup is for games that Steam does not start. Hermit cannot
// start them itself or look into their launcher's settings, so it only hands
// out the wrapper to paste there.
function ExternalLaunchSetup({ info }: { info: LaunchInfo }) {
  return (
    <div className="flex flex-col gap-3 rounded-lg border border-zinc-800 p-4">
      <span className="text-sm font-medium">Launch options</span>
      <p className="text-xs text-zinc-400">
        This game is not started by Steam, so start it from the launcher you use for it and let that launcher run it
        through Hermit. The active profile is then linked into the game while it runs, the same as for Steam games.
      </p>

      <CopyRow
        label="Lutris, Heroic and other launchers — as a command prefix or wrapper"
        value={info.launchPrefix}
      />
      <p className="-mt-1 text-xs text-zinc-500">
        Lutris: Configure → System options → Command prefix. Heroic: game settings → Advanced → Wrapper command.
      </p>

      <CopyRow label="Steam, if you added the game as a non-Steam game — its Launch Options" value={info.launchOptions} />
    </div>
  );
}

function CopyRow({ label, value }: { label: string; value: string }) {
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    await Clipboard.SetText(value);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };
  return (
    <div className="flex flex-col gap-1.5">
      <span className="text-xs text-zinc-400">{label}</span>
      <div className="flex gap-2">
        {/* Wrapped, not truncated: the end of the line names the game. */}
        <code className="flex-1 rounded-md bg-zinc-900 px-3 py-1.5 font-mono text-xs break-all text-zinc-300 select-text">
          {value}
        </code>
        <Button disabled={!value} onClick={copy}>
          {copied ? "Copied" : "Copy"}
        </Button>
      </div>
    </div>
  );
}

// PortProton's icon is read once; every game page shows the same one.
let portProtonIcon: Promise<string> | null = null;

export function usePortProtonIcon(): string {
  const [icon, setIcon] = useState("");
  useEffect(() => {
    portProtonIcon ??= IconService.GetPortProtonIcon().catch(() => "");
    let active = true;
    portProtonIcon.then((src) => active && setIcon(src));
    return () => {
      active = false;
    };
  }, []);
  return icon;
}

type PortProtonProps = {
  running: boolean;
  starting: boolean;
  error: string;
  onPlay: () => void;
};

// PortProtonButton starts a game outside Steam through PortProton, which has
// nowhere to paste launch options, so Hermit starts it itself. Launch settings
// sit behind the gear next to it, out of the way until they are needed.
function PortProtonButton({ running, starting, error, onPlay }: PortProtonProps) {
  const icon = usePortProtonIcon();
  return (
    <div className="flex flex-col items-end gap-1">
      <Button variant="primary" disabled={running || starting} onClick={onPlay} className="flex items-center gap-2">
        {running ? "Running" : starting ? "Starting…" : "Play via PortProton"}
        {icon && <img src={icon} alt="" className="h-5 w-5 shrink-0" />}
      </Button>
      <ErrorText>{error}</ErrorText>
    </div>
  );
}
