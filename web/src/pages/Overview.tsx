import { formatBytes, formatNumber, formatTime } from "../api";
import { Empty, Section, SeverityLamp } from "../components";
import { useI18n } from "../i18n";
import type { DashboardData } from "../types";

export function Overview({ data }: { data: DashboardData }) {
  const { t } = useI18n();
  const { status, stats, auths, signals } = data;
  const total = stats.total;
  const savedPct = total.bytes_before > 0 ? (total.bytes_before - total.bytes_after) / total.bytes_before * 100 : 0;
  const critical = signals.filter((s) => s.severity === "critical").length;
  const high = signals.filter((s) => s.severity === "high").length;
  const p50 = median(auths.map((a) => a.avg_ttft_ms));
  const p95 = auths.length ? Math.max(...auths.map((a) => a.p95_ttft_ms)) : 0;
  const hasTTFT = p50 > 0 || p95 > 0;

  return (
    <>
      <Section title={t("overview.title")} hint={t("overview.hint")}>
        <div className="readout-grid">
          <Readout label={t("overview.saved")} value={savedPct > 0 ? `${savedPct.toFixed(1)}%` : "0%"} note={t("overview.saved.note", { saved: formatBytes(total.bytes_before - total.bytes_after), before: formatBytes(total.bytes_before) })} tone="green" />
          <Readout label={t("overview.requests")} value={formatNumber(total.requests)} note={t("overview.requests.note", { n: formatNumber(total.blocks) })} />
          <Readout label={t("overview.alerts")} value={formatNumber(signals.length)} note={t("overview.alerts.note", { critical, high })} tone={critical ? "red" : high ? "amber" : "green"} />
          <Readout label={t("overview.ttft")} value={hasTTFT ? `${formatNumber(p50)} / ${formatNumber(p95)} ms` : "—"} note={t("overview.ttft.note", { n: auths.length })} tone="cobalt" />
        </div>
      </Section>

      <Section title={t("overview.auths.title")} hint={t("overview.auths.hint")}>
        {auths.length === 0 ? (
          <Empty>{t("overview.auths.empty")}</Empty>
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>{t("table.auth")}</th>
                  <th>{t("table.model")}</th>
                  <th className="num">{t("table.samples")}</th>
                  <th className="num">{t("table.p50")}</th>
                  <th className="num">{t("table.p95")}</th>
                  <th className="num">{t("table.error")}</th>
                  <th className="num">{t("table.degraded")}</th>
                  <th>{t("table.state")}</th>
                </tr>
              </thead>
              <tbody>
                {auths.map((a) => (
                  <tr key={`${a.auth_id}:${a.model}`}>
                    <td className="mono">{a.auth_id}</td>
                    <td>{a.model || "—"}</td>
                    <td className="num">{a.samples}</td>
                    <td className="num">{a.avg_ttft_ms ? `${a.avg_ttft_ms} ms` : "—"}</td>
                    <td className="num">{a.p95_ttft_ms ? `${a.p95_ttft_ms} ms` : "—"}</td>
                    <td className="num">{a.error_rate ? `${(a.error_rate * 100).toFixed(1)}%` : "0%"}</td>
                    <td className="num">{a.signals}</td>
                    <td><StatePill samples={a.samples} error={a.error_rate} degrade={a.degrade_rate} /></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Section>

      <Section title={t("overview.signals.title")} hint={t("overview.signals.hint")}>
        {signals.length === 0 ? (
          <Empty>{t("overview.signals.empty")}</Empty>
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr><th>{t("table.time")}</th><th>{t("table.severity")}</th><th>{t("table.code")}</th><th>{t("table.model")}</th><th>{t("table.auth")}</th></tr>
              </thead>
              <tbody>
                {signals.slice(0, 8).map((s, i) => (
                  <tr key={`${s.time}-${i}`}>
                    <td className="mono">{formatTime(s.time)}</td>
                    <td><span className={`severity severity-${s.severity}`}><SeverityLamp severity={s.severity} />{t(`severity.${s.severity}`)}</span></td>
                    <td className="mono">{s.code}</td>
                    <td>{s.model || s.requested_model || "—"}</td>
                    <td className="mono">{s.auth_id || "—"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Section>
    </>
  );
}

function Readout({ label, value, note, tone = "plain" }: { label: string; value: string; note: string; tone?: string }) {
  return (
    <div className={`readout readout-${tone}`}>
      <div className="readout-label">{label}</div>
      <div className="readout-value">{value}</div>
      <div className="readout-note">{note}</div>
    </div>
  );
}

function StatePill({ samples, error, degrade }: { samples: number; error: number; degrade: number }) {
  const { t } = useI18n();
  if (samples < 3) return <span className="state state-cold">{t("state.cold")}</span>;
  if (error > 0.15 || degrade > 0.2) return <span className="state state-bad">{t("state.bad")}</span>;
  if (error > 0.05 || degrade > 0.05) return <span className="state state-warn">{t("state.warn")}</span>;
  return <span className="state state-ok">{t("state.ok")}</span>;
}

function median(values: number[]): number {
  const nums = values.filter((v) => v > 0).sort((a, b) => a - b);
  if (!nums.length) return 0;
  return nums[Math.floor(nums.length / 2)];
}
