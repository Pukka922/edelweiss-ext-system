import { useI18n } from "@edelweiss/sdk";
import { Index, Show } from "solid-js";
import { formatRate } from "./format";
import type { NetRate } from "./types";

export function NetTable(props: { net: NetRate[] }) {
  const { t, locale } = useI18n();
  return (
    <div class="card min-w-0 bg-base-200">
      <div class="card-body">
        <h2 class="card-title text-base">{t("net.title")}</h2>
        <Show
          when={props.net.length > 0}
          fallback={<p class="text-sm text-base-content/70">{t("net.empty")}</p>}
        >
          <div class="overflow-x-auto">
            <table class="table table-sm">
              <thead>
                <tr>
                  <th>{t("net.iface")}</th>
                  <th class="text-right">{t("net.rx")}</th>
                  <th class="text-right">{t("net.tx")}</th>
                </tr>
              </thead>
              <tbody>
                {/* Index, not For: every metrics event brings new objects, For would recreate the rows (no text selection). */}
                <Index each={props.net}>
                  {(n) => (
                    <tr>
                      <td>{n().iface}</td>
                      <td class="text-right tabular-nums">{formatRate(n().rxBps, locale())}</td>
                      <td class="text-right tabular-nums">{formatRate(n().txBps, locale())}</td>
                    </tr>
                  )}
                </Index>
              </tbody>
            </table>
          </div>
        </Show>
      </div>
    </div>
  );
}
