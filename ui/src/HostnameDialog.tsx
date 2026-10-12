import { ApiError, useApi, useI18n, useSession, useToast } from "@edelweiss/sdk";
import { createEffect, createSignal, Show } from "solid-js";

const label = /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/;
const digitsOnly = /^[0-9]+$/;
/** Linux HOST_NAME_MAX: sethostname(2) fails for longer names. */
const maxLength = 64;

/**
 * Same rule as the backend (ValidHostname): RFC 1123 labels, lower case, at most 64 characters
 * (the kernel limit), and a last label that is not all digits (no dotted-decimal names like 192.168.1.1).
 */
export function isValidHostname(h: string): boolean {
  if (h.length === 0 || h.length > maxLength) return false;
  const labels = h.split(".");
  return labels.every((part) => label.test(part)) && !digitsOnly.test(labels[labels.length - 1] ?? "");
}

export function HostnameDialog(props: {
  open: boolean;
  current: string;
  onClose: () => void;
  onSaved: (hostname: string) => void;
}) {
  const api = useApi();
  const session = useSession();
  const toast = useToast();
  const { t } = useI18n();
  const [value, setValue] = createSignal("");
  const [error, setError] = createSignal<string>();
  const [busy, setBusy] = createSignal(false);

  createEffect(() => {
    if (props.open) {
      setValue(props.current);
      setError(undefined);
    }
  });

  const save = async (e: SubmitEvent) => {
    e.preventDefault();
    if (busy()) return; // a second submit (Enter) while the admin prompt or the request is pending
    const name = value().trim().toLowerCase();
    if (!isValidHostname(name)) {
      setError(t("hostname.invalid"));
      return;
    }
    setBusy(true); // before requestAdmin(): the prompt can stay open for a long time
    setError(undefined);
    try {
      if (!session.admin() && !(await session.requestAdmin())) return;
      const res = await api.put<{ hostname: string }>("/hostname", { hostname: name });
      toast.success(t("hostname.saved", { hostname: res.hostname }));
      props.onSaved(res.hostname);
      props.onClose();
      // the navbar shows the hostname of the session data; a failing reload is not worth reporting
      session.refresh().catch(() => {});
    } catch (err) {
      setError(
        err instanceof ApiError && err.code === "invalid_hostname"
          ? t("hostname.invalid")
          : t("hostname.failed", { message: err instanceof Error ? err.message : String(err) }),
      );
    } finally {
      setBusy(false);
    }
  };

  return (
    <Show when={props.open}>
      <div class="modal modal-open" role="dialog" aria-modal="true" aria-label={t("hostname.title")}>
        <form class="modal-box flex flex-col gap-4" onSubmit={(e) => void save(e)}>
          <h3 class="text-lg font-bold">{t("hostname.title")}</h3>
          <Show when={error()}>
            <div role="alert" class="alert alert-error text-sm">
              {error()}
            </div>
          </Show>
          <label class="flex flex-col gap-1">
            <span class="text-sm">{t("hostname.label")}</span>
            <input
              class="input w-full"
              value={value()}
              onInput={(e) => setValue(e.currentTarget.value)}
              required
            />
          </label>
          <div class="modal-action">
            <button type="button" class="btn" onClick={() => props.onClose()}>
              {t("hostname.cancel")}
            </button>
            <button type="submit" class="btn btn-primary" disabled={busy()}>
              {t("hostname.save")}
            </button>
          </div>
        </form>
      </div>
    </Show>
  );
}
