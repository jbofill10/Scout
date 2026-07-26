import React, { useCallback, useMemo, useState } from "react";
import Snackbar from "@mui/material/Snackbar";
import Alert from "@mui/material/Alert";
import type { AlertColor } from "@mui/material/Alert";
import Button from "@mui/material/Button";
import { useNavigate } from "react-router-dom";
import { ToastContext, type ShowToast } from "../../contexts/ToastContext";

interface ToastState {
  key: number;
  message: string;
  severity: AlertColor;
  duration: number | null;
  linkToActivity: boolean;
}

/**
 * ToastProvider
 *
 * App-wide confirmation banner. Every action that fires a request whose result
 * the user cannot otherwise see (download requests, most notably) reports
 * through here, so no button click is silent.
 */
const ToastProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [toast, setToast] = useState<ToastState | null>(null);
  const navigate = useNavigate();

  const showToast = useCallback<ShowToast>((message, options) => {
    setToast({
      key: Date.now(),
      message,
      severity: options?.severity ?? "success",
      duration: options?.duration === undefined ? 5000 : options.duration,
      linkToActivity: options?.linkToActivity ?? false,
    });
  }, []);

  const handleClose = useCallback(() => setToast(null), []);

  const value = useMemo(() => showToast, [showToast]);

  return (
    <ToastContext.Provider value={value}>
      {children}
      <Snackbar
        key={toast?.key}
        open={toast !== null}
        autoHideDuration={toast?.duration ?? null}
        onClose={handleClose}
        anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
      >
        <Alert
          onClose={handleClose}
          severity={toast?.severity ?? "success"}
          variant="filled"
          sx={{ width: "100%", alignItems: "center" }}
          action={
            toast?.linkToActivity ? (
              <Button
                color="inherit"
                size="small"
                onClick={() => {
                  handleClose();
                  navigate("/activity");
                }}
              >
                View activity
              </Button>
            ) : undefined
          }
        >
          {toast?.message}
        </Alert>
      </Snackbar>
    </ToastContext.Provider>
  );
};

export default ToastProvider;
