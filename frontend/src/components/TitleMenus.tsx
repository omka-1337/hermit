import { CSSProperties, useState } from "react";
import { Browser, Dialogs } from "@wailsio/runtime";
import { confirm, errorMessage, InfoService } from "../api";
import ContextMenu, { anchorBelow, MenuAnchor, MenuItem } from "./ContextMenu";
import hermitIcon from "../assets/hermit.png";

const noDrag = { "--wails-draggable": "no-drag" } as CSSProperties;

// installFromFileEvent asks the open page to install a mod from a file; the
// page knows the game and profile it goes into.
export const installFromFileEvent = "hermit:install-from-file";

// TitleMenus are the menus at the left of the title bar: Hermit itself, and
// File.
export default function TitleMenus() {
  const [menu, setMenu] = useState<{ which: "hermit" | "file"; at: MenuAnchor } | null>(null);

  const open = (which: "hermit" | "file") => (e: React.MouseEvent<HTMLElement>) =>
    setMenu(menu?.which === which ? null : { which, at: anchorBelow(e.currentTarget) });

  const pick = (action: () => void) => () => {
    setMenu(null);
    action();
  };

  const openRepository = async () => {
    const info = await InfoService.GetInfo();
    if (info.repository) Browser.OpenURL(info.repository);
  };

  const checkForUpdates = async () => {
    try {
      const update = await InfoService.CheckUpdate();
      if (!update.available) {
        await Dialogs.Info({ Title: "Hermit is up to date", Message: `${update.current} is the latest version.` });
        return;
      }
      const ok = await confirm(
        `Hermit ${update.latest} is out`,
        `You have ${update.current}. Hermit does not update itself: open the release page to download the new version?`,
        "Open the release page",
      );
      if (ok && update.url) Browser.OpenURL(update.url);
    } catch (err) {
      await Dialogs.Error({ Title: "Could not check for updates", Message: errorMessage(err) });
    }
  };

  const button = (which: "hermit" | "file") =>
    `flex h-full items-center px-3 text-xs transition-colors ${
      menu?.which === which ? "bg-zinc-800 text-zinc-100" : "text-zinc-400 hover:bg-zinc-800 hover:text-zinc-100"
    }`;

  return (
    <div style={noDrag} className="flex h-full" onDoubleClick={(e) => e.stopPropagation()}>
      <button className={`${button("hermit")} gap-1.5 font-semibold`} onClick={open("hermit")}>
        <img src={hermitIcon} alt="" className="h-5 w-5" />
        Hermit
      </button>
      <button className={button("file")} onClick={open("file")}>
        File
      </button>
      {menu?.which === "hermit" && (
        <ContextMenu at={menu.at} onClose={() => setMenu(null)}>
          <MenuItem onClick={pick(openRepository)}>Hermit on GitHub</MenuItem>
          <MenuItem onClick={pick(checkForUpdates)}>Check for updates…</MenuItem>
        </ContextMenu>
      )}
      {menu?.which === "file" && (
        <ContextMenu at={menu.at} onClose={() => setMenu(null)}>
          <MenuItem onClick={pick(() => window.dispatchEvent(new CustomEvent(installFromFileEvent)))}>
            Install from file…
          </MenuItem>
        </ContextMenu>
      )}
    </div>
  );
}
