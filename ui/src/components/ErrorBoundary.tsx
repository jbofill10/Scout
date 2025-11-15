import { Component } from 'react';
import type { ErrorInfo, ReactNode } from 'react';
import Box from '@mui/material/Box';
import Alert from '@mui/material/Alert';
import AlertTitle from '@mui/material/AlertTitle';
import Button from '@mui/material/Button';
import Typography from '@mui/material/Typography';
import HomeIcon from '@mui/icons-material/Home';
import { logError } from '../lib/logger';

interface ErrorBoundaryProps {
  children: ReactNode;
}

interface ErrorBoundaryState {
  hasError: boolean;
  error: Error | null;
  errorInfo: ErrorInfo | null;
}

/**
 * ErrorBoundary Component
 *
 * React Error Boundary that catches errors in child components and displays
 * a user-friendly error message with recovery options.
 *
 * Features:
 * - Catches JavaScript errors anywhere in child component tree
 * - Logs error details to console for debugging
 * - Displays Material-UI Alert with error message
 * - Provides "Go Home" button to recover from error state
 * - Resets error state when navigating away
 */
class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  constructor(props: ErrorBoundaryProps) {
    super(props);
    this.state = {
      hasError: false,
      error: null,
      errorInfo: null,
    };
  }

  static getDerivedStateFromError(error: Error): Partial<ErrorBoundaryState> {
    // Update state so the next render will show the fallback UI
    return {
      hasError: true,
      error,
    };
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo): void {
    // Log error details to console for debugging (preserved for development)
    console.error('ErrorBoundary caught an error:', error);
    console.error('Error info:', errorInfo);

    // Send error to OpenTelemetry collector
    logError('React ErrorBoundary caught an error', error, {
      componentStack: errorInfo.componentStack,
      errorBoundary: 'ErrorBoundary',
    });

    // Update state with error info
    this.setState({
      errorInfo,
    });
  }

  handleGoHome = (): void => {
    // Reset error state and navigate home
    this.setState({
      hasError: false,
      error: null,
      errorInfo: null,
    });
    window.location.href = '/';
  };

  render(): ReactNode {
    if (this.state.hasError) {
      return (
        <Box
          sx={{
            display: 'flex',
            justifyContent: 'center',
            alignItems: 'center',
            minHeight: '100vh',
            backgroundColor: 'background.default',
            p: 3,
          }}
        >
          <Box sx={{ maxWidth: 600, width: '100%' }}>
            <Alert
              severity="error"
              sx={{
                mb: 2,
                '& .MuiAlert-message': {
                  width: '100%',
                },
              }}
            >
              <AlertTitle sx={{ fontWeight: 600 }}>Something went wrong</AlertTitle>
              <Typography variant="body2" sx={{ mb: 2 }}>
                {this.state.error?.message || 'An unexpected error occurred'}
              </Typography>

              {/* Show stack trace in development */}
              {import.meta.env.DEV && this.state.errorInfo && (
                <Box
                  component="pre"
                  sx={{
                    mt: 2,
                    p: 2,
                    backgroundColor: 'rgba(0, 0, 0, 0.1)',
                    borderRadius: 1,
                    fontSize: '0.75rem',
                    overflow: 'auto',
                    maxHeight: 200,
                  }}
                >
                  {this.state.errorInfo.componentStack}
                </Box>
              )}
            </Alert>

            <Button
              variant="contained"
              color="primary"
              startIcon={<HomeIcon />}
              onClick={this.handleGoHome}
              fullWidth
              size="large"
            >
              Go Home
            </Button>
          </Box>
        </Box>
      );
    }

    return this.props.children;
  }
}

export default ErrorBoundary;
