import { ReactNode } from "react";
import { Browser } from "@wailsio/runtime";

// BBCode shows the description of a mod from Nexus Mods, written in BBCode
// with HTML line breaks, as elements built here: nothing of it is taken as
// HTML, which a page of the app must never run from someone else's text.

type Node = string | { tag: string; arg: string; children: Node[] };

// The tags understood; any other stays as it was written.
const known = new Set([
  "b", "i", "u", "s", "url", "img", "center", "right", "left", "size", "color", "font",
  "list", "*", "quote", "spoiler", "youtube", "line", "heading", "code", "hr",
]);
// Tags that hold nothing and need no closing.
const empty = new Set(["line", "hr"]);

const tagPattern = /\[(\/?)([a-z*]+)(?:=([^\]]*))?\]/gi;

function parse(text: string): Node[] {
  const root = { tag: "", arg: "", children: [] as Node[] };
  const stack = [root];
  const top = () => stack[stack.length - 1];
  let at = 0;
  for (const m of text.matchAll(tagPattern)) {
    const [whole, closing, rawName, arg = ""] = m;
    const name = rawName.toLowerCase();
    if (m.index! > at) top().children.push(text.slice(at, m.index));
    at = m.index! + whole.length;
    if (!known.has(name)) {
      top().children.push(whole);
      continue;
    }
    if (!closing) {
      // An item ends where the next one starts.
      if (name === "*" && top().tag === "*") stack.pop();
      const node = { tag: name, arg: arg.trim().replace(/^["']|["']$/g, ""), children: [] as Node[] };
      top().children.push(node);
      if (!empty.has(name)) stack.push(node);
      continue;
    }
    const i = stack.map((n) => n.tag).lastIndexOf(name);
    if (i <= 0) {
      if (name !== "*") top().children.push(whole);
      continue;
    }
    stack.length = i;
  }
  if (at < text.length) top().children.push(text.slice(at));
  return root.children;
}

// clean turns the HTML the text carries, line breaks and entities, into
// plain text.
function clean(text: string): string {
  return text
    .replace(/<br\s*\/?>\n?/gi, "\n")
    .replace(/<[^>]+>/g, "")
    .replace(/&nbsp;/g, " ")
    .replace(/&quot;/g, '"')
    .replace(/&#0?39;/g, "'")
    .replace(/&lt;/g, "<")
    .replace(/&gt;/g, ">")
    .replace(/&amp;/g, "&");
}

function textOf(nodes: Node[]): string {
  return nodes.map((n) => (typeof n === "string" ? n : textOf(n.children))).join("");
}

// trimmed is nodes without the line breaks at their ends.
function trimmed(nodes: Node[]): Node[] {
  const out = [...nodes];
  if (typeof out[0] === "string") out[0] = out[0].trimStart();
  const last = out.length - 1;
  if (typeof out[last] === "string") out[last] = (out[last] as string).trimEnd();
  return out;
}

const web = (url: string) => /^https?:\/\//i.test(url);
const colour = (c: string) => /^(#[0-9a-f]{3,8}|[a-z]+)$/i.test(c);

function render(nodes: Node[], key = ""): ReactNode[] {
  return nodes.map((n, i): ReactNode => {
    const k = `${key}${i}`;
    if (typeof n === "string") return n;
    const inner = () => render(n.children, `${k}.`);
    switch (n.tag) {
      case "b":
        return <strong key={k}>{inner()}</strong>;
      case "i":
        return <em key={k}>{inner()}</em>;
      case "u":
        return <u key={k}>{inner()}</u>;
      case "s":
        return <s key={k}>{inner()}</s>;
      case "url": {
        const href = (n.arg || textOf(n.children)).trim();
        if (!web(href)) return <span key={k}>{inner()}</span>;
        return (
          <a key={k} href={href} title={href} onClick={(e) => (e.preventDefault(), Browser.OpenURL(href))} className="text-accent-400 hover:underline">
            {inner()}
          </a>
        );
      }
      case "img": {
        const src = textOf(n.children).trim();
        return web(src) ? <img key={k} src={src} alt="" loading="lazy" className="my-1 inline-block max-w-full rounded" /> : null;
      }
      case "youtube": {
        const id = textOf(n.children).trim();
        const href = `https://www.youtube.com/watch?v=${encodeURIComponent(id)}`;
        return (
          <a key={k} href={href} onClick={(e) => (e.preventDefault(), Browser.OpenURL(href))} className="text-accent-400 hover:underline">
            ▶ Video on YouTube
          </a>
        );
      }
      case "center":
      case "right":
      case "left":
        return (
          <div key={k} className={n.tag === "center" ? "text-center" : n.tag === "right" ? "text-right" : "text-left"}>
            {inner()}
          </div>
        );
      case "size": {
        const size = Number(n.arg);
        const cls = size >= 5 ? "text-lg font-semibold" : size === 4 ? "text-base" : size > 0 && size <= 2 ? "text-xs" : "";
        return (
          <span key={k} className={cls}>
            {inner()}
          </span>
        );
      }
      case "color":
        return (
          <span key={k} style={colour(n.arg) ? { color: n.arg } : undefined}>
            {inner()}
          </span>
        );
      case "heading":
        return (
          <span key={k} className="block text-base font-semibold">
            {inner()}
          </span>
        );
      case "list":
        // The line breaks between items are not lines of the list.
        return (
          <ul key={k} className={`my-1 pl-5 ${n.arg ? "list-decimal" : "list-disc"}`}>
            {render(
              n.children.filter((c) => typeof c !== "string" || c.trim() !== ""),
              `${k}.`,
            )}
          </ul>
        );
      case "*":
        return <li key={k}>{render(trimmed(n.children), `${k}.`)}</li>;
      case "quote":
        return (
          <blockquote key={k} className="my-1 border-l-2 border-zinc-700 pl-3 text-zinc-400">
            {inner()}
          </blockquote>
        );
      case "spoiler":
        return (
          <details key={k} className="my-1 rounded bg-zinc-800/60 px-3 py-1">
            <summary className="cursor-pointer text-zinc-400">{n.arg || "Spoiler"}</summary>
            {inner()}
          </details>
        );
      case "code":
        return (
          <code key={k} className="rounded bg-zinc-800 px-1 font-mono text-xs">
            {inner()}
          </code>
        );
      case "line":
      case "hr":
        return <hr key={k} className="my-2 border-zinc-800" />;
      default:
        return <span key={k}>{inner()}</span>;
    }
  });
}

export default function BBCode({ children }: { children: string }) {
  return <div className="text-sm leading-relaxed whitespace-pre-line text-zinc-300">{render(parse(clean(children)))}</div>;
}
