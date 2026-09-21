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

export function PlayButton({ game, profileId, label = "Play", onPlayed }: PlayProps) {
  const { info, reload } = useLaunchInfo(game.id);
  const [error, setError] = useState("");
  const [starting, setStarting] = useState(false);

  const play = async () => {
    setError("");
    try {
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
      setStarting(true);
      await LaunchService.Play(game.id, profileId);
      onPlayed?.();
      // Steam takes a few seconds to start the game.
      setTimeout(() => {
        setStarting(false);
        reload();
      }, 8000);
    } catch (err) {
      setStarting(false);
      setError(errorMessage(err));
    }
  };

  if (info?.external) {
    if (info.portProton) {
      return <PortProtonButton game={game} profileId={profileId} running={info.running} onPlayed={onPlayed} />;
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

export function LaunchSetup({ game }: { game: Game }) {
  const { info, reload } = useLaunchInfo(game.id);
  const [copied, setCopied] = useState(false);

  if (info && !info.supported) {
    return <p className="text-sm text-zinc-400">{info.reason}</p>;
  }

  // While the info is being read the card is drawn as it will look, so the
  // page keeps its shape instead of collapsing on every game switch.
  const shown: LaunchInfo = info ?? {
    supported: true,
    configured: false,
    running: false,
    runningProfile: "",
    reason: "",
    launchOptions: "",
    external: false,
    launchPrefix: "",
    portProton: false,
  };

  if (shown.external) {
    return <ExternalLaunchSetup info={shown} />;
  }

  const copy = async () => {
    await Clipboard.SetText(shown.launchOptions);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className={`flex flex-col gap-3 rounded-lg border border-zinc-800 p-4 ${info ? "" : "opacity-60"}`}>
      <div className="flex items-center justify-between gap-3">
        <span className="text-sm font-medium">Steam launch options</span>
        {!info ? (
          <span className="text-xs text-zinc-500">Checking…</span>
        ) : shown.configured ? (
          <span className="text-xs text-emerald-400">✓ Set in Steam</span>
        ) : (
          <span className="text-xs text-amber-400">Not detected</span>
        )}
      </div>
      <p className="text-xs text-zinc-400">
        In Steam open the game&apos;s Properties → General → Launch Options and paste this. The active profile is then
        loaded whenever the game starts, including from Steam directly. The game folder is left untouched: Proton
        games get links to the profile only while running, native Linux games load BepInEx straight from the
        profile.
      </p>
      <div className="flex gap-2">
        <code className="flex-1 truncate rounded-md bg-zinc-900 px-3 py-1.5 font-mono text-xs text-zinc-300 select-text">
          {shown.launchOptions}
        </code>
        <Button disabled={!info} onClick={copy}>
          {copied ? "Copied" : "Copy"}
        </Button>
        {!shown.configured && (
          <Button variant="ghost" disabled={!info} onClick={reload}>
            Check again
          </Button>
        )}
      </div>
      {!shown.configured && (
        <p className="text-xs text-zinc-500">Steam saves launch options with a delay, sometimes only when it exits.</p>
      )}
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

function usePortProtonIcon(): string {
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
  game: Game;
  profileId: string;
  running: boolean;
  onPlayed?: () => void;
};

// PortProtonButton starts a game outside Steam through PortProton, which has
// nowhere to paste launch options, so Hermit starts it itself.
function PortProtonButton({ game, profileId, running, onPlayed }: PortProtonProps) {
  const icon = usePortProtonIcon();
  const [error, setError] = useState("");
  const [starting, setStarting] = useState(false);

  const play = async () => {
    setError("");
    setStarting(true);
    try {
      await LaunchService.PlayViaPortProton(game.id, profileId);
      onPlayed?.();
      // PortProton takes a while to bring Wine up; "Running" shows once the
      // session is there.
      setTimeout(() => setStarting(false), 8000);
    } catch (err) {
      setStarting(false);
      setError(errorMessage(err));
    }
  };

  return (
    <div className="flex flex-col items-end gap-1">
      <Button variant="primary" disabled={running || starting} onClick={play} className="flex items-center gap-2">
        {running ? "Running" : starting ? "Starting…" : "Play via PortProton"}
        {icon && <img src={icon} alt="" className="h-5 w-5 shrink-0" />}
      </Button>
      <ErrorText>{error}</ErrorText>
    </div>
  );
}
