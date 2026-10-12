import { ErrorAlert, LoadingState, PageHeader, useApi, useI18n, useSession, useStream } from "@edelweiss/sdk";
import { createEffect, createResource, createSignal, Match, on, onCleanup, Show, Switch } from "solid-js";
import { DiskTable } from "./DiskTable";
import { formatBytes, percent, uptimeParts } from "./format";
import { HostnameDialog } from "./HostnameDialog";
import { pushSample } from "./history";
import { NetTable } from "./NetTable";
import { StatCard } from "./StatCard";
import type { Info, Metrics } from "./types";

export function Overview() {
  const api = useApi();
  const session = useSession();
  const { t, locale } = useI18n();
  const [info, { mutate, refetch }] = createResource(() => api.get<Info>("/info"));
  // reading an errored resource throws; everything outside the Switch uses host()
  const host = () => (info.error ? undefined : info());
  const stream = useStream<Metrics>("/metrics", { event: "metrics" });
  const [cpu, setCpu] = createSignal<number[]>([]);
  const [mem, setMem] = createSignal<number[]>([]);
  const [swap, setSwap] = createSignal<number[]>([]);
  const [editing, setEditing] = createSignal(false);
  const [now, setNow] = createSignal(Date.now());
  const tick = setInterval(() => setNow(Date.now()), 60_000);
  onCleanup(() => clearInterval(tick));

  createEffect(
    on(stream.data, (m) => {
      if (!m) return;
      setCpu((h) => pushSample(h, m.cpu));
      setMem((h) => pushSample(h, percent(m.memory.used, m.memory.total)));
      setSwap((h) => pushSample(h, percent(m.memory.swapUsed, m.memory.swapTotal)));
    }),
  );

  const uptime = (i: Info) => {
    const p = uptimeParts((now() - Date.parse(i.bootTime)) / 1000);
    return [
      p.days ? t("uptime.days", { n: p.days }) : "",
      p.hours ? t("uptime.hours", { n: p.hours }) : "",
      t("uptime.minutes", { n: p.minutes }),
    ]
      .filter(Boolean)
      .join(" ");
  };
  const bytes = (n: number) => formatBytes(n, locale());

  return (
    <div class="flex flex-col gap-6">
      <Switch fallback={<LoadingState label={t("loading")} />}>
        <Match when={info.error}>
          <ErrorAlert error={info.error} onRetry={() => void refetch()} />
        </Match>
        <Match when={host()}>
          {(i) => (
            <>
              <PageHeader
                title={i().hostname}
                subtitle={`${i().os.prettyName} · ${i().arch}`}
                actions={
                  <Show when={session.canAdmin()}>
                    <button type="button" class="btn btn-sm" onClick={() => setEditing(true)}>
                      {t("hostname.change")}
                    </button>
                  </Show>
                }
              />
              <div class="card bg-base-200">
                <div class="card-body">
                  <h2 class="card-title text-base">{t("info.title")}</h2>
                  <dl class="grid gap-x-6 gap-y-2 sm:grid-cols-2 lg:grid-cols-3">
                    <div>
                      <dt class="text-xs text-base-content/60">{t("info.os")}</dt>
                      <dd>{i().os.prettyName}</dd>
                    </div>
                    <div>
                      <dt class="text-xs text-base-content/60">{t("info.kernel")}</dt>
                      <dd>{i().kernel}</dd>
                    </div>
                    <div>
                      <dt class="text-xs text-base-content/60">{t("info.arch")}</dt>
                      <dd>{i().arch}</dd>
                    </div>
                    <div>
                      <dt class="text-xs text-base-content/60">{t("info.cpu")}</dt>
                      <dd>{i().cpu.model || "–"}</dd>
                    </div>
                    <div>
                      <dt class="text-xs text-base-content/60">{t("info.memory")}</dt>
                      <dd>{bytes(i().memoryTotal)}</dd>
                    </div>
                    <div>
                      <dt class="text-xs text-base-content/60">{t("info.uptime")}</dt>
                      <dd>{uptime(i())}</dd>
                    </div>
                  </dl>
                </div>
              </div>
            </>
          )}
        </Match>
      </Switch>

      <Show when={stream.status() !== "open"}>
        <div class="badge badge-ghost">
          {stream.status() === "error" ? t("stream.error") : t("stream.connecting")}
        </div>
      </Show>

      <Show when={stream.data()}>
        {(m) => (
          <>
            <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
              <StatCard
                title={t("cpu.title")}
                value={m().cpu}
                detail={t("cpu.detail", { cores: host()?.cpu.cores ?? "–" })}
                history={cpu()}
              />
              <StatCard
                title={t("memory.title")}
                value={percent(m().memory.used, m().memory.total)}
                detail={`${bytes(m().memory.used)} / ${bytes(m().memory.total)}`}
                history={mem()}
              />
              <StatCard
                title={t("swap.title")}
                value={percent(m().memory.swapUsed, m().memory.swapTotal)}
                detail={
                  m().memory.swapTotal > 0
                    ? `${bytes(m().memory.swapUsed)} / ${bytes(m().memory.swapTotal)}`
                    : t("swap.none")
                }
                history={swap()}
              />
              <div class="card bg-base-200">
                <div class="card-body">
                  <h2 class="card-title text-base">{t("load.title")}</h2>
                  <p class="text-2xl font-semibold tabular-nums">
                    {m()
                      .load.map((v) => v.toFixed(2))
                      .join(" · ")}
                  </p>
                  <p class="text-sm text-base-content/70">{t("load.detail")}</p>
                </div>
              </div>
            </div>
            <div class="grid gap-4 xl:grid-cols-2">
              <DiskTable disks={m().disks} />
              <NetTable net={m().net} />
            </div>
          </>
        )}
      </Show>

      <HostnameDialog
        open={editing()}
        current={host()?.hostname ?? ""}
        onClose={() => setEditing(false)}
        onSaved={(hostname) => {
          const current = host();
          if (current) mutate({ ...current, hostname });
        }}
      />
    </div>
  );
}
