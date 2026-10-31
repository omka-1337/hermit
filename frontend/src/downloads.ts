import { useCallback, useEffect, useState } from "react";
import { Events } from "@wailsio/runtime";
import { Download, DownloadService, NexusAccount, NXMHandler } from "./api";

// useDownloads follows the downloads from Nexus Mods, as they change.
export function useDownloads(): Download[] {
  const [list, setList] = useState<Download[]>([]);
  useEffect(() => {
    DownloadService.List()
      .then((l) => setList(l ?? []))
      .catch(console.error);
    return Events.On("downloads:changed", (ev) => setList(ev.data ?? []));
  }, []);
  return list;
}

// useNexusSetup says whether downloads from Nexus Mods work: an API key is
// there, and "Mod Manager Download" links open in Hermit.
export function useNexusSetup() {
  const [account, setAccount] = useState<NexusAccount | null>(null);
  const [handler, setHandler] = useState<NXMHandler | null>(null);
  const reload = useCallback(() => {
    DownloadService.Account().then(setAccount).catch(console.error);
    DownloadService.NXMHandler().then(setHandler).catch(console.error);
  }, []);
  useEffect(reload, [reload]);
  const ready = !!account?.hasKey && !!handler?.hermit;
  return { account, handler, ready, reload, setAccount, setHandler };
}
