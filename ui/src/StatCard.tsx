import { Sparkline } from "./Sparkline";

export function StatCard(props: { title: string; value: number; detail: string; history: number[] }) {
  const rounded = () => Math.round(props.value);
  return (
    <div class="card bg-base-200">
      <div class="card-body gap-3">
        <div class="flex items-center justify-between gap-4">
          <div>
            <h2 class="card-title text-base">{props.title}</h2>
            <p class="text-sm text-base-content/70">{props.detail}</p>
          </div>
          <div
            class="radial-progress text-primary"
            style={{ "--value": rounded(), "--size": "4rem" }}
            role="progressbar"
            aria-label={props.title}
            aria-valuenow={rounded()}
            aria-valuemin={0}
            aria-valuemax={100}
          >
            {rounded()}%
          </div>
        </div>
        <Sparkline values={props.history} />
      </div>
    </div>
  );
}
