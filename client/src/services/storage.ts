import type { SkillSnippet, StoredValue, ThirdPartyApiConfig } from 'src/types/shellai';

const KEY_API_KEY = 'shellai.apiKey';
const KEY_API_BASE = 'shellai.apiBase';
const KEY_API_CONFIGS = 'shellai.thirdPartyApis';
const KEY_SKILLS = 'shellai.skills';

function now(): number {
  return Date.now();
}

function parseJSON<T>(raw: string | null): T | null {
  if (!raw) {
    return null;
  }
  try {
    return JSON.parse(raw) as T;
  } catch {
    return null;
  }
}

export function loadApiBase(): string {
  return localStorage.getItem(KEY_API_BASE) ?? 'http://127.0.0.1:8080';
}

export function saveApiBase(baseUrl: string): void {
  localStorage.setItem(KEY_API_BASE, baseUrl.trim());
}

export function saveApiKey(apiKey: string, ttlMs?: number): void {
  const payload: StoredValue<string> = {
    value: apiKey.trim(),
  };
  if (ttlMs && ttlMs > 0) {
    payload.expiresAt = now() + ttlMs;
  }
  localStorage.setItem(KEY_API_KEY, JSON.stringify(payload));
}

export function loadApiKey(): string {
  const payload = parseJSON<StoredValue<string>>(localStorage.getItem(KEY_API_KEY));
  if (!payload?.value) {
    return '';
  }
  if (payload.expiresAt && payload.expiresAt <= now()) {
    localStorage.removeItem(KEY_API_KEY);
    return '';
  }
  return payload.value;
}

export function clearApiKey(): void {
  localStorage.removeItem(KEY_API_KEY);
}

export function loadThirdPartyApis(): ThirdPartyApiConfig[] {
  const stored = parseJSON<ThirdPartyApiConfig[]>(localStorage.getItem(KEY_API_CONFIGS));
  if (!stored) {
    return [];
  }
  return stored;
}

export function saveThirdPartyApis(configs: ThirdPartyApiConfig[]): void {
  localStorage.setItem(KEY_API_CONFIGS, JSON.stringify(configs));
}

export function loadSkills(): SkillSnippet[] {
  const stored = parseJSON<SkillSnippet[]>(localStorage.getItem(KEY_SKILLS));
  if (!stored) {
    return [];
  }
  return stored;
}

export function saveSkills(skills: SkillSnippet[]): void {
  localStorage.setItem(KEY_SKILLS, JSON.stringify(skills));
}
