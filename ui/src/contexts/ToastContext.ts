import { createContext, useContext } from "react";
import type { AlertColor } from "@mui/material/Alert";

export interface ToastOptions {
  severity?: AlertColor;
  /** Milliseconds before auto-hide; null keeps it until dismissed. */
  duration?: number | null;
  /** Adds a "View activity" shortcut to the Activity page. */
  linkToActivity?: boolean;
}

export type ShowToast = (message: string, options?: ToastOptions) => void;

export const ToastContext = createContext<ShowToast>(() => {
  // Rendering outside the provider should not break an action; the request
  // still happens, the user just misses the confirmation.
  console.warn("useToast called outside ToastProvider");
});

/** Returns the app-wide toast function. Safe to call anywhere under the provider. */
export function useToast(): ShowToast {
  return useContext(ToastContext);
}
