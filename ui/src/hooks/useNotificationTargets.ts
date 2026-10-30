import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useToast } from "../contexts/ToastContext";
import { logError } from "../lib/logger";

export type TargetKind = "webhook" | "ntfy" | "discord";
export type TargetEvent = "completed" | "failed" | "cancelled";

export const TARGET_KINDS: { value: TargetKind; label: string; hint: string }[] = [
  { value: "webhook", label: "Webhook (JSON)", hint: "Any URL that takes a JSON POST, including a Home Assistant webhook" },
  { value: "ntfy", label: "ntfy", hint: "A topic URL such as https://ntfy.sh/scout" },
  { value: "discord", label: "Discord", hint: "A channel webhook URL" },
];

export const TARGET_EVENTS: { value: TargetEvent; label: string }[] = [
  { value: "completed", label: "Downloaded" },
  { value: "failed", label: "Failed" },
  { value: "cancelled", label: "Cancelled" },
];

/** Somewhere download outcomes are sent. The token is write-only. */
export interface NotificationTarget {
  id: number;
  name: string;
  kind: TargetKind;
  url: string;
  has_token: boolean;
  events: TargetEvent[];
  enabled: boolean;
  last_sent_at?: string;
  last_error?: string;
  created_at: string;
}

export interface TargetInput {
  name: string;
  kind: TargetKind;
  url: string;
  /** Omit to keep the stored token on update; empty string clears it. */
  token?: string;
  events: TargetEvent[];
  enabled?: boolean;
}

export const TARGETS_QUERY_KEY = ["notification-targets"] as const;

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, init);
  if (!response.ok) {
    let message = response.statusText;
    try {
      const data = await response.json();
      if (data && typeof data.error === "string") message = data.error;
    } catch {
      // keep the status text
    }
    throw new Error(message);
  }
  if (response.status === 204) return undefined as T;
  return response.json();
}

const json = (method: string, body: unknown): RequestInit => ({
  method,
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify(body),
});

export function useNotificationTargets() {
  return useQuery({
    queryKey: TARGETS_QUERY_KEY,
    queryFn: () => request<NotificationTarget[]>("/api/notification-targets"),
    staleTime: 30_000,
  });
}

export function useCreateTarget() {
  const queryClient = useQueryClient();
  const showToast = useToast();
  return useMutation<NotificationTarget, Error, TargetInput>({
    mutationFn: (input) => request<NotificationTarget>("/api/notification-targets", json("POST", input)),
    retry: 0,
    onSuccess: (target) => {
      queryClient.invalidateQueries({ queryKey: TARGETS_QUERY_KEY });
      showToast(`Added ${target.name}. Send a test to check it.`, { severity: "success" });
    },
    onError: (error) => {
      logError("Create notification target failed", error, { component: "useCreateTarget" });
      showToast(error.message || "Couldn't add the target.", { severity: "error" });
    },
  });
}

export function useUpdateTarget() {
  const queryClient = useQueryClient();
  const showToast = useToast();
  return useMutation<NotificationTarget, Error, { id: number } & TargetInput>({
    mutationFn: ({ id, ...input }) => request<NotificationTarget>(`/api/notification-targets/${id}`, json("PUT", input)),
    retry: 0,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: TARGETS_QUERY_KEY }),
    onError: (error) => {
      logError("Update notification target failed", error, { component: "useUpdateTarget" });
      showToast(error.message || "Couldn't save the target.", { severity: "error" });
      queryClient.invalidateQueries({ queryKey: TARGETS_QUERY_KEY });
    },
  });
}

export function useDeleteTarget() {
  const queryClient = useQueryClient();
  const showToast = useToast();
  return useMutation<void, Error, NotificationTarget>({
    mutationFn: (target) => request<void>(`/api/notification-targets/${target.id}`, { method: "DELETE" }),
    retry: 0,
    onSuccess: (_d, target) => {
      queryClient.invalidateQueries({ queryKey: TARGETS_QUERY_KEY });
      showToast(`Removed ${target.name}`, { severity: "info" });
    },
    onError: (error) => {
      logError("Delete notification target failed", error, { component: "useDeleteTarget" });
      showToast(error.message || "Couldn't remove the target.", { severity: "error" });
    },
  });
}

export function useTestTarget() {
  const queryClient = useQueryClient();
  const showToast = useToast();
  return useMutation<{ ok: boolean }, Error, NotificationTarget>({
    mutationFn: (target) => request<{ ok: boolean }>(`/api/notification-targets/${target.id}/test`, { method: "POST" }),
    retry: 0,
    onSuccess: (_d, target) => {
      queryClient.invalidateQueries({ queryKey: TARGETS_QUERY_KEY });
      showToast(`Test sent to ${target.name}`, { severity: "success" });
    },
    onError: (error, target) => {
      logError("Test notification target failed", error, { component: "useTestTarget" });
      showToast(`${target.name}: ${error.message || "delivery failed"}`, { severity: "error" });
      queryClient.invalidateQueries({ queryKey: TARGETS_QUERY_KEY });
    },
  });
}
