import { useEffect, useRef, useState } from "react";
import {
  BrowseView,
  DownloadService,
  errorMessage,
  Game,
  isCancelled,
  ModFile,
  ModPage,
  ModSummary,
  NexusService,
  SettingsStore,
} from "../../api";
import { useNexusSetup } from "../../downloads";
import { openInBrowser } from "../../screens/BrowserPage";
import { formatAgo, formatBytes, formatCount } from "../../format";
import { useUIMode } from "../../uimode";
import BBCode from "../BBCode";
import { Button, ErrorText, inputClass, NsfwBadge } from "../ui";
import { CardsIcon, ListIcon, ViewButton } from "./PackageBrowser";

const sorts: { value: string; label: string }[] = [
  { value: "endorsements", label: "Most endorsed" },
  { value: "downloads", label: "Most downloaded" },
  { value: "updatedAt", label: "Last updated" },
  { value: "createdAt", label: "Newest" },
  { value: "relevance", label: "Best match" },
];

const pageSize = 20;

type Props = {
  game: Game;
  // installed are the ids on Nexus Mods of the mods the profile has.
  installed: Set<number>;
};

// NexusBrowser searches and pages through the mods Nexus Mods has for a
// game, as PackageBrowser does through Thunderstore's. Files are downloaded
// into the active profile: straight away for a Premium account, through
// the "Mod Manager Download" button of the mod's page otherwise, as Nexus
// Mods gives its files to no one else without its website.
export default function NexusBrowser({ game, installed }: Props) {
  const { layout, cards, setBrowseView } = useUIMode();
  const [query, setQuery] = useState("");
  const [debouncedQuery, setDebouncedQuery] = useState("");
  const [sort, setSort] = useState("endorsements");
  const [adult, setAdult] = useState(false);

  const [mods, setMods] = useState<ModSummary[]>([]);
  const [page, setPage] = useState(0);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [selected, setSelected] = useState<number | null>(null);

  const listRef = useRef<HTMLDivElement>(null);
  const sentinelRef = useRef<HTMLDivElement>(null);

  // Mods with adult content show when Settings allow NSFW packages.
  useEffect(() => {
    SettingsStore.Get()
      .then((s) => setAdult(s.allowNsfw))
      .catch(() => {});
  }, []);

  useEffect(() => {
    const t = setTimeout(() => setDebouncedQuery(query.trim()), 400);
    return () => clearTimeout(t);
  }, [query]);

  // Any filter change restarts the listing from the first page.
  useEffect(() => {
    setPage(0);
    listRef.current?.scrollTo({ top: 0 });
  }, [debouncedQuery, sort, adult]);

  useEffect(() => {
    setLoading(true);
    setError("");
    const call = NexusService.Search(game.id, {
      game: "",
      query: debouncedQuery,
      sort,
      offset: page * pageSize,
      count: pageSize,
      adult,
    });
    call
      .then((r) => {
        const items = r.mods ?? [];
        setMods((prev) => (page === 0 ? items : [...prev, ...items]));
        setTotal(r.total);
        setLoading(false);
      })
      .catch((err) => {
        if (isCancelled(err)) return;
        setError(errorMessage(err));
        setLoading(false);
      });
    return () => {
      call.cancel();
    };
  }, [game.id, debouncedQuery, sort, adult, page]);

  const hasMore = mods.length < total;
  // Load the next page when the bottom of the list scrolls into view.
  useEffect(() => {
    const el = sentinelRef.current;
    if (!el || !hasMore || loading) return;
    const observer = new IntersectionObserver((entries) => entries[0].isIntersecting && setPage((p) => p + 1), {
      root: listRef.current,
      rootMargin: "400px",
    });
    observer.observe(el);
    return () => observer.disconnect();
  }, [hasMore, loading, mods]);

  return (
    <div className="relative flex h-full min-h-0">
      <div className="flex min-w-0 flex-1 flex-col">
        <div className="flex items-center gap-2 border-b border-zinc-800 px-6 py-3">
          <input
            className={inputClass}
            placeholder={`Search ${game.name} mods on Nexus Mods`}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
          <select className={`${inputClass} w-auto`} value={sort} onChange={(e) => setSort(e.target.value)}>
            {sorts.map((s) => (
              <option key={s.value} value={s.value}>
                {s.label}
              </option>
            ))}
          </select>
          <div className="flex shrink-0 gap-1">
            <ViewButton label="Cards" active={cards} onClick={() => setBrowseView(BrowseView.BrowseViewCards)} icon={<CardsIcon />} />
            <ViewButton label="List" active={!cards} onClick={() => setBrowseView(BrowseView.BrowseViewList)} icon={<ListIcon />} />
          </div>
        </div>

        <div ref={listRef} className="flex-1 overflow-y-auto">
          <div className="px-6 py-2 text-xs text-zinc-500">{formatCount(total)} mods</div>
          {cards ? (
            <ul className="grid grid-cols-[repeat(auto-fill,minmax(13rem,1fr))] gap-3 px-4 pb-3">
              {mods.map((m, i) => (
                <li key={m.modId}>
                  <button
                    data-focus-first={i === 0 || undefined}
                    onClick={() => setSelected(m.modId)}
                    className={`flex h-full w-full flex-col overflow-hidden rounded-lg border text-left transition ${
                      selected === m.modId ? "border-accent-500 bg-zinc-800" : "border-zinc-800 bg-zinc-900/60 hover:border-zinc-700"
                    }`}
                  >
                    <div className="aspect-video w-full bg-zinc-800">
                      {m.thumbnailUrl && <img src={m.thumbnailUrl} alt="" loading="lazy" className="h-full w-full object-cover" />}
                    </div>
                    <div className="flex min-w-0 flex-1 flex-col gap-0.5 p-2.5">
                      <span className="truncate text-sm font-medium">{m.name}</span>
                      <span className="truncate text-xs text-zinc-500">by {m.author}</span>
                      <p className="mt-1 line-clamp-2 text-xs text-zinc-400">{m.summary}</p>
                      <div className="mt-1 flex flex-wrap items-center gap-1.5">
                        {m.adultContent && <NsfwBadge />}
                        {installed.has(m.modId) && <span className="text-xs text-accent-400">Installed</span>}
                      </div>
                      <div className="mt-auto flex gap-3 pt-2 text-xs text-zinc-500">
                        <span>↓ {formatCount(m.downloads)}</span>
                        <span>♥ {formatCount(m.endorsements)}</span>
                      </div>
                    </div>
                  </button>
                </li>
              ))}
            </ul>
          ) : (
            <ul className="flex flex-col px-3 pb-3">
              {mods.map((m, i) => (
                <li key={m.modId}>
                  <button
                    data-focus-first={i === 0 || undefined}
                    onClick={() => setSelected(m.modId)}
                    className={`flex w-full gap-3 rounded-md px-3 py-2.5 text-left ${
                      selected === m.modId ? "bg-zinc-800" : "hover:bg-zinc-800/50"
                    }`}
                  >
                    <Thumbnail url={m.thumbnailUrl} />
                    <div className="flex min-w-0 flex-1 flex-col">
                      <div className="flex items-baseline gap-2">
                        <span className="truncate text-sm font-medium">{m.name}</span>
                        <span className="truncate text-xs text-zinc-500">by {m.author}</span>
                        {m.adultContent && <NsfwBadge />}
                        {installed.has(m.modId) && <span className="ml-auto shrink-0 text-xs text-accent-400">Installed</span>}
                      </div>
                      <p className="line-clamp-2 text-xs text-zinc-400">{m.summary}</p>
                      <div className="mt-1 flex gap-3 text-xs text-zinc-500">
                        <span>↓ {formatCount(m.downloads)}</span>
                        <span>♥ {formatCount(m.endorsements)}</span>
                        <span>{formatAgo(m.updatedAt)}</span>
                      </div>
                    </div>
                  </button>
                </li>
              ))}
            </ul>
          )}
          <div ref={sentinelRef} />
          <div className="px-6 pb-6">
            <ErrorText>{error}</ErrorText>
            {loading && <p className="text-center text-sm text-zinc-500">Loading…</p>}
            {!loading && !error && mods.length === 0 && <p className="text-center text-sm text-zinc-500">No mods found.</p>}
          </div>
        </div>
      </div>

      {selected !== null && (
        <aside
          className={
            layout === "deck"
              ? "absolute inset-0 z-10 border-l border-zinc-800 bg-zinc-900"
              : "w-[480px] shrink-0 border-l border-zinc-800 bg-zinc-900"
          }
        >
          <NexusDetails key={selected} game={game} modId={selected} onClose={() => setSelected(null)} />
        </aside>
      )}
    </div>
  );
}

// Thumbnail is a mod's picture in a row of the list, wider than tall as
// Nexus Mods' pictures are.
function Thumbnail({ url }: { url: string }) {
  return (
    <div className="h-12 w-20 shrink-0 overflow-hidden rounded-md bg-zinc-800">
      {url && <img src={url} alt="" loading="lazy" className="h-full w-full object-cover" />}
    </div>
  );
}

const categoryLabels: Record<string, string> = {
  MAIN: "Main files",
  UPDATE: "Updates",
  OPTIONAL: "Optional files",
  MISCELLANEOUS: "Miscellaneous files",
  OLD_VERSION: "Old versions",
};

function NexusDetails({ game, modId, onClose }: { game: Game; modId: number; onClose: () => void }) {
  const nexus = useNexusSetup();
  const [page, setPage] = useState<ModPage | null>(null);
  const [error, setError] = useState("");
  const [queued, setQueued] = useState<Set<number>>(new Set());

  useEffect(() => {
    const call = NexusService.Page(game.id, modId);
    call.then(setPage).catch((err) => !isCancelled(err) && setError(errorMessage(err)));
    return () => {
      call.cancel();
    };
  }, [game.id, modId]);

  // A Premium account downloads straight away; any other through the button
  // of the file on Nexus Mods, opened in Hermit's browser, which hands the
  // file to Hermit and takes the user back here.
  const download = async (f: ModFile) => {
    setError("");
    try {
      if (nexus.account?.premium) {
        await DownloadService.DownloadFile(game.id, modId, f.fileId);
        setQueued((s) => new Set(s).add(f.fileId));
      } else {
        openInBrowser(await NexusService.FilesURL(game.id, modId, f.fileId));
      }
    } catch (err) {
      setError(errorMessage(err));
    }
  };

  const groups = Object.keys(categoryLabels)
    .map((c) => ({ category: c, files: (page?.files ?? []).filter((f) => f.category === c) }))
    .filter((g) => g.files.length > 0);

  return (
    <div className="flex h-full flex-col">
      <div className="flex justify-end px-4 pt-3">
        <Button variant="ghost" onClick={onClose} aria-label="Close details">
          ✕
        </Button>
      </div>
      <div className="flex-1 overflow-y-auto px-6 pb-6">
        <ErrorText>{error}</ErrorText>
        {!page && !error && <p className="text-sm text-zinc-500">Loading…</p>}
        {page && (
          <div className="flex flex-col gap-5">
            {page.pictureUrl && <img src={page.pictureUrl} alt="" className="w-full rounded-lg bg-zinc-800" />}
            <div className="flex flex-col gap-1">
              <div className="flex items-center gap-2">
                <h2 className="text-xl font-semibold">{page.name}</h2>
                {page.adultContent && <NsfwBadge />}
              </div>
              <span className="text-sm text-zinc-400">by {page.author}</span>
              <span className="text-xs text-zinc-500">
                v{page.version} · updated {formatAgo(page.updatedAt)}
                {page.category && ` · ${page.category}`}
              </span>
            </div>
            <p className="text-sm text-zinc-300">{page.summary}</p>

            {!nexus.account?.premium && (
              <p className="rounded-md border border-zinc-800 bg-zinc-900 px-3 py-2 text-xs text-zinc-400">
                Download opens the file's page of Nexus Mods here: its{" "}
                <span className="text-zinc-200">Mod Manager Download</span> button hands it to Hermit, which brings you back
                and installs it into the active profile. The first time, log in to Nexus Mods on that page.
                {!nexus.account?.hasKey && " Hermit needs your Nexus Mods API key in Settings as well."}
              </p>
            )}

            <div className="-mt-2 flex items-center gap-2">
              <Button variant="ghost" onClick={async () => openInBrowser(await NexusService.FilesURL(game.id, modId, 0))}>
                Open on Nexus Mods
              </Button>
            </div>

            <dl className="grid grid-cols-2 gap-3 text-sm">
              {[
                ["Endorsements", formatCount(page.endorsements)],
                ["Downloads", formatCount(page.downloads)],
              ].map(([label, value]) => (
                <div key={label} className="rounded-md bg-zinc-800/60 px-3 py-2">
                  <dt className="text-xs text-zinc-500">{label}</dt>
                  <dd className="font-medium">{value}</dd>
                </div>
              ))}
            </dl>

            {groups.map((g) => (
              <section key={g.category} className="flex flex-col gap-2">
                <h3 className="text-xs font-semibold tracking-wide text-zinc-400 uppercase">{categoryLabels[g.category]}</h3>
                <ul className="flex flex-col gap-1.5">
                  {g.files.map((f) => (
                    <li key={f.fileId} className="flex items-start gap-3 rounded-md bg-zinc-800/40 px-3 py-2">
                      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
                        <span className="text-sm font-medium break-words">{f.name}</span>
                        <span className="text-xs text-zinc-500">
                          {f.version && `v${f.version} · `}
                          {formatBytes(Number(f.sizeInBytes))} · {formatAgo(new Date(f.date * 1000).toISOString())}
                        </span>
                        {f.description && <span className="line-clamp-3 text-xs text-zinc-400">{f.description}</span>}
                      </div>
                      <Button
                        variant={g.category === "MAIN" ? "primary" : "secondary"}
                        disabled={queued.has(f.fileId)}
                        title={nexus.account?.premium ? "Download into the active profile" : "Open the file's page of Nexus Mods, with its Mod Manager Download"}
                        onClick={() => download(f)}
                      >
                        {queued.has(f.fileId) ? "Queued" : "Download"}
                      </Button>
                    </li>
                  ))}
                </ul>
              </section>
            ))}

            <section className="border-t border-zinc-800 pt-3">
              <BBCode>{page.description}</BBCode>
            </section>
          </div>
        )}
      </div>
    </div>
  );
}
