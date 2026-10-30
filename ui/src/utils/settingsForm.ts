/**
 * Pure helpers for the Settings page: parsing what the user typed into what
 * the API expects, and explaining what is wrong before a request is sent.
 */

/** "en, ES,,fr" -> ["en", "es", "fr"], deduplicated and lower-cased. */
export function parseLanguages(input: string): string[] {
  const out: string[] = [];
  for (const raw of input.split(/[,\s]+/)) {
    const lang = raw.trim().toLowerCase();
    if (lang && !out.includes(lang)) out.push(lang);
  }
  return out;
}

/** Mirrors the torrenter's language rule: two or three letters. */
export function invalidLanguage(langs: string[]): string | null {
  for (const lang of langs) {
    if (!/^[a-z]{2,3}$/.test(lang)) return lang;
  }
  return null;
}

const DURATION_RE = /^(\d+(\.\d+)?(ns|us|µs|ms|s|m|h))+$/;

/**
 * Validates a Go duration string such as "2m", "1h30m" or "0s". Returns a
 * message to show, or null when the value is fine. allowZero is for the sync
 * interval, where 0 means "off".
 */
export function durationError(value: string, options: { allowZero?: boolean; min?: string } = {}): string | null {
  const trimmed = value.trim();
  if (!trimmed) return "Enter a duration such as 2m or 1h30m";
  if (trimmed === "0" || /^0+(s|m|h|ms)?$/.test(trimmed)) {
    return options.allowZero ? null : "Must be more than 0";
  }
  if (!DURATION_RE.test(trimmed)) return "Use a duration such as 2m or 1h30m";
  if (options.min && durationToSeconds(trimmed) < durationToSeconds(options.min)) {
    return `Must be at least ${options.min}`;
  }
  return null;
}

/** Seconds in a Go duration string; NaN when it cannot be parsed. */
export function durationToSeconds(value: string): number {
  const units: Record<string, number> = { ns: 1e-9, us: 1e-6, "µs": 1e-6, ms: 1e-3, s: 1, m: 60, h: 3600 };
  let total = 0;
  const re = /(\d+(?:\.\d+)?)(ns|us|µs|ms|s|m|h)/g;
  let matched = false;
  for (const m of value.matchAll(re)) {
    matched = true;
    total += parseFloat(m[1]) * units[m[2]];
  }
  return matched ? total : NaN;
}

/** "2m0s" -> "2m", "30m0s" -> "30m", "0s" -> "off" when zero means off. */
export function tidyDuration(value: string, zeroLabel = "0s"): string {
  if (!value) return "";
  if (durationToSeconds(value) === 0) return zeroLabel;
  return value.replace(/(\D)0s$/, "$1").replace(/^0h/, "").replace(/(\d+h)0m$/, "$1") || value;
}
