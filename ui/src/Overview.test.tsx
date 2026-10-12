import { createI18n } from "@edelweiss/sdk";
import { FakeEventSource, fakeSession, recordingToasts, TestShell } from "@edelweiss/sdk/testing";
import { cleanup, fireEvent, render, waitFor } from "@solidjs/testing-library";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import de from "../public/locales/de.json";
import en from "../public/locales/en.json";
import { Overview } from "./Overview";

const info = {
  hostname: "alpine",
  os: { name: "Alpine Linux", version: "3.24.2", prettyName: "Alpine Linux v3.24" },
  kernel: "6.6.50-0-lts",
  arch: "x86_64",
  cpu: { model: "Test CPU", cores: 4 },
  memoryTotal: 2 * 1024 ** 3,
  bootTime: new Date(Date.now() - 3600_000).toISOString(),
  uptimeSeconds: 3600,
};

const metrics = {
  time: new Date().toISOString(),
  cpu: 42,
  load: [0.5, 0.25, 0.1],
  memory: { total: 2 * 1024 ** 3, available: 1024 ** 3, used: 1024 ** 3, swapTotal: 0, swapUsed: 0 },
  disks: [{ mount: "/", device: "/dev/sda3", fsType: "ext4", total: 10 * 1024 ** 3, used: 5 * 1024 ** 3 }],
  net: [{ iface: "eth0", rxBps: 2048, txBps: 1024 }],
};

let fetchMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  FakeEventSource.instances = [];
  vi.stubGlobal("EventSource", FakeEventSource);
  fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
    if (url === "/ext/system/api/info") return new Response(JSON.stringify(info), { status: 200 });
    if (url === "/ext/system/api/hostname" && init?.method === "PUT") {
      return new Response(JSON.stringify({ hostname: "nas-01" }), { status: 200 });
    }
    return new Response(null, { status: 404 });
  });
  vi.stubGlobal("fetch", fetchMock);
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function mount(admin: boolean, canAdmin = true) {
  const i18n = createI18n("en");
  i18n.register("system", "en", en);
  i18n.register("system", "de", de);
  const session = fakeSession({ admin, canAdmin });
  const toasts = recordingToasts();
  const view = render(() => (
    <TestShell extension="system" i18n={i18n} session={session.api} toasts={toasts}>
      <Overview />
    </TestShell>
  ));
  return { ...view, toasts, session };
}

describe("Overview", () => {
  it("shows host information and live metrics", async () => {
    const view = mount(false);
    expect((await view.findByRole("heading", { name: "alpine" })).textContent).toBe("alpine");
    expect(view.getByText("Alpine Linux v3.24")).toBeTruthy();
    const es = FakeEventSource.instances[0]!;
    expect(es.url).toBe("/ext/system/api/metrics");
    es.open();
    es.emit("metrics", JSON.stringify(metrics));
    await waitFor(() =>
      expect(view.getByRole("progressbar", { name: "CPU" }).getAttribute("aria-valuenow")).toBe("42"),
    );
    expect(view.getByText("/dev/sda3")).toBeTruthy();
    expect(view.getByText("eth0")).toBeTruthy();
    expect(view.getByText("2 KiB/s")).toBeTruthy();
  });

  it("keeps the table rows when the next metrics event arrives, so their text stays selectable", async () => {
    const view = mount(false);
    await view.findByRole("heading", { name: "alpine" });
    const es = FakeEventSource.instances[0]!;
    es.open();
    es.emit("metrics", JSON.stringify(metrics));
    const diskRow = (await view.findByText("/dev/sda3")).closest("tr")!;
    const netRow = view.getByText("eth0").closest("tr")!;
    const mountCell = view.getByText("/").closest("td")!;

    // the same mount and interface, as new objects with new numbers: what every event looks like
    es.emit(
      "metrics",
      JSON.stringify({
        ...metrics,
        disks: [{ ...metrics.disks[0], used: 6 * 1024 ** 3 }],
        net: [{ iface: "eth0", rxBps: 4096, txBps: 1024 }],
      }),
    );
    await waitFor(() => expect(view.getByText("4 KiB/s")).toBeTruthy());
    expect(view.getByText(/6 GiB/)).toBeTruthy();
    expect(diskRow.isConnected).toBe(true);
    expect(netRow.isConnected).toBe(true);
    expect(view.getByText("/dev/sda3").closest("tr")).toBe(diskRow);
    expect(view.getByText("eth0").closest("tr")).toBe(netRow);
    expect(view.getByText("/").closest("td")).toBe(mountCell);
  });

  it("changes the hostname in admin mode", async () => {
    const view = mount(true);
    fireEvent.click(await view.findByRole("button", { name: "Change hostname" }));
    const input = view.getByLabelText("New hostname") as HTMLInputElement;
    fireEvent.input(input, { target: { value: "NAS-01" } });
    fireEvent.submit(input.closest("form")!);
    await waitFor(() => expect(view.toasts.shown[0]?.message).toBe("Hostname changed to nas-01."));
    const put = fetchMock.mock.calls.find((c) => (c[1] as RequestInit | undefined)?.method === "PUT")!;
    expect(JSON.parse((put[1] as RequestInit).body as string)).toEqual({ hostname: "nas-01" });
    expect((await view.findByRole("heading", { name: "nas-01" })).textContent).toBe("nas-01");
  });

  it("reloads the session after the hostname changed, so the navbar shows the new name", async () => {
    const view = mount(true);
    fireEvent.click(await view.findByRole("button", { name: "Change hostname" }));
    const input = view.getByLabelText("New hostname") as HTMLInputElement;
    fireEvent.input(input, { target: { value: "nas-01" } });
    expect(view.session.calls.refresh).toBe(0);
    fireEvent.submit(input.closest("form")!);
    await waitFor(() => expect(view.session.calls.refresh).toBe(1));
  });

  it("ignores a failing session reload after the hostname changed", async () => {
    const view = mount(true);
    const refresh = vi.fn(async () => {
      throw new Error("session unavailable");
    });
    view.session.api.refresh = refresh;
    fireEvent.click(await view.findByRole("button", { name: "Change hostname" }));
    const input = view.getByLabelText("New hostname") as HTMLInputElement;
    fireEvent.input(input, { target: { value: "nas-01" } });
    fireEvent.submit(input.closest("form")!);
    await waitFor(() => expect(refresh).toHaveBeenCalledOnce());
    await waitFor(() => expect(view.toasts.shown.map((s) => s.kind)).toEqual(["success"]));
    expect((await view.findByRole("heading", { name: "nas-01" })).textContent).toBe("nas-01");
    expect(view.queryByRole("dialog")).toBeNull();
  });

  it("ignores a second submit while the admin prompt is open and allows a new try after a refusal", async () => {
    const view = mount(false);
    const answers: ((admin: boolean) => void)[] = [];
    view.session.api.requestAdmin = () => new Promise<boolean>((resolve) => answers.push(resolve));
    fireEvent.click(await view.findByRole("button", { name: "Change hostname" }));
    const input = view.getByLabelText("New hostname") as HTMLInputElement;
    const form = input.closest("form")!;
    const save = view.getByRole("button", { name: "Save" }) as HTMLButtonElement;
    const puts = () =>
      fetchMock.mock.calls.filter((c) => (c[1] as RequestInit | undefined)?.method === "PUT");
    fireEvent.input(input, { target: { value: "nas-02" } });

    fireEvent.submit(form);
    fireEvent.submit(form); // e.g. Enter pressed twice
    expect(answers).toHaveLength(1);
    expect(save.disabled).toBe(true);

    answers[0]?.(false); // the admin prompt was dismissed
    await waitFor(() => expect(save.disabled).toBe(false));
    expect(puts()).toHaveLength(0);

    fireEvent.submit(form);
    expect(answers).toHaveLength(2);
    answers[1]?.(true);
    await waitFor(() => expect(view.toasts.shown[0]?.message).toBe("Hostname changed to nas-01."));
    expect(puts()).toHaveLength(1);
  });

  it("asks for admin mode before changing the hostname", async () => {
    const view = mount(false);
    fireEvent.click(await view.findByRole("button", { name: "Change hostname" }));
    const input = view.getByLabelText("New hostname") as HTMLInputElement;
    fireEvent.input(input, { target: { value: "nas-02" } });
    fireEvent.submit(input.closest("form")!);
    await waitFor(() => expect(view.session.calls.requestAdmin).toBe(1));
    expect(fetchMock.mock.calls.some((c) => (c[1] as RequestInit | undefined)?.method === "PUT")).toBe(false);
  });

  it("offers no hostname change to users outside the admin group", async () => {
    const view = mount(false, false);
    await view.findByRole("heading", { name: "alpine" });
    expect(view.queryByRole("button", { name: "Change hostname" })).toBeNull();
  });
});
