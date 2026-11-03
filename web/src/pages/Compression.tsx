import { useState } from "react";
import { previewCompression } from "../api";
import { NumberField, Section, Segmented, ToggleRow } from "../components";
import { useI18n } from "../i18n";
import type { Config, FilterToggles } from "../types";

const FILTERS: { key: keyof FilterToggles; label: string; hint: string }[] = [
  { key: "git_diff", label: "filter.git_diff", hint: "filter.git_diff.hint" },
  { key: "git_status", label: "filter.git_status", hint: "filter.git_status.hint" },
  { key: "git_log", label: "filter.git_log", hint: "filter.git_log.hint" },
  { key: "grep", label: "filter.grep", hint: "filter.grep.hint" },
  { key: "find", label: "filter.find", hint: "filter.find.hint" },
  { key: "ls", label: "filter.ls", hint: "filter.ls.hint" },
  { key: "tree", label: "filter.tree", hint: "filter.tree.hint" },
  { key: "build_output", label: "filter.build_output", hint: "filter.build_output.hint" },
  { key: "test_output", label: "filter.test_output", hint: "filter.test_output.hint" },
  { key: "dedup_log", label: "filter.dedup_log", hint: "filter.dedup_log.hint" },
  { key: "smart_truncate", label: "filter.smart_truncate", hint: "filter.smart_truncate.hint" },
  { key: "read_numbered", label: "filter.read_numbered", hint: "filter.read_numbered.hint" },
  { key: "json_minify", label: "filter.json_minify", hint: "filter.json_minify.hint" },
  { key: "strip_ansi", label: "filter.strip_ansi", hint: "filter.strip_ansi.hint" },
];

export function CompressionPage({
  config,
  update,
}: {
  config: Config;
  update: (path: (string | number)[], value: unknown) => void;
}) {
  const c = config.compression;
  const { t } = useI18n();
  const [text, setText] = useState("");
  const [format, setFormat] = useState("openai-response");
  const [preview, setPreview] = useState<{ before: number; after: number; filter: string; output: string; shadow: boolean } | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const runPreview = async () => {
    setBusy(true);
    setError("");
    try {
      setPreview(await previewCompression(text, format));
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <Section title={t("comp.title")} hint={t("comp.hint")}>
        <div className="control-row">
          <ToggleRow
            label={t("comp.enabled")}
            hint={t("comp.enabled.hint")}
            checked={c.enabled}
            onChange={(v) => update(["compression", "enabled"], v)}
            danger={t("comp.enabled.danger")}
            tone="green"
          />
          <ToggleRow
            label={t("comp.shadow")}
            hint={t("comp.shadow.hint")}
            checked={c.shadow}
            onChange={(v) => update(["compression", "shadow"], v)}
            tone="amber"
          />
          <ToggleRow
            label={t("comp.tail")}
            hint={t("comp.tail.hint")}
            checked={c.tail_only}
            onChange={(v) => update(["compression", "tail_only"], v)}
            tone="green"
          />
        </div>

        <div className="control-row control-row-inline">
          <div className="field">
            <span className="field-label">{t("comp.mode")}</span>
            <Segmented
              label={t("comp.mode")}
              value={c.mode}
              options={[
                { value: "safe", label: t("comp.mode.safe") },
                { value: "balanced", label: t("comp.mode.balanced") },
                { value: "aggressive", label: t("comp.mode.aggressive") },
              ]}
              onChange={(v) => update(["compression", "mode"], v)}
            />
          </div>
          <div className="field">
            <span className="field-label">{t("comp.protocols")}</span>
            <div className="switch-line">
              <ToggleRow label="OpenAI Chat" checked={c.protocols.openai_chat} onChange={(v) => update(["compression", "protocols", "openai_chat"], v)} />
              <ToggleRow label="Responses / Codex" checked={c.protocols.openai_responses} onChange={(v) => update(["compression", "protocols", "openai_responses"], v)} />
              <ToggleRow label="Gemini" checked={c.protocols.gemini} onChange={(v) => update(["compression", "protocols", "gemini"], v)} />
            </div>
          </div>
        </div>

        <div className="field-grid">
          <NumberField label={t("comp.min_bytes")} value={c.min_bytes} min={0} onChange={(v) => update(["compression", "min_bytes"], v)} />
          <NumberField label={t("comp.max_bytes")} value={c.max_bytes} min={0} onChange={(v) => update(["compression", "max_bytes"], v)} />
          <NumberField label={t("comp.max_chars")} value={c.max_chars_per_result} min={256} onChange={(v) => update(["compression", "max_chars_per_result"], v)} />
          <NumberField label={t("comp.max_lines")} value={c.max_lines_per_result} min={10} onChange={(v) => update(["compression", "max_lines_per_result"], v)} />
          <NumberField label={t("comp.dedup")} value={c.dedup_threshold} min={2} onChange={(v) => update(["compression", "dedup_threshold"], v)} />
          <NumberField label={t("comp.tail_items")} value={c.tail_items} min={1} onChange={(v) => update(["compression", "tail_items"], v)} />
        </div>
      </Section>

      <Section title={t("comp.filters.title")} hint={t("comp.filters.hint")}>
        <div className="filter-grid">
          {FILTERS.map((f) => (
            <ToggleRow
              key={f.key}
              label={t(f.label)}
              hint={t(f.hint)}
              checked={c.filters[f.key]}
              onChange={(v) => update(["compression", "filters", f.key], v)}
              tone="green"
            />
          ))}
        </div>
      </Section>

      <Section title={t("comp.preview.title")} hint={t("comp.preview.hint")}>
        <div className="preview-grid">
          <label className="preview-pane">
            <span>{t("comp.preview.original")}</span>
            <textarea value={text} onChange={(e) => setText(e.target.value)} placeholder={t("comp.preview.placeholder")} />
          </label>
          <label className="preview-pane">
            <span>{t("comp.preview.compressed")}</span>
            <textarea readOnly value={preview?.output ?? ""} placeholder={t("comp.preview.empty")} />
          </label>
        </div>
        <div className="preview-actions">
          <label className="field">
            <span className="field-label">{t("comp.preview.format")}</span>
            <select value={format} onChange={(e) => setFormat(e.target.value)}>
              <option value="openai-response">OpenAI Responses / Codex</option>
              <option value="openai">OpenAI Chat</option>
              <option value="gemini">Gemini</option>
            </select>
          </label>
          <button type="button" className="btn btn-primary" onClick={runPreview} disabled={busy || !text}>
            {busy ? t("comp.preview.running") : t("comp.preview.run")}
          </button>
          {preview ? (
            <span className="preview-stat mono">
              {preview.before} → {preview.after} bytes · {preview.filter || t("comp.preview.no_filter")}
              {preview.shadow ? ` · ${t("comp.preview.shadow")}` : ""}
            </span>
          ) : null}
        </div>
        {error ? <div className="inline-error">{error}</div> : null}
      </Section>
    </>
  );
}
