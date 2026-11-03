import { Fragment, useMemo, useState } from "react";
import { formatTime } from "../api";
import { Empty, Section, SeverityLamp } from "../components";
import { useI18n } from "../i18n";
import type { Signal } from "../types";

const SEVERITIES = ["all", "critical", "high", "medium", "low"] as const;

export function AlertsPage({ signals }: { signals: Signal[] }) {
  const { t } = useI18n();
  const [severity, setSeverity] = useState<(typeof SEVERITIES)[number]>("all");
  const [code, setCode] = useState("");
  const [open, setOpen] = useState<string | null>(null);

  const codes = useMemo(() => {
    const set = new Set(signals.map((s) => s.code));
    return Array.from(set).sort();
  }, [signals]);

  const filtered = signals.filter((s) => {
    if (severity !== "all" && s.severity !== severity) return false;
    if (code && s.code !== code) return false;
    return true;
  });

  return (
    <>
      <Section title={t("alerts.title")} hint={t("alerts.hint")}>
        <div className="filter-bar">
          <div className="filter-pills" role="group" aria-label={t("aria.severity")}>
            {SEVERITIES.map((s) => (
              <button key={s} type="button" className={severity === s ? "is-selected" : ""} onClick={() => setSeverity(s)}>
                {t(`severity.${s}`)}
              </button>
            ))}
          </div>
          <label className="field">
            <span className="field-label">{t("alerts.code")}</span>
            <select value={code} onChange={(e) => setCode(e.target.value)}>
              <option value="">{t("alerts.all_codes")}</option>
              {codes.map((c) => <option key={c} value={c}>{c}</option>)}
            </select>
          </label>
          <span className="filter-count mono">{filtered.length} / {signals.length}</span>
        </div>

        {filtered.length === 0 ? (
          <Empty>{signals.length === 0 ? t("alerts.empty.all") : t("alerts.empty.filter")}</Empty>
        ) : (
          <div className="table-wrap">
            <table className="alert-table">
              <thead>
                <tr><th>{t("table.time")}</th><th>{t("table.severity")}</th><th>{t("table.code")}</th><th>{t("table.model")}</th><th>{t("table.auth")}</th><th>{t("table.request")}</th><th aria-label={t("aria.expand")} /></tr>
              </thead>
              <tbody>
                {filtered.map((s, i) => {
                  const id = `${s.time}-${i}`;
                  const expanded = open === id;
                  return (
                    <Fragment key={id}>
                      <tr className={expanded ? "is-expanded" : ""} onClick={() => setOpen(expanded ? null : id)}>
                        <td className="mono">{formatTime(s.time)}</td>
                        <td><span className={`severity severity-${s.severity}`}><SeverityLamp severity={s.severity} />{t(`severity.${s.severity}`)}</span></td>
                        <td className="mono">{s.code}</td>
                        <td>{s.model || s.requested_model || "—"}</td>
                        <td className="mono">{s.auth_id || "—"}</td>
                        <td className="mono dim">{s.request_id ? s.request_id.slice(0, 8) : "—"}</td>
                        <td className="expand-cell">{expanded ? "−" : "+"}</td>
                      </tr>
                      {expanded ? (
                        <tr key={`${id}-detail`} className="detail-row">
                          <td colSpan={7}>
                            <dl className="detail-list">
                              <div><dt>{t("alerts.requested")}</dt><dd className="mono">{s.requested_model || "—"}</dd></div>
                              <div><dt>{t("alerts.response")}</dt><dd className="mono">{s.response_model || "—"}</dd></div>
                              <div><dt>{t("alerts.provider")}</dt><dd className="mono">{s.provider || "—"}</dd></div>
                              <div><dt>{t("alerts.trace")}</dt><dd className="mono">{s.trace_id || "—"}</dd></div>
                            </dl>
                            {s.detail ? <pre className="detail-json">{JSON.stringify(s.detail, null, 2)}</pre> : null}
                          </td>
                        </tr>
                      ) : null}
                    </Fragment>
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
