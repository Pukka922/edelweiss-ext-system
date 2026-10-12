import { useI18n } from "@edelweiss/sdk";
import { Index, Show } from "solid-js";
import { formatBytes, percent } from "./format";
import type { Disk } from "./types";

export function DiskTable(props: { disks: Disk[] }) {
  const { t, locale } = useI18n();
  return (
    <div class="card min-w-0 bg-base-200">
      <div class="card-body">
        <h2 class="card-title text-base">{t("disks.title")}</h2>
        <Show
          when={props.disks.length > 0}
          fallback={<p class="text-sm text-base-content/70">{t("disks.empty")}</p>}
        >
          <div class="overflow-x-auto">
            <table class="table table-sm">
              <thead>
                <tr>
                  <th>{t("disks.mount")}</th>
                  <th>{t("disks.device")}</th>
                  <th>{t("disks.type")}</th>
                  <th class="w-1/3">{t("disks.used")}</th>
                </tr>
              </thead>
              <tbody>
                {/* Index, not For: every metrics event brings new objects, For would recreate the rows (no text selection). */}
                <Index each={props.disks}>
                  {(d) => (
                    <tr>
                      <td>
                        <code>{d().mount}</code>
                      </td>
                      <td>{d().device}</td>
                      <td>{d().fsType}</td>
                      <td>
                        <div class="flex items-center gap-2">
                          <progress
                            class="progress progress-primary w-full"
                            value={percent(d().used, d().total)}
                            max="100"
                          />
                          <span class="whitespace-nowrap text-xs">
                            {formatBytes(d().used, locale())} / {formatBytes(d().total, locale())}
                          </span>
                        </div>
                      </td>
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
