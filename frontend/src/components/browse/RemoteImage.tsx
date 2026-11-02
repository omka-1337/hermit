import { useState } from "react";

// RemoteImage is a picture from a website, such as a mod's on Nexus Mods,
// filling its box: a shimmer while it loads, the picture fading in once it
// is there, and a sign of a broken picture if it does not come. It loads at
// once rather than lazily: WebKit left lazy pictures of a page made while the
// window switched games unloaded for good, and a page of them is small.
export default function RemoteImage({ src, className = "" }: { src: string; className?: string }) {
  const [state, setState] = useState<"loading" | "loaded" | "failed">(src ? "loading" : "failed");
  return (
    <div className={`relative overflow-hidden bg-zinc-800 ${className}`}>
      {state === "loading" && <div className="shimmer absolute inset-0" />}
      {state === "failed" ? (
        <div className="absolute inset-0 flex items-center justify-center text-zinc-600" title={src ? "The picture did not load" : undefined}>
          <svg viewBox="0 0 24 24" className="h-6 w-6" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
            <rect x="3" y="3" width="18" height="18" rx="2" />
            <circle cx="9" cy="9" r="2" />
            <path d="m21 15-3.1-3.1a2 2 0 0 0-2.8 0L6 21" />
          </svg>
        </div>
      ) : (
        <img
          src={src}
          alt=""
          onLoad={() => setState("loaded")}
          onError={() => setState("failed")}
          className={`h-full w-full object-cover transition-opacity duration-300 ${state === "loaded" ? "opacity-100" : "opacity-0"}`}
        />
      )}
    </div>
  );
}
