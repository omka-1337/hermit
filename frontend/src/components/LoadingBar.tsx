import { stageColors } from "../downloads";

// LoadingBar is the bar in the middle of the status bar while Hermit loads
// something, from disk or into memory, for as long as that takes: a stretch
// in the accent colour runs from end to end. The spaces beside it are where
// the count and the percent of downloads go, so that it does not move.
export default function LoadingBar() {
  return (
    <div className="absolute left-1/2 flex -translate-x-1/2 items-center gap-2" title="Loading">
      <span className="w-6" />
      <span className="relative h-1.5 w-64 overflow-hidden rounded-full bg-zinc-800">
        <span className={`loading-bar absolute inset-y-0 w-2/5 rounded-full ${stageColors.load}`} />
      </span>
      <span className="w-9" />
    </div>
  );
}
