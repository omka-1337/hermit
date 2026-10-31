import { useEffect, useRef, useState } from "react";
import { Events } from "@wailsio/runtime";
import { BethesdaService, errorMessage, PreparedMod, UnpackProgress } from "./api";
import DuplicateModModal from "./components/DuplicateModModal";

// QueueStatus is what the status bar shows of a queue of installs.
export type QueueStatus = {
  total: number;
  finished: number;
  // action is what is being done right now.
  action: string;
  unpack: UnpackProgress | null;
};

type Job = { file: string; name: string; gameId: string; profileId: string; fomod: boolean };
type Prepared = { job: Job; prepared: PreparedMod };
type Question = { prepared: PreparedMod; answer: (go: boolean) => void };

const baseName = (file: string) => file.split("/").pop() ?? file;

// useInstallQueue installs mod archives one after another. Archives are
// unpacked in the background, one at a time, and those that need nothing
// more are installed right away; those whose mod the profile has, or with a
// FOMOD installer, wait their turn for a question or the installer's window,
// one at a time, while the rest goes on unpacking. Archives added while the
// queue runs join it.
export function useInstallQueue(onError: (message: string) => void) {
  const [status, setStatus] = useState<QueueStatus | null>(null);
  const [question, setQuestion] = useState<Question | null>(null);
  const q = useRef({
    toUnpack: [] as Job[],
    toAsk: [] as Prepared[],
    unpacking: false,
    asking: false,
    total: 0,
    finished: 0,
    fomods: 0,
    fomodsOpened: 0,
    action: "",
    unpack: null as UnpackProgress | null,
    errors: [] as string[],
    // lingering keeps the last count on screen for a moment after the end.
    lingering: false,
  });
  // waiting maps the sessions of open installer windows to what goes on once
  // they are done with.
  const waiting = useRef(new Map<string, () => void>());
  const onErrorRef = useRef(onError);
  onErrorRef.current = onError;

  const show = () => {
    const s = q.current;
    if (!s.total && s.lingering) return;
    setStatus(s.total ? { total: s.total, finished: s.finished, action: s.action, unpack: s.unpack } : null);
  };

  useEffect(() => {
    const done = (ev: { data?: { session?: string } }) => {
      const session = ev.data?.session;
      const go = session ? waiting.current.get(session) : undefined;
      if (session && go) {
        waiting.current.delete(session);
        go();
      }
    };
    const offInstalled = Events.On("bethesda:installed", done);
    const offClosed = Events.On("bethesda:install-closed", done);
    const offUnpack = Events.On("bethesda:unpack", (ev) => {
      if (!q.current.unpacking) return;
      q.current.unpack = ev.data;
      show();
    });
    return () => {
      offInstalled();
      offClosed();
      offUnpack();
    };
  }, []);

  const finishOne = () => {
    const s = q.current;
    s.finished++;
    if (s.finished < s.total) return show();
    s.action = "Done";
    s.unpack = null;
    show();
    const errors = s.errors;
    Object.assign(s, { total: 0, finished: 0, fomods: 0, fomodsOpened: 0, action: "", unpack: null, errors: [], lingering: true });
    setTimeout(() => {
      s.lingering = false;
      show();
    }, 1500);
    if (errors.length) onErrorRef.current(errors.join("\n"));
  };

  const fail = (name: string, err: unknown) => q.current.errors.push(`${name}: ${errorMessage(err)}`);

  const unpackAll = async () => {
    const s = q.current;
    if (s.unpacking) return;
    s.unpacking = true;
    while (s.toUnpack.length) {
      const job = s.toUnpack.shift()!;
      s.action = `Unpacking ${job.name}`;
      s.unpack = null;
      show();
      try {
        const prepared = await BethesdaService.PrepareInstall(job.gameId, job.profileId, job.file);
        if (prepared.existing || prepared.fomod) {
          s.toAsk.push({ job, prepared });
          askAll();
          continue;
        }
        s.action = `Installing ${prepared.name}`;
        s.unpack = null;
        show();
        await BethesdaService.FinishInstall(prepared.session, null);
      } catch (err) {
        fail(job.name, err);
        if (job.fomod) s.fomods--;
      }
      finishOne();
    }
    s.unpacking = false;
    s.unpack = null;
    show();
  };

  const askAll = async () => {
    const s = q.current;
    if (s.asking) return;
    s.asking = true;
    while (s.toAsk.length) {
      const { job, prepared } = s.toAsk.shift()!;
      try {
        if (prepared.existing) {
          const go = await new Promise<boolean>((answer) => setQuestion({ prepared, answer }));
          setQuestion(null);
          if (!go) {
            await BethesdaService.CancelInstall(prepared.session);
            if (prepared.fomod) s.fomods--;
            finishOne();
            continue;
          }
        }
        if (prepared.fomod) {
          s.fomodsOpened++;
          const closed = new Promise<void>((go) => waiting.current.set(prepared.session, go));
          await BethesdaService.OpenFomod(prepared.session, s.fomodsOpened, s.fomods);
          await closed;
        } else {
          await BethesdaService.FinishInstall(prepared.session, null);
        }
      } catch (err) {
        fail(job.name, err);
      }
      finishOne();
    }
    s.asking = false;
  };

  // add puts archives into the queue of the active profile of a game.
  const add = async (gameId: string, profileId: string, files: string[]) => {
    if (!files.length) return;
    const fomods = await BethesdaService.CheckFomods(files);
    const s = q.current;
    files.forEach((file, i) =>
      s.toUnpack.push({ file, name: baseName(file), gameId, profileId, fomod: !!fomods?.[i] }),
    );
    s.total += files.length;
    s.fomods += (fomods ?? []).filter(Boolean).length;
    show();
    unpackAll();
  };

  const dialog = question && (
    <DuplicateModModal
      prepared={question.prepared}
      onChoose={async (target, name) => {
        await BethesdaService.ChooseTarget(question.prepared.session, target, name);
        question.answer(true);
      }}
      onCancel={() => question.answer(false)}
    />
  );

  return { add, status, dialog };
}
