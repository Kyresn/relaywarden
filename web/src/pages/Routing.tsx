import { formatNumber } from "../api";
import { NumberField, Section, Segmented, ToggleRow } from "../components";
import { useI18n } from "../i18n";
import type { AuthSnapshot, Config } from "../types";

export function RoutingPage({
  config,
  update,
  auths,
}: {
  config: Config;
  update: (path: (string | number)[], value: unknown) => void;
  auths: AuthSnapshot[];
}) {
  const r = config.routing;
  const { t } = useI18n();
  const sum = r.weights.ttft + r.weights.error + r.weights.degrade;

  return (
    <>
      <Section title={t("routing.title")} hint={t("routing.hint")}>
        <div className="control-row">
          <ToggleRow
            label={t("routing.enabled")}
            hint={t("routing.enabled.hint")}
            checked={r.enabled}
            onChange={(v) => update(["routing", "enabled"], v)}
            danger={t("routing.enabled.danger")}
            tone="cobalt"
          />
          <ToggleRow
            label={t("routing.affinity")}
            hint={t("routing.affinity.hint")}
            checked={r.cache_affinity}
            onChange={(v) => update(["routing", "cache_affinity"], v)}
            tone="cobalt"
          />
        </div>

        <div className="field">
          <span className="field-label">{t("routing.strategy")}</span>
          <Segmented
            label={t("routing.strategy")}
            value={r.strategy}
            options={[
              { value: "latency_cache", label: t("routing.strategy.latency_cache") },
              { value: "latency_only", label: t("routing.strategy.latency_only") },
              { value: "off", label: t("routing.strategy.off") },
            ]}
            onChange={(v) => update(["routing", "strategy"], v)}
          />
        </div>

        <div className="field-grid">
          <NumberField label={t("routing.weight.ttft")} value={r.weights.ttft} min={0} max={1} step={0.05} onChange={(v) => update(["routing", "weights", "ttft"], v)} />
          <NumberField label={t("routing.weight.error")} value={r.weights.error} min={0} max={1} step={0.05} onChange={(v) => update(["routing", "weights", "error"], v)} />
          <NumberField label={t("routing.weight.degrade")} value={r.weights.degrade} min={0} max={1} step={0.05} onChange={(v) => update(["routing", "weights", "degrade"], v)} />
          <div className={`weight-sum ${Math.abs(sum - 1) > 0.01 ? "is-bad" : ""}`}>{t("routing.sum", { sum: sum.toFixed(2) })}</div>
        </div>

        <div className="field-grid">
          <NumberField label={t("routing.window")} value={r.window} min={3} onChange={(v) => update(["routing", "window"], v)} />
          <NumberField label={t("routing.ttl")} value={r.ttl_seconds} min={60} onChange={(v) => update(["routing", "ttl_seconds"], v)} />
          <NumberField label={t("routing.min_samples")} value={r.min_samples} min={1} onChange={(v) => update(["routing", "min_samples"], v)} />
          <NumberField label={t("routing.affinity_ttl")} value={r.affinity_ttl_seconds} min={60} onChange={(v) => update(["routing", "affinity_ttl_seconds"], v)} />
        </div>
      </Section>

      <Section title={t("routing.auths.title")} hint={t("routing.auths.hint")}>
        {auths.length === 0 ? (
          <div className="empty">{t("routing.auths.empty")}</div>
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr><th>{t("table.auth")}</th><th>{t("table.model")}</th><th className="num">{t("table.samples")}</th><th className="num">{t("table.p50")}</th><th className="num">{t("table.p95")}</th><th className="num">{t("table.error")}</th><th className="num">{t("table.degraded")}</th><th>{t("routing.score")}</th></tr>
              </thead>
              <tbody>
                {auths.map((a) => {
                  const score = a.avg_ttft_ms > 0 ? Math.min(1, a.error_rate + a.degrade_rate + a.avg_ttft_ms / Math.max(1, Math.max(...auths.map((x) => x.avg_ttft_ms)))) / 3 : 0.5;
                  return (
                    <tr key={`${a.auth_id}:${a.model}`}>
                      <td className="mono">{a.auth_id}</td>
                      <td>{a.model || "—"}</td>
                      <td className="num">{formatNumber(a.samples)}</td>
                      <td className="num">{a.avg_ttft_ms ? `${a.avg_ttft_ms} ms` : "—"}</td>
                      <td className="num">{a.p95_ttft_ms ? `${a.p95_ttft_ms} ms` : "—"}</td>
                      <td className="num">{(a.error_rate * 100).toFixed(1)}%</td>
                      <td className="num">{a.signals}</td>
                      <td><span className="score-bar"><span style={{ width: `${Math.round(score * 100)}%` }} /></span><span className="mono score-num">{score.toFixed(2)}</span></td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </Section>
    </>
  );
}
