import { useState, useEffect, useRef } from "react";
import {
  IconButton,
  Badge,
  Backdrop,
  Box,
  Typography,
  Card,
  CardContent,
  CardMedia,
  Collapse,
  List,
  ListItem,
  ListItemText,
  Button,
  Skeleton,
  Slide,
  Chip,
  Tooltip,
  alpha,
  useTheme,
} from "@mui/material";
import {
  Notifications as NotificationsIcon,
  Close as CloseIcon,
  ExpandMore as ExpandMoreIcon,
  CheckCircle as CompletedIcon,
  Download as DownloadingIcon,
  Search as SearchingIcon,
  Schedule as ScheduledIcon,
  Error as FailedIcon,
  Clear as DismissIcon,
  NotificationsNoneOutlined as NoNotificationsIcon,
} from "@mui/icons-material";
import {
  useUnreadCount,
  useGroupedNotifications,
  useMarkAsRead,
  useDismissNotification,
  type NotificationGroup,
} from "../../hooks/useNotifications";

// ==================== Helper Functions ====================

function getStatusIcon(status: string | undefined) {
  if (!status) return <SearchingIcon sx={{ fontSize: 18 }} />;
  switch (status) {
    case "completed":
      return <CompletedIcon sx={{ fontSize: 18 }} />;
    case "downloading":
      return <DownloadingIcon sx={{ fontSize: 18 }} />;
    case "searching":
      return <SearchingIcon sx={{ fontSize: 18 }} />;
    case "scheduled":
      return <ScheduledIcon sx={{ fontSize: 18 }} />;
    case "failed":
      return <FailedIcon sx={{ fontSize: 18 }} />;
    default:
      return <SearchingIcon sx={{ fontSize: 18 }} />;
  }
}

function getStatusColor(status: string | undefined) {
  if (!status) return "#9CA3AF"; // gray
  switch (status) {
    case "completed":
      return "#10B981"; // green
    case "downloading":
      return "#8B5CF6"; // purple
    case "searching":
      return "#FFA500"; // yellow/orange
    case "scheduled":
      return "#4F46E5"; // indigo/blue
    case "failed":
      return "#EF4444"; // red
    default:
      return "#9CA3AF"; // gray
  }
}

function getStatusLabel(status: string | undefined) {
  if (!status) return "Unknown";
  return status.charAt(0).toUpperCase() + status.slice(1);
}

function formatEpisode(
  season?: number,
  episode?: number,
  absoluteEpisode?: number,
  isAnime?: boolean,
): string {
  if (isAnime && absoluteEpisode) {
    return `Episode ${absoluteEpisode}`;
  }
  if (season !== undefined && episode !== undefined) {
    return `Season ${season.toString().padStart(2, "0")} Episode ${episode.toString().padStart(2, "0")}`;
  }
  return "Episode";
}

function formatTimestamp(timestamp: string): string {
  const date = new Date(timestamp);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMs / 3600000);
  const diffDays = Math.floor(diffMs / 86400000);

  if (diffMins < 1) return "Just now";
  if (diffMins < 60) return `${diffMins}m ago`;
  if (diffHours < 24) return `${diffHours}h ago`;
  if (diffDays < 7) return `${diffDays}d ago`;
  return date.toLocaleDateString();
}

// ==================== NotificationCard Component ====================

interface NotificationCardProps {
  group: NotificationGroup;
  onMarkAsRead: (id: number) => void;
  onDismiss: (id: number) => void;
}

function NotificationCard({
  group,
  onMarkAsRead,
  onDismiss,
}: NotificationCardProps) {
  const [expanded, setExpanded] = useState(false);
  const theme = useTheme();

  const hasMultiple = group.notifications.length > 1;
  const latestNotification = group.notifications[0]; // Assuming sorted by latest
  const unreadCount = group.notifications.filter((n) => !n.is_read).length;

  const handleCardClick = () => {
    // Mark all unread notifications in this group as read
    group.notifications.forEach((notification) => {
      if (!notification.is_read) {
        onMarkAsRead(notification.id);
      }
    });
    if (hasMultiple) {
      setExpanded(!expanded);
    }
  };

  return (
    <Card
      sx={{
        mb: 2,
        backgroundColor: theme.palette.background.paper,
        border: `1px solid ${theme.palette.divider}`,
        cursor: hasMultiple ? "pointer" : "default",
        transition: "all 0.2s ease",
        "&:hover": {
          backgroundColor: theme.palette.action.hover,
        },
      }}
      onClick={handleCardClick}
    >
      <CardContent sx={{ p: 2, "&:last-child": { pb: 2 } }}>
        <Box sx={{ display: "flex", gap: 2 }}>
          {/* Poster Image */}
          {group.poster_url && (
            <CardMedia
              component="img"
              sx={{
                width: 60,
                height: 90,
                borderRadius: 1,
                objectFit: "cover",
                flexShrink: 0,
              }}
              image={group.poster_url}
              alt={group.media_title}
            />
          )}

          {/* Content */}
          <Box sx={{ flex: 1, minWidth: 0 }}>
            {/* Title and Dismiss Button */}
            <Box
              sx={{
                display: "flex",
                justifyContent: "space-between",
                alignItems: "start",
                mb: 0.5,
              }}
            >
              <Typography
                variant="h6"
                sx={{
                  fontSize: "1rem",
                  fontWeight: 600,
                  color: theme.palette.text.primary,
                  mr: 1,
                }}
              >
                {group.media_title}
              </Typography>
              <IconButton
                size="small"
                onClick={(e) => {
                  e.stopPropagation();
                  onDismiss(latestNotification.id);
                }}
                sx={{ ml: "auto", flexShrink: 0 }}
              >
                <DismissIcon fontSize="small" />
              </IconButton>
            </Box>

            {/* Status Chip */}
            <Chip
              icon={getStatusIcon(group.latest_status)}
              label={getStatusLabel(group.latest_status)}
              size="small"
              sx={{
                backgroundColor: alpha(getStatusColor(group.latest_status), 0.16),
                color: getStatusColor(group.latest_status),
                fontWeight: 600,
                fontSize: "0.75rem",
                height: 24,
                mb: 0.5,
                "& .MuiChip-icon": {
                  color: getStatusColor(group.latest_status),
                },
              }}
            />

            {/* Latest Episode Info (for series) */}
            {group.category === "series" &&
              latestNotification.season !== undefined && (
                <Typography
                  variant="body2"
                  sx={{ color: theme.palette.text.secondary, mb: 0.5 }}
                >
                  {formatEpisode(
                    latestNotification.season,
                    latestNotification.episode,
                    latestNotification.absolute_episode,
                    group.is_anime,
                  )}
                </Typography>
              )}

            {/* Failure Reason */}
            {latestNotification.status === "failed" &&
              latestNotification.reason && (
                <Typography
                  variant="caption"
                  sx={{ color: theme.palette.error.main, display: "block", mb: 0.5 }}
                >
                  {latestNotification.reason}
                </Typography>
              )}

            {/* Timestamp and Count */}
            <Box
              sx={{
                display: "flex",
                justifyContent: "space-between",
                alignItems: "center",
                mt: 1,
              }}
            >
              <Typography
                variant="caption"
                sx={{ color: theme.palette.text.secondary }}
              >
                {formatTimestamp(group.latest_timestamp)}
              </Typography>
              {hasMultiple && (
                <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
                  {unreadCount > 0 && (
                    <Chip
                      label={`${unreadCount} unread`}
                      size="small"
                      sx={{
                        height: 20,
                        fontSize: "0.7rem",
                        backgroundColor: theme.palette.primary.main,
                        color: "white",
                      }}
                    />
                  )}
                  <ExpandMoreIcon
                    sx={{
                      transform: expanded ? "rotate(180deg)" : "rotate(0deg)",
                      transition: "transform 0.3s ease",
                      color: theme.palette.text.secondary,
                    }}
                  />
                </Box>
              )}
            </Box>
          </Box>
        </Box>

        {/* Expanded Episode List */}
        {hasMultiple && (
          <Collapse in={expanded}>
            <List dense sx={{ mt: 2, pl: 2 }}>
              {group.notifications.map((notification) => (
                <ListItem
                  key={notification.id}
                  sx={{
                    py: 0.5,
                    px: 1,
                    borderRadius: 1,
                    backgroundColor: notification.is_read
                      ? "transparent"
                      : theme.palette.action.selected,
                  }}
                >
                  <Box
                    sx={{
                      display: "flex",
                      alignItems: "center",
                      width: "100%",
                      gap: 1,
                    }}
                  >
                    <Box sx={{ color: getStatusColor(notification.status) }}>
                      {getStatusIcon(notification.status)}
                    </Box>
                    <ListItemText
                      primary={formatEpisode(
                        notification.season,
                        notification.episode,
                        notification.absolute_episode,
                        group.is_anime,
                      )}
                      secondary={getStatusLabel(notification.status)}
                      primaryTypographyProps={{
                        variant: "body2",
                        sx: { color: theme.palette.text.primary, fontSize: "0.875rem" },
                      }}
                      secondaryTypographyProps={{
                        variant: "caption",
                        sx: {
                          color: theme.palette.text.secondary,
                          fontSize: "0.75rem",
                        },
                      }}
                    />
                    {notification.status === "failed" &&
                      notification.reason && (
                        <Typography
                          variant="caption"
                          sx={{
                            color: theme.palette.error.main,
                            fontSize: "0.7rem",
                            maxWidth: 200,
                            overflow: "hidden",
                            textOverflow: "ellipsis",
                            whiteSpace: "nowrap",
                          }}
                        >
                          {notification.reason}
                        </Typography>
                      )}
                  </Box>
                </ListItem>
              ))}
            </List>
          </Collapse>
        )}
      </CardContent>
    </Card>
  );
}

// ==================== Main NotificationDropdown Component ====================

export default function NotificationDropdown() {
  const [isOpen, setIsOpen] = useState(false);
  const dropdownRef = useRef<HTMLDivElement>(null);
  const theme = useTheme();

  // Queries and Mutations
  const unreadCount = useUnreadCount();
  const groupedNotifications = useGroupedNotifications(isOpen);
  const markAsReadMutation = useMarkAsRead();
  const dismissMutation = useDismissNotification();

  // Handle ESC key
  useEffect(() => {
    const handleEscape = (e: KeyboardEvent) => {
      if (e.key === "Escape" && isOpen) {
        setIsOpen(false);
      }
    };
    window.addEventListener("keydown", handleEscape);
    return () => window.removeEventListener("keydown", handleEscape);
  }, [isOpen]);

  // Auto-focus dropdown when opened
  useEffect(() => {
    if (isOpen && dropdownRef.current) {
      dropdownRef.current.focus();
    }
  }, [isOpen]);

  const handleToggle = () => {
    setIsOpen(!isOpen);
  };

  const handleClose = () => {
    setIsOpen(false);
  };

  const handleMarkAsRead = (id: number) => {
    markAsReadMutation.mutate(id);
  };

  const handleDismiss = (id: number) => {
    dismissMutation.mutate(id);
  };

  const notificationGroups = groupedNotifications.data
    ? Object.values(groupedNotifications.data)
    : [];

  return (
    <>
      {/* Bell Icon Button */}
      <Tooltip title="Notifications">
        <IconButton
          onClick={handleToggle}
          aria-label={
            unreadCount.data ? `Notifications, ${unreadCount.data} unread` : "Notifications"
          }
          aria-expanded={isOpen}
          sx={{
            color: isOpen ? theme.palette.text.primary : theme.palette.text.secondary,
            "&:hover": {
              color: theme.palette.text.primary,
              backgroundColor: alpha(theme.palette.primary.main, 0.14),
            },
          }}
        >
          <Badge
            badgeContent={unreadCount.data || 0}
            sx={{
              "& .MuiBadge-badge": {
                minWidth: 18,
                height: 18,
                fontSize: "0.6875rem",
                fontWeight: 700,
                backgroundColor: theme.palette.primary.main,
                color: theme.palette.common.white,
                border: `2px solid ${theme.palette.background.default}`,
              },
            }}
          >
            <NotificationsIcon />
          </Badge>
        </IconButton>
      </Tooltip>

      {/* Backdrop */}
      {isOpen && (
        <Backdrop
          open={isOpen}
          onClick={handleClose}
          sx={{
            backgroundColor: alpha("#020617", 0.6),
            backdropFilter: "blur(6px)",
            zIndex: 1200,
          }}
        />
      )}

      {/* Dropdown */}
      <Slide direction="down" in={isOpen} mountOnEnter unmountOnExit>
        <Box
          ref={dropdownRef}
          tabIndex={-1}
          role="dialog"
          aria-label="Notifications"
          sx={{
            position: "fixed",
            // Hangs off the bell rather than covering the navbar
            top: 76,
            right: { xs: 8, sm: 24 },
            width: { xs: "calc(100vw - 16px)", sm: 440 },
            maxHeight: "calc(100vh - 100px)",
            backgroundColor: theme.palette.background.paper,
            border: `1px solid ${theme.palette.divider}`,
            borderRadius: 3,
            boxShadow: theme.shadows[16],
            zIndex: 1300,
            display: "flex",
            flexDirection: "column",
            overflow: "hidden",
          }}
        >
          {/* Header */}
          <Box
            sx={{
              display: "flex",
              justifyContent: "space-between",
              alignItems: "center",
              px: 2.5,
              py: 1.75,
              borderBottom: `1px solid ${theme.palette.divider}`,
            }}
          >
            <Box sx={{ display: "flex", alignItems: "baseline", gap: 1.25 }}>
              <Typography variant="h6" sx={{ color: theme.palette.text.primary }}>
                Notifications
              </Typography>
              {!!unreadCount.data && (
                <Typography variant="caption" sx={{ color: theme.palette.text.secondary }}>
                  {unreadCount.data} unread
                </Typography>
              )}
            </Box>
            <IconButton onClick={handleClose} size="small">
              <CloseIcon />
            </IconButton>
          </Box>

          {/* Body */}
          <Box
            sx={{
              flex: 1,
              overflowY: "auto",
              p: 2,
              "&::-webkit-scrollbar": {
                width: 8,
              },
              "&::-webkit-scrollbar-track": {
                backgroundColor: "transparent",
              },
              "&::-webkit-scrollbar-thumb": {
                backgroundColor: theme.palette.action.selected,
                borderRadius: 4,
              },
            }}
          >
            {/* Loading State */}
            {groupedNotifications.isLoading && (
              <>
                {[1, 2, 3].map((i) => (
                  <Card
                    key={i}
                    sx={{
                      mb: 2,
                      backgroundColor: theme.palette.background.paper,
                    }}
                  >
                    <CardContent>
                      <Box sx={{ display: "flex", gap: 2 }}>
                        <Skeleton
                          variant="rectangular"
                          width={60}
                          height={90}
                          sx={{ borderRadius: 1 }}
                        />
                        <Box sx={{ flex: 1 }}>
                          <Skeleton variant="text" width="60%" height={24} />
                          <Skeleton
                            variant="text"
                            width="40%"
                            height={20}
                            sx={{ mt: 1 }}
                          />
                          <Skeleton
                            variant="text"
                            width="30%"
                            height={16}
                            sx={{ mt: 0.5 }}
                          />
                        </Box>
                      </Box>
                    </CardContent>
                  </Card>
                ))}
              </>
            )}

            {/* Error State */}
            {groupedNotifications.isError && (
              <Box sx={{ textAlign: "center", py: 6 }}>
                <Typography variant="subtitle1" sx={{ color: theme.palette.text.primary }}>
                  Couldn't load notifications
                </Typography>
                <Typography
                  variant="body2"
                  sx={{ color: theme.palette.text.secondary, mb: 2 }}
                >
                  Scout couldn't reach the server.
                </Typography>
                <Button
                  variant="outlined"
                  size="small"
                  color="primary"
                  onClick={() => groupedNotifications.refetch()}
                >
                  Retry
                </Button>
              </Box>
            )}

            {/* Empty State */}
            {!groupedNotifications.isLoading &&
              !groupedNotifications.isError &&
              notificationGroups.length === 0 && (
                <Box sx={{ textAlign: "center", py: 6 }}>
                  <NoNotificationsIcon
                    sx={{ fontSize: 34, color: theme.palette.text.disabled, mb: 1 }}
                  />
                  <Typography variant="subtitle1" sx={{ color: theme.palette.text.primary }}>
                    You're all caught up
                  </Typography>
                  <Typography variant="body2" sx={{ color: theme.palette.text.secondary }}>
                    Download activity will show up here.
                  </Typography>
                </Box>
              )}

            {/* Notification Groups */}
            {!groupedNotifications.isLoading &&
              !groupedNotifications.isError &&
              notificationGroups.map((group) => (
                <NotificationCard
                  key={group.tvdb_id}
                  group={group}
                  onMarkAsRead={handleMarkAsRead}
                  onDismiss={handleDismiss}
                />
              ))}
          </Box>
        </Box>
      </Slide>
    </>
  );
}
