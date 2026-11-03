import { useCallback, useEffect, useRef, useState } from "react";
import { MissingKeyError, clearKey, getKey, loadDashboard, patchConfig, setKey, formatTime } from "./api";
import { useI18n } from "./i18n";
import { AlertsPage } from "./pages/Alerts";
import { CompressionPage } from "./pages/Compression";
import { GuardPage } from "./pages/Guard";
import { Overview } from "./pages/Overview";
import { RoutingPage } from "./pages/Routing";
import { SettingsPage } from "./pages/Settings";
import type { DashboardData } from "./types";

type Tab = "overview" | "compression" | "guard" | "routing" | "alerts" | "settings";
type ConfigEvent = { time: string; line: string; state: string; revision?: number };

const LAST_CHANGE_KEY = "relaywarden-last-change";

function readLastChange(): ConfigEvent[] {
  try {
    const raw = localStorage.getItem(LAST_CHANGE_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed.filter((e) => e && typeof e.time === "string" && typeof e.line === "string" && typeof e.state === "string").slice(0, 20) : [];
  } catch {
    return [];
  }
}

const TABS: { id: Tab; key: string; line: string }[] = [
  { id: "overview", key: "tab.overview", line: "all" },
  { id: "compression", key: "tab.compression", line: "compression" },
  { id: "guard", key: "tab.guard", line: "guard" },
  { id: "routing", key: "tab.routing", line: "routing" },
  { id: "alerts", key: "tab.alerts", line: "alerts" },
  { id: "settings", key: "tab.settings", line: "settings" },
];

export default function App() {
  const { t, lang, setLang } = useI18n();
  const [data, setData] = useState<DashboardData | null>(null);
  const [tab, setTab] = useState<Tab>("overview");
  const [needsKey, setNeedsKey] = useState(!getKey());
  const [keyInput, setKeyInput] = useState("");
  const [error, setError] = useState("");
  const [applying, setApplying] = useState(false);
  const [configEvents, setConfigEvents] = useState<ConfigEvent[]>(readLastChange);
  const timer = useRef<number | null>(null);
  const alive = useRef(true);

  const refresh = useCallback(async () => {
    try {
      const next = await loadDashboard();
      if (!alive.current) return;
      setData(next);
      setNeedsKey(false);
      setError("");
    } catch (err) {
      if (!alive.current) return;
      if (err instanceof MissingKeyError) {
        setNeedsKey(true);
      } else {
        setError(err instanceof Error ? err.message : String(err));
      }
    }
  }, []);

  useEffect(() => {
    alive.current = true;
    refresh();
    return () => {
      alive.current = false;
      if (timer.current) window.clearInterval(timer.current);
    };
  }, [refresh]);

  useEffect(() => {
    if (!data || needsKey) return;
    const interval = Math.max(500, data.config.ui.poll_interval_ms || 2000);
    if (timer.current) window.clearInterval(timer.current);
    timer.current = window.setInterval(() => {
      if (document.visibilityState === "visible") refresh();
    }, interval);
    return () => {
      if (timer.current) window.clearInterval(timer.current);
    };
  }, [data?.config.ui.poll_interval_ms, needsKey, refresh]);

  const update = useCallback(
    async (path: (string | number)[], value: unknown) => {
      if (!data) return;
      const previous = data;
      setData({ ...data, config: deepSet(data.config, path, value) as DashboardData["config"] });
      setApplying(true);
      setError("");
      try {
        await patchConfig(buildPatch(path, value));
        const state = typeof value === "boolean" ? (value ? t("common.on") : t("common.off")) : String(value);
        setConfigEvents((prev) => {
          const next = [{ time: new Date().toISOString(), line: configLine(path, t), state, revision: data.status.config_revision + 1 }, ...prev].slice(0, 20);
          try {
            localStorage.setItem(LAST_CHANGE_KEY, JSON.stringify(next));
          } catch {
            // localStorage may be unavailable; the in-memory tape still works.
          }
          return next;
        });
        window.setTimeout(refresh, 250);
        window.setTimeout(refresh, 1200);
      } catch (err) {
        setData(previous);
        setError(err instanceof Error ? err.message : String(err));
      } finally {
        window.setTimeout(() => setApplying(false), 800);
      }
    },
    [data, refresh, t],
  );

  if (needsKey) {
    return (
      <KeyGate
        value={keyInput}
        onChange={setKeyInput}
        onSubmit={() => {
          setKey(keyInput.trim());
          setKeyInput("");
          setNeedsKey(false);
          refresh();
        }}
      />
    );
  }

  if (!data) {
    return (
      <div className="boot">
        <div className="boot-mark">RelayWarden</div>
        <div className="boot-line">{t("boot.connecting")}</div>
        {error ? <div className="inline-error">{error}</div> : null}
      </div>
    );
  }

  const { config, status, signals, auths, stats } = data;
  const alertsOn = config.guard.alerts.log || config.guard.alerts.webhook.enabled;
  const tapeItems = [
    ...configEvents.map((event) => ({ kind: "config" as const, time: event.time, event })),
    ...signals.map((signal) => ({ kind: "signal" as const, time: signal.time, signal })),
  ]
    .sort((a, b) => Date.parse(b.time) - Date.parse(a.time))
    .slice(0, 4);

  return (
    <div className="shell">
      <header className="topbar">
        <div className="brand">
          <span className="brand-mark" aria-hidden="true" />
          <span className="brand-name">RelayWarden</span>
          <span className="brand-sub">{t("brand.sub")}</span>
        </div>
        <FlowStrip
          compression={config.compression.enabled}
          shadow={config.compression.shadow}
          guard={config.guard.enabled}
          routing={config.routing.enabled}
          alerts={alertsOn}
        />
        <div className="topbar-meta">
          <span className="live-dot" aria-hidden="true" />
          <span className="mono">{t("top.live")}</span>
          <span className="mono dim">r{status.config_revision}</span>
          <div className="lang-switch" role="group" aria-label={t("common.language")}>
            <button type="button" className={lang === "zh" ? "is-active" : ""} onClick={() => setLang("zh")}>中</button>
            <button type="button" className={lang === "en" ? "is-active" : ""} onClick={() => setLang("en")}>EN</button>
          </div>
          {applying ? <span className="applying">{t("top.applying")}</span> : null}
        </div>
      </header>

      {error ? (
        <div className="error-banner" role="alert">
          <span>{error}</span>
          <button type="button" onClick={() => setError("")}>{t("top.dismiss")}</button>
        </div>
      ) : null}

      <div className="body">
        <LineRail
          config={config}
          version={status.version}
          active={tab}
          onSelect={setTab}
        />

        <main className="main">
          <nav className="tabs" aria-label={t("aria.sections")}>
            {TABS.map((item) => (
              <button key={item.id} type="button" className={tab === item.id ? "is-active" : ""} onClick={() => setTab(item.id)}>
                {t(item.key)}
              </button>
            ))}
          </nav>

          <div className="page">
            {tab === "overview" ? <Overview data={data} /> : null}
            {tab === "compression" ? <CompressionPage config={config} update={update} /> : null}
            {tab === "guard" ? <GuardPage config={config} update={update} /> : null}
            {tab === "routing" ? <RoutingPage config={config} update={update} auths={auths} /> : null}
            {tab === "alerts" ? <AlertsPage signals={signals} /> : null}
            {tab === "settings" ? (
              <SettingsPage
                config={config}
                status={status}
                lastChange={configEvents[0]}
                update={update}
                onKeyCleared={() => {
                  clearKey();
                  setNeedsKey(true);
                  setData(null);
                }}
              />
            ) : null}
          </div>
        </main>
      </div>

      <footer className="event-tape" aria-label={t("aria.events")}>
        <span className="tape-label">{t("tape.label")}</span>
        <div className="tape-items">
          {tapeItems.length === 0 ? (
            <span className="tape-empty">{t("tape.empty")}</span>
          ) : (
            tapeItems.map((item, i) =>
              item.kind === "config" ? (
                <span key={`cfg-${item.event.time}-${i}`} className="tape-item tape-config">
                  <span className="mono">{formatTime(item.event.time)}</span>
                  <span className="mono">{t("tape.config", { line: item.event.line, state: item.event.state })}{item.event.revision ? ` r${item.event.revision}` : ""}</span>
                </span>
              ) : (
                <span key={`${item.signal.time}-${i}`} className={`tape-item tape-${item.signal.severity}`}>
                  <span className="mono">{formatTime(item.signal.time)}</span>
                  <span className="mono">{item.signal.code}</span>
                  <span>{item.signal.model || item.signal.requested_model || "—"}</span>
                </span>
              ),
            )
          )}
        </div>
        <span className="tape-stat mono">{t("tape.blocks", { n: stats.total.blocks })}</span>
      </footer>
    </div>
  );
}

function KeyGate({ value, onChange, onSubmit }: { value: string; onChange: (v: string) => void; onSubmit: () => void }) {
  const { t, lang, setLang } = useI18n();
  return (
    <div className="key-gate">
      <div className="key-lang">
        <button type="button" className={lang === "zh" ? "is-active" : ""} onClick={() => setLang("zh")}>中</button>
        <button type="button" className={lang === "en" ? "is-active" : ""} onClick={() => setLang("en")}>EN</button>
      </div>
      <form
        className="key-card"
        onSubmit={(e) => {
          e.preventDefault();
          if (value.trim()) onSubmit();
        }}
      >
        <div className="brand-mark large" aria-hidden="true" />
        <h1>{t("key.title")}</h1>
        <p>{t("key.desc")}</p>
        <label className="key-field">
          <span>{t("key.label")}</span>
          <input autoFocus type="password" value={value} onChange={(e) => onChange(e.target.value)} placeholder="Bearer key" />
        </label>
        <button type="submit" className="btn btn-primary" disabled={!value.trim()}>{t("key.connect")}</button>
        <div className="key-help">{t("key.help")}</div>
      </form>
    </div>
  );
}

function LineRail({
  config,
  version,
  active,
  onSelect,
}: {
  config: DashboardData["config"];
  version: string;
  active: Tab;
  onSelect: (tab: Tab) => void;
}) {
  const { t } = useI18n();
  const alertsOn = config.guard.alerts.log || config.guard.alerts.webhook.enabled;
  const lines = [
    { id: "compression" as const, label: t("rail.compression"), tone: "green", checked: config.compression.enabled },
    { id: "guard" as const, label: t("rail.guard"), tone: "amber", checked: config.guard.enabled },
    { id: "routing" as const, label: t("rail.routing"), tone: "cobalt", checked: config.routing.enabled },
    { id: "alerts" as const, label: t("rail.alerts"), tone: "red", checked: alertsOn },
  ];
  return (
    <aside className="line-rail" aria-label={t("rail.title")}>
      <div className="rail-title">{t("rail.title")}</div>
      {lines.map((l) => (
        <div key={l.id} className={`rail-line rail-${l.tone} ${active === l.id ? "is-active" : ""}`}>
          <button type="button" className="rail-select" onClick={() => onSelect(l.id)} aria-label={l.label}>
            <span className="rail-swatch" aria-hidden="true" />
            <span className="rail-name">{l.label}</span>
            <span className={`rail-state rail-state-${l.tone} ${l.checked ? "is-on" : ""}`}>
              {l.checked ? t("common.on") : t("common.off")}
            </span>
          </button>
        </div>
      ))}
      <div className="rail-foot mono">{version ? `v${version}` : ""}</div>
    </aside>
  );
}

function FlowStrip({ compression, shadow, guard, routing, alerts }: { compression: boolean; shadow: boolean; guard: boolean; routing: boolean; alerts: boolean }) {
  const { t } = useI18n();
  return (
    <div className="flow" aria-label={t("aria.flow")}>
      <div className="flow-node">
        <span className="flow-name">{t("flow.client")}</span>
        <span className="flow-sub">{t("flow.client.sub")}</span>
      </div>
      <div className="flow-lines">
        <span className={`flow-line line-compression ${compression ? (shadow ? "is-shadow" : "is-on") : "is-off"}`} />
        <span className={`flow-line line-guard ${guard ? "is-on" : "is-off"}`} />
        <span className={`flow-line line-routing ${routing ? "is-on" : "is-off"}`} />
        <span className={`flow-line line-alerts ${alerts ? "is-on" : "is-off"}`} />
      </div>
      <div className="flow-node center">
        <span className="flow-name">{t("flow.cpa")}</span>
        <span className="flow-sub">{t("flow.cpa.sub")}</span>
      </div>
      <div className="flow-lines">
        <span className={`flow-line line-compression ${compression ? (shadow ? "is-shadow" : "is-on") : "is-off"}`} />
        <span className={`flow-line line-guard ${guard ? "is-on" : "is-off"}`} />
        <span className={`flow-line line-routing ${routing ? "is-on" : "is-off"}`} />
        <span className={`flow-line line-alerts ${alerts ? "is-on" : "is-off"}`} />
      </div>
      <div className="flow-node">
        <span className="flow-name">{t("flow.upstream")}</span>
        <span className="flow-sub">{t("flow.upstream.sub")}</span>
      </div>
    </div>
  );
}

function configLine(path: (string | number)[], t: (key: string) => string): string {
  const root = String(path[0]);
  let label = root;
  if (root === "compression") label = t("rail.compression");
  else if (root === "guard") label = path[1] === "alerts" ? t("rail.alerts") : t("rail.guard");
  else if (root === "routing") label = t("rail.routing");
  else if (root === "metrics" || root === "ui") label = t("tab.settings");
  const leaf = path.slice(1).join(".");
  return leaf ? `${label} · ${leaf}` : label;
}

function buildPatch(path: (string | number)[], value: unknown): unknown {  let out: unknown = value;
  for (let i = path.length - 1; i >= 0; i--) {
    out = { [String(path[i])]: out };
  }
  return out;
}

function deepSet<T>(obj: T, path: (string | number)[], value: unknown): T {
  if (path.length === 0) return value as T;
  const [head, ...rest] = path;
  const source = obj as Record<string, unknown>;
  const key = String(head);
  const child = source[key];
  const next = deepSet(child ?? {}, rest, value);
  return { ...source, [key]: next } as T;
}
