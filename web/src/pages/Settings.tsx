import { clearKey, getKey } from "../api";
import { NumberField, Section, Segmented, ToggleRow } from "../components";
import { useI18n } from "../i18n";
import type { CavemanLevel, Config, PonytailLevel, Status } from "../types";

export function SettingsPage({
  config,
  status,
  lastChange,
  update,
  onKeyCleared,
}: {
  config: Config;
  status: Status;
  lastChange?: { time: string; line: string; state: string; revision?: number };
  update: (path: (string | number)[], value: unknown) => void;
  onKeyCleared: () => void;
}) {
  const key = getKey();
  const { t } = useI18n();
  const cavemanOptions: { value: CavemanLevel; label: string }[] = [
    { value: "off", label: t("style.off") },
    { value: "lite", label: "lite" },
    { value: "full", label: "full" },
    { value: "ultra", label: "ultra" },
    { value: "wenyan-lite", label: "文言 lite" },
    { value: "wenyan", label: "文言" },
    { value: "wenyan-ultra", label: "文言 ultra" },
  ];
  const ponytailOptions: { value: PonytailLevel; label: string }[] = [
    { value: "off", label: t("style.off") },
    { value: "lite", label: "lite" },
    { value: "full", label: "full" },
    { value: "ultra", label: "ultra" },
  ];
  const download = () => {
    const blob = new Blob([JSON.stringify(config, null, 2)], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = "relaywarden-config.json";
    a.click();
    URL.revokeObjectURL(url);
  };

  return (
    <>
      <Section title={t("settings.title")} hint={t("settings.hint")}>
        <div className="settings-grid">
          <div className="setting">
            <div className="setting-label">{t("settings.key")}</div>
            <div className="setting-value mono">{key ? `${key.slice(0, 4)}…${key.slice(-4)}` : t("settings.key.unset")}</div>
            <button type="button" className="btn btn-quiet" onClick={() => { clearKey(); onKeyCleared(); }}>{t("settings.key.clear")}</button>
          </div>
          <div className="setting">
            <div className="setting-label">{t("settings.plugin")}</div>
            <div className="setting-value">v{status.version}</div>
            <div className="setting-help">{t("settings.revision", { n: status.config_revision })}</div>
          </div>
          <div className="setting">
            <div className="setting-label">{t("settings.started")}</div>
            <div className="setting-value mono">{status.started_at ? new Date(status.started_at).toLocaleString(document.documentElement.lang || "zh-CN") : "—"}</div>
          </div>
          <div className="setting">
            <div className="setting-label">{t("settings.last_change")}</div>
            <div className="setting-value mono">{lastChange ? `${lastChange.line} → ${lastChange.state}${lastChange.revision ? ` · r${lastChange.revision}` : ""}` : t("settings.last_change.none")}</div>
            {lastChange ? <div className="setting-help mono">{new Date(lastChange.time).toLocaleTimeString(document.documentElement.lang || "zh-CN", { hour12: false })}</div> : null}
          </div>
        </div>
      </Section>

      <Section title={t("style.title")} hint={t("style.hint")}>
        <div className="setting">
          <div className="setting-label">{t("style.caveman")}</div>
          <Segmented
            label={t("style.caveman")}
            value={config.caveman.level}
            options={cavemanOptions}
            onChange={(v) => update(["caveman", "level"], v)}
          />
          <div className="setting-help">{t("style.caveman.hint")}</div>
        </div>
        <div className="setting">
          <div className="setting-label">{t("style.ponytail")}</div>
          <Segmented
            label={t("style.ponytail")}
            value={config.ponytail.level}
            options={ponytailOptions}
            onChange={(v) => update(["ponytail", "level"], v)}
          />
          <div className="setting-help">{t("style.ponytail.hint")}</div>
        </div>
      </Section>

      <Section title={t("settings.interface")} hint={t("settings.interface.hint")}>
        <div className="control-row">
          <ToggleRow
            label={t("settings.advanced")}
            hint={t("settings.advanced.hint")}
            checked={config.ui.show_advanced}
            onChange={(v) => update(["ui", "show_advanced"], v)}
            tone="cobalt"
          />
          <ToggleRow
            label={t("settings.metrics")}
            hint={t("settings.metrics.hint")}
            checked={config.metrics.enabled}
            onChange={(v) => update(["metrics", "enabled"], v)}
            tone="cobalt"
          />
          <ToggleRow
            label={t("settings.ui")}
            hint={t("settings.ui.hint")}
            checked={config.ui.enabled}
            onChange={(v) => update(["ui", "enabled"], v)}
            tone="cobalt"
          />
          <NumberField label={t("settings.poll")} value={config.ui.poll_interval_ms} min={500} max={60000} step={500} onChange={(v) => update(["ui", "poll_interval_ms"], v)} />
          <NumberField label={t("settings.max_signals")} value={config.metrics.max_signals} min={10} onChange={(v) => update(["metrics", "max_signals"], v)} />
        </div>
        <div className="field">
          <span className="field-label">{t("settings.export")}</span>
          <div className="input-row">
            <button type="button" className="btn btn-primary" onClick={download}>{t("settings.download")}</button>
            <span className="field-help">{t("settings.export.help")}</span>
          </div>
        </div>
        {config.ui.show_advanced ? (
          <pre className="config-json">{JSON.stringify(config, null, 2)}</pre>
        ) : null}
      </Section>

      <Section title={t("settings.boundaries")} hint={t("settings.boundaries.hint")}>
        <ul className="boundary-list">
          <li>{t("settings.boundary.1")}</li>
          <li>{t("settings.boundary.2")}</li>
          <li>{t("settings.boundary.3")}</li>
          <li>{t("settings.boundary.4")}</li>
        </ul>
      </Section>
    </>
  );
}
