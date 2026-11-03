import type { AuthSnapshot, CompressionStats, Config, DashboardData, Signal, Status } from "./types";

const KEY = "cpa-guard-key";

export class MissingKeyError extends Error {}

export function getKey(): string {
  try {
    return sessionStorage.getItem(KEY) ?? "";
  } catch {
    return "";
  }
}

export function setKey(key: string): void {
  try {
    sessionStorage.setItem(KEY, key);
  } catch {
    // sessionStorage can be unavailable in hardened browser modes.
  }
}

export function clearKey(): void {
  try {
    sessionStorage.removeItem(KEY);
  } catch {
    // ignore
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const key = getKey();
  if (!key) throw new MissingKeyError("management key required");
  const headers = new Headers(init.headers);
  headers.set("Authorization", `Bearer ${key}`);
  if (init.body && !headers.has("Content-Type")) headers.set("Content-Type", "application/json");
  const res = await fetch(path, { ...init, headers, cache: "no-store" });
  if (res.status === 401 || res.status === 403) {
    clearKey();
    throw new MissingKeyError("management key rejected");
  }
  if (!res.ok) {
    let message = `${res.status} ${res.statusText}`;
    try {
      const body = await res.json();
      message = body?.error?.message ?? body?.message ?? message;
    } catch {
      // keep status text
    }
    throw new Error(message);
  }
  return (await res.json()) as T;
}

export async function loadDashboard(): Promise<DashboardData> {
  const [status, stats, signals, routing, config] = await Promise.all([
    request<Status>("/v0/management/cpa-guard/status"),
    request<CompressionStats>("/v0/management/cpa-guard/compression/stats"),
    request<{ signals: Signal[] }>("/v0/management/cpa-guard/signals?limit=100"),
    request<{ auths: AuthSnapshot[] }>("/v0/management/cpa-guard/routing"),
    request<Config>("/v0/management/cpa-guard/config"),
  ]);
  return { status, stats, signals: signals.signals ?? [], auths: routing.auths ?? [], config };
}

export async function patchConfig(patch: unknown): Promise<void> {
  await request("/v8/management/config/plugins/configs/cpa-guard", {
    method: "PATCH",
    body: JSON.stringify(patch),
  });
}

export async function previewCompression(text: string, format: string): Promise<{
  enabled: boolean;
  shadow: boolean;
  filter: string;
  before: number;
  after: number;
  saved: number;
  output: string;
}> {
  return request("/v0/management/cpa-guard/compression/preview", {
    method: "POST",
    body: JSON.stringify({ text, format }),
  });
}

export function formatBytes(n: number): string {
  if (!Number.isFinite(n) || n === 0) return "0 B";
  const units = ["B", "KB", "MB", "GB"];
  let v = Math.abs(n);
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${n < 0 ? "-" : ""}${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

function locale(): string {
  return document.documentElement.lang || "zh-CN";
}

export function formatNumber(n: number): string {
  return new Intl.NumberFormat(locale()).format(n);
}

export function formatTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleTimeString(locale(), { hour12: false });
}
