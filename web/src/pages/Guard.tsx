import { useState } from "react";
import { NumberField, Section, ToggleRow } from "../components";
import { useI18n } from "../i18n";
import type { Config, SignalToggles } from "../types";

const CHECKS: { key: keyof Config["guard"]; label: string; hint: string }[] = [
  { key: "check_model", label: "check.check_model", hint: "check.check_model.hint" },
  { key: "check_reasoning", label: "check.check_reasoning", hint: "check.check_reasoning.hint" },
  { key: "check_cache", label: "check.check_cache", hint: "check.check_cache.hint" },
  { key: "check_tools", label: "check.check_tools", hint: "check.check_tools.hint" },
  { key: "check_params", label: "check.check_params", hint: "check.check_params.hint" },
  { key: "check_fingerprint", label: "check.check_fingerprint", hint: "check.check_fingerprint.hint" },
  { key: "check_usage", label: "check.check_usage", hint: "check.check_usage.hint" },
  { key: "check_cache_reason", label: "check.check_cache_reason", hint: "check.check_cache_reason.hint" },
  { key: "check_truncation", label: "check.check_truncation", hint: "check.check_truncation.hint" },
  { key: "check_empty", label: "check.check_empty", hint: "check.check_empty.hint" },
  { key: "check_ttft", label: "check.check_ttft", hint: "check.check_ttft.hint" },
];

const SIGNALS: { key: keyof SignalToggles; label: string }[] = [
  { key: "model_mismatch", label: "model_mismatch" },
  { key: "reasoning_stripped", label: "reasoning_stripped" },
  { key: "source_param_dropped", label: "source_param_dropped" },
  { key: "cache_miss_streak", label: "cache_miss_streak" },
  { key: "service_tier_downgraded", label: "service_tier_downgraded" },
  { key: "system_fingerprint_changed", label: "system_fingerprint_changed" },
  { key: "cache_miss_reason", label: "cache_miss_reason" },
  { key: "usage_invariant_violation", label: "usage_invariant_violation" },
  { key: "tools_changed", label: "tools_changed" },
  { key: "system_prompt_changed", label: "system_prompt_changed" },
  { key: "empty_output", label: "empty_output" },
  { key: "truncated", label: "truncated" },
  { key: "usage_missing", label: "usage_missing" },
  { key: "ttft_anomaly", label: "ttft_anomaly" },
];

export function GuardPage({
  config,
  update,
}: {
  config: Config;
  update: (path: (string | number)[], value: unknown) => void;
}) {
  const g = config.guard;
  const { t } = useI18n();
  const [webhook, setWebhook] = useState(g.alerts.webhook.url);
  const [webhookSaved, setWebhookSaved] = useState(true);

  return (
    <>
      <Section title={t("guard.title")} hint={t("guard.hint")}>
        <div className="control-row">
          <ToggleRow
            label={t("guard.enabled")}
            hint={t("guard.enabled.hint")}
            checked={g.enabled}
            onChange={(v) => update(["guard", "enabled"], v)}
            danger={t("guard.enabled.danger")}
            tone="amber"
          />
          <ToggleRow
            label={t("guard.reinject")}
            hint={t("guard.reinject.hint")}
            checked={g.reinject_source_params}
            onChange={(v) => update(["guard", "reinject_source_params"], v)}
            tone="amber"
          />
          <ToggleRow
            label={t("guard.inject_key")}
            hint={t("guard.inject_key.hint")}
            checked={g.inject_prompt_cache_key}
            onChange={(v) => update(["guard", "inject_prompt_cache_key"], v)}
            tone="amber"
          />
          <div className="locked-row">
            <div>
              <div className="toggle-label">{t("guard.observe_only")}</div>
              <div className="toggle-hint">{t("guard.observe_only.hint")}</div>
            </div>
            <span className="locked-pill">{t("guard.locked")}</span>
          </div>
        </div>

        <div className="field-grid">
          <NumberField label={t("guard.cache_threshold")} value={g.cache_miss_threshold} min={2} onChange={(v) => update(["guard", "cache_miss_threshold"], v)} />
          <NumberField label={t("guard.prompt_threshold")} value={g.prompt_token_threshold} min={256} onChange={(v) => update(["guard", "prompt_token_threshold"], v)} />
          <NumberField label={t("guard.cooldown")} value={g.alerts.cooldown_seconds} min={0} onChange={(v) => update(["guard", "alerts", "cooldown_seconds"], v)} />
        </div>
      </Section>

      <Section title={t("guard.checks.title")} hint={t("guard.checks.hint")}>
        <div className="filter-grid">
          {CHECKS.map((c) => (
            <ToggleRow
              key={c.key}
              label={t(c.label)}
              hint={t(c.hint)}
              checked={Boolean(g[c.key])}
              onChange={(v) => update(["guard", c.key as string], v)}
              tone="amber"
            />
          ))}
        </div>
      </Section>

      <Section title={t("guard.signals.title")} hint={t("guard.signals.hint")}>
        <div className="signal-grid">
          {SIGNALS.map((s) => (
            <ToggleRow
              key={s.key}
              label={s.label}
              checked={g.signals[s.key]}
              onChange={(v) => update(["guard", "signals", s.key], v)}
              tone="red"
            />
          ))}
        </div>
      </Section>

      <Section title={t("guard.alerts.title")} hint={t("guard.alerts.hint")}>
        <div className="control-row">
          <ToggleRow label={t("guard.alerts.log")} hint={t("guard.alerts.log.hint")} checked={g.alerts.log} onChange={(v) => update(["guard", "alerts", "log"], v)} tone="red" />
          <ToggleRow label={t("guard.alerts.webhook")} hint={t("guard.alerts.webhook.hint")} checked={g.alerts.webhook.enabled} onChange={(v) => update(["guard", "alerts", "webhook", "enabled"], v)} tone="red" />
        </div>
        <div className="field webhook-field">
          <span className="field-label">{t("guard.alerts.url")}</span>
          <div className="input-row">
            <input
              type="url"
              value={webhook}
              placeholder="https://example.com/cpa-guard"
              onChange={(e) => {
                setWebhook(e.target.value);
                setWebhookSaved(false);
              }}
            />
            <button
              type="button"
              className="btn btn-primary"
              disabled={webhookSaved}
              onClick={() => {
                update(["guard", "alerts", "webhook", "url"], webhook);
                setWebhookSaved(true);
              }}
            >
              {t("guard.alerts.save_url")}
            </button>
          </div>
          <span className="field-help">{t("guard.alerts.url_help")}</span>
        </div>
        <div className="field">
          <span className="field-label">{t("guard.alerts.min_severity")}</span>
          <select value={g.alerts.min_severity} onChange={(e) => update(["guard", "alerts", "min_severity"], e.target.value)}>
            <option value="low">low</option>
            <option value="medium">medium</option>
            <option value="high">high</option>
            <option value="critical">critical</option>
          </select>
        </div>
      </Section>
    </>
  );
}
