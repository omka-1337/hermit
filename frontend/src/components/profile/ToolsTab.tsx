import { useEffect, useState } from "react";
import { BethesdaService, errorMessage, Tool } from "../../api";
import { useLaunchInfo } from "../launch";
import { Button, ErrorText, Spinner } from "../ui";
import { useProfile } from "./ProfileContext";

// ToolsTab lists the tools the profile's mods bring, Pandora or BodySlide,
// and runs them the way the game runs: in its Proton prefix, with the
// profile's mods in Data. What a tool writes there stays in the profile.
export default function ToolsTab() {
  const { game, profile } = useProfile();
  const { info, reload } = useLaunchInfo(game.id);
  const [tools, setTools] = useState<Tool[] | null>(null);
  const [starting, setStarting] = useState<string | null>(null);
  const [error, setError] = useState("");

  // Mods switched on or off bring tools or take them away.
  const enabled = (profile.mods ?? [])
    .filter((m) => m.enabled)
    .map((m) => m.id)
    .join("\n");
  useEffect(() => {
    BethesdaService.Tools(game.id)
      .then((list) => setTools(list ?? []))
      .catch((err) => setError(errorMessage(err)));
  }, [game.id, enabled]);

  const run = async (tool: Tool) => {
    setError("");
    setStarting(tool.path);
    try {
      await BethesdaService.RunTool(game.id, tool.path);
      // Proton takes a moment; the session shows up once the tool runs.
      setTimeout(() => {
        setStarting(null);
        reload();
      }, 5000);
    } catch (err) {
      setStarting(null);
      setError(errorMessage(err));
    }
  };

  return (
    <div className="h-full overflow-y-auto px-6 py-3">
      <ErrorText>{error}</ErrorText>
      {tools?.length === 0 && <p className="py-3 text-sm text-zinc-500">None of the enabled mods brings a tool.</p>}
      <ul className="divide-y divide-zinc-800">
        {tools?.map((tool) => {
          const running = !!info?.running && info.runningTool === tool.name;
          return (
            <li key={tool.path} className="flex items-center gap-3 py-2.5">
              <div className="flex min-w-0 flex-1 flex-col">
                <span className="truncate text-sm font-medium">{tool.name}</span>
                <span className="truncate text-xs text-zinc-500" title={tool.path}>
                  {tool.mod}
                </span>
              </div>
              {running || starting === tool.path ? (
                <span className="flex items-center gap-2 text-xs text-zinc-300">
                  <Spinner />
                  {running ? "Running" : "Starting…"}
                </span>
              ) : (
                <Button
                  disabled={!!info?.running || starting !== null}
                  title={info?.running ? "Wait until the game or the tool that runs has exited" : undefined}
                  onClick={() => run(tool)}
                >
                  Run
                </Button>
              )}
            </li>
          );
        })}
      </ul>
    </div>
  );
}
