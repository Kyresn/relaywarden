import { useState, type ReactNode } from "react";
import { useI18n } from "./i18n";

export function Switch({
  checked,
  onChange,
  disabled,
  label,
  tone = "green",
}: {
  checked: boolean;
  onChange: (next: boolean) => void;
  disabled?: boolean;
  label: string;
  tone?: "green" | "amber" | "cobalt" | "red";
}) {
  const { t } = useI18n();
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      disabled={disabled}
      className={`switch switch-${tone} ${checked ? "is-on" : ""}`}
      onClick={() => onChange(!checked)}
    >
      <span className="switch-track">
        <span className="switch-thumb" />
      </span>
      <span className="switch-state">{checked ? t("common.on") : t("common.off")}</span>
    </button>
  );
}

export function ToggleRow({
  label,
  hint,
  checked,
  onChange,
  danger,
  disabled,
  tone = "green",
}: {
  label: string;
  hint?: string;
  checked: boolean;
  onChange: (next: boolean) => void;
  danger?: string;
  disabled?: boolean;
  tone?: "green" | "amber" | "cobalt" | "red";
}) {
  const [confirming, setConfirming] = useState(false);
  const { t } = useI18n();
  const request = (next: boolean) => {
    if (!next && danger) {
      setConfirming(true);
      return;
    }
    onChange(next);
  };
  return (
    <div className="toggle-row">
      <div className="toggle-copy">
        <div className="toggle-label">{label}</div>
        {hint ? <div className="toggle-hint">{hint}</div> : null}
      </div>
      <Switch checked={checked} onChange={request} disabled={disabled} label={label} tone={tone} />
      {confirming ? (
        <div className="confirm-row" role="alert">
          <span>{danger}</span>
          <button
            type="button"
            className="btn btn-quiet"
            onClick={() => setConfirming(false)}
          >
            {t("common.cancel")}
          </button>
          <button
            type="button"
            className="btn btn-danger"
            onClick={() => {
              setConfirming(false);
              onChange(false);
            }}
          >
            {t("common.confirm_off")}
          </button>
        </div>
      ) : null}
    </div>
  );
}

export function Segmented<T extends string>({
  value,
  options,
  onChange,
  label,
}: {
  value: T;
  options: { value: T; label: string; hint?: string }[];
  onChange: (next: T) => void;
  label: string;
}) {
  return (
    <div className="segmented" role="radiogroup" aria-label={label}>
      {options.map((opt) => (
        <button
          key={opt.value}
          type="button"
          role="radio"
          aria-checked={value === opt.value}
          className={value === opt.value ? "is-selected" : ""}
          onClick={() => onChange(opt.value)}
          title={opt.hint}
        >
          {opt.label}
        </button>
      ))}
    </div>
  );
}

export function NumberField({
  label,
  value,
  min,
  max,
  step = 1,
  onChange,
}: {
  label: string;
  value: number;
  min?: number;
  max?: number;
  step?: number;
  onChange: (next: number) => void;
}) {
  return (
    <label className="number-field">
      <span>{label}</span>
      <input
        type="number"
        value={Number.isFinite(value) ? value : 0}
        min={min}
        max={max}
        step={step}
        onChange={(e) => {
          const n = Number(e.target.value);
          if (Number.isFinite(n)) onChange(n);
        }}
      />
    </label>
  );
}

export function Section({
  title,
  hint,
  children,
  actions,
}: {
  title: string;
  hint?: string;
  children: ReactNode;
  actions?: ReactNode;
}) {
  return (
    <section className="section">
      <header className="section-head">
        <div>
          <h2>{title}</h2>
          {hint ? <p>{hint}</p> : null}
        </div>
        {actions ? <div className="section-actions">{actions}</div> : null}
      </header>
      {children}
    </section>
  );
}

export function SeverityLamp({ severity }: { severity: string }) {
  const { t } = useI18n();
  return <span className={`lamp lamp-${severity}`} aria-label={t(`severity.${severity}`)} />;
}

export function Empty({ children }: { children: ReactNode }) {
  return <div className="empty">{children}</div>;
}
