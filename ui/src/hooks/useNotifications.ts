import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';

// ==================== Types ====================

export interface Notification {
  id: number;
  tvdb_id: string;
  media_title: string;
  category: 'series' | 'movie';
  season?: number;
  episode?: number;
  absolute_episode?: number;
  poster_url?: string;
  is_anime: boolean;
  status: 'scheduled' | 'searching' | 'downloading' | 'completed' | 'failed';
  reason?: string;
  is_read: boolean;
  auto_dismissed: boolean;
  created_at: string;
  updated_at: string;
}

export interface NotificationGroup {
  tvdb_id: string;
  media_title: string;
  category: 'series' | 'movie';
  poster_url?: string;
  is_anime: boolean;
  latest_status: string;
  latest_timestamp: string;
  notifications: Notification[];
}

export interface GroupedNotifications {
  [tvdb_id: string]: NotificationGroup;
}

export interface UnreadCountResponse {
  count: number;
}

// ==================== API Functions ====================

const API_BASE = '/api/notifications';

async function fetchNotifications(): Promise<Notification[]> {
  const response = await fetch(`${API_BASE}?limit=100`);
  if (!response.ok) {
    throw new Error(`Failed to fetch notifications: ${response.statusText}`);
  }
  return response.json();
}

async function fetchGroupedNotifications(): Promise<GroupedNotifications> {
  const response = await fetch(`${API_BASE}/grouped`);
  if (!response.ok) {
    throw new Error(`Failed to fetch grouped notifications: ${response.statusText}`);
  }
  const data = await response.json();

  // Transform array response to object keyed by tvdb_id
  // Backend returns: [{tvdb_id, notifications: [...]}, ...]
  // UI expects: {tvdb_id: {tvdb_id, notifications: [...], latest_status}, ...}
  const grouped: GroupedNotifications = {};
  for (const group of data) {
    // Calculate latest_status from the notifications array
    // Notifications are sorted by created_at DESC (most recent first)
    const latestStatus = group.notifications?.[0]?.status || 'unknown';
    const latestTimestamp = group.notifications?.[0]?.updated_at || group.notifications?.[0]?.created_at;

    grouped[group.tvdb_id] = {
      ...group,
      latest_status: latestStatus,
      latest_timestamp: latestTimestamp,
    };
  }
  return grouped;
}

async function fetchUnreadCount(): Promise<UnreadCountResponse> {
  const response = await fetch(`${API_BASE}/unread/count`);
  if (!response.ok) {
    throw new Error(`Failed to fetch unread count: ${response.statusText}`);
  }
  return response.json();
}

async function markNotificationAsRead(id: number): Promise<void> {
  const response = await fetch(`${API_BASE}/${id}/read`, {
    method: 'PATCH',
  });
  if (!response.ok) {
    throw new Error(`Failed to mark notification as read: ${response.statusText}`);
  }
}

async function dismissNotification(id: number): Promise<void> {
  const response = await fetch(`${API_BASE}/${id}`, {
    method: 'DELETE',
  });
  if (!response.ok) {
    throw new Error(`Failed to dismiss notification: ${response.statusText}`);
  }
}

// ==================== Query Hooks ====================

/**
 * Fetch all notifications (not grouped)
 * Useful for simple notification lists
 */
export function useNotifications() {
  return useQuery({
    queryKey: ['notifications'],
    queryFn: fetchNotifications,
    staleTime: 30000, // 30 seconds
    refetchInterval: 30000, // Auto-refresh every 30 seconds
  });
}

/**
 * Fetch notifications grouped by media (tvdb_id)
 * This is the primary hook for the NotificationDropdown
 */
export function useGroupedNotifications(enabled: boolean = true) {
  return useQuery({
    queryKey: ['notifications', 'grouped'],
    queryFn: fetchGroupedNotifications,
    staleTime: 30000, // 30 seconds
    refetchInterval: enabled ? 30000 : false, // Auto-refresh only when enabled
    enabled, // Allow disabling query (e.g., when dropdown is closed)
  });
}

/**
 * Fetch unread notification count
 * Used for the bell icon badge
 */
export function useUnreadCount() {
  return useQuery({
    queryKey: ['notifications', 'unread', 'count'],
    queryFn: fetchUnreadCount,
    staleTime: 30000, // 30 seconds
    refetchInterval: 30000, // Auto-refresh every 30 seconds
    select: (data) => data.count, // Extract just the count number
  });
}

// ==================== Mutation Hooks ====================

/**
 * Mark a single notification as read
 */
export function useMarkAsRead() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: markNotificationAsRead,
    onMutate: async (notificationId) => {
      // Cancel outgoing refetches
      await queryClient.cancelQueries({ queryKey: ['notifications'] });

      // Snapshot previous value
      const previousNotifications = queryClient.getQueryData(['notifications']);
      const previousGrouped = queryClient.getQueryData(['notifications', 'grouped']);
      const previousCount = queryClient.getQueryData(['notifications', 'unread', 'count']);

      // Optimistically update - mark as read
      queryClient.setQueryData<Notification[]>(['notifications'], (old) => {
        if (!old) return old;
        return old.map((n) => (n.id === notificationId ? { ...n, is_read: true } : n));
      });

      queryClient.setQueryData<GroupedNotifications>(['notifications', 'grouped'], (old) => {
        if (!old) return old;
        const updated = { ...old };
        Object.keys(updated).forEach((tvdbId) => {
          updated[tvdbId] = {
            ...updated[tvdbId],
            notifications: updated[tvdbId].notifications.map((n) =>
              n.id === notificationId ? { ...n, is_read: true } : n
            ),
          };
        });
        return updated;
      });

      queryClient.setQueryData<number>(['notifications', 'unread', 'count'], (old) => {
        if (!old) return old;
        return Math.max(0, old - 1);
      });

      return { previousNotifications, previousGrouped, previousCount };
    },
    onError: (_err, _notificationId, context) => {
      // Revert optimistic updates on error
      if (context?.previousNotifications) {
        queryClient.setQueryData(['notifications'], context.previousNotifications);
      }
      if (context?.previousGrouped) {
        queryClient.setQueryData(['notifications', 'grouped'], context.previousGrouped);
      }
      if (context?.previousCount) {
        queryClient.setQueryData(['notifications', 'unread', 'count'], context.previousCount);
      }
    },
    onSettled: () => {
      // Always refetch after mutation completes
      queryClient.invalidateQueries({ queryKey: ['notifications'] });
    },
  });
}

/**
 * Dismiss (soft delete) a notification
 */
export function useDismissNotification() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: dismissNotification,
    onMutate: async (notificationId) => {
      await queryClient.cancelQueries({ queryKey: ['notifications'] });

      const previousNotifications = queryClient.getQueryData(['notifications']);
      const previousGrouped = queryClient.getQueryData(['notifications', 'grouped']);
      const previousCount = queryClient.getQueryData(['notifications', 'unread', 'count']);

      // Optimistically remove notification
      queryClient.setQueryData<Notification[]>(['notifications'], (old) => {
        if (!old) return old;
        return old.filter((n) => n.id !== notificationId);
      });

      queryClient.setQueryData<GroupedNotifications>(['notifications', 'grouped'], (old) => {
        if (!old) return old;
        const updated = { ...old };
        Object.keys(updated).forEach((tvdbId) => {
          const filtered = updated[tvdbId].notifications.filter((n) => n.id !== notificationId);
          if (filtered.length === 0) {
            delete updated[tvdbId]; // Remove group if no notifications left
          } else {
            updated[tvdbId] = {
              ...updated[tvdbId],
              notifications: filtered,
            };
          }
        });
        return updated;
      });

      // Decrement unread count if notification was unread
      queryClient.setQueryData<Notification[]>(['notifications'], (old) => {
        const notification = old?.find((n) => n.id === notificationId);
        if (notification && !notification.is_read) {
          queryClient.setQueryData<number>(['notifications', 'unread', 'count'], (count) => {
            if (!count) return count;
            return Math.max(0, count - 1);
          });
        }
        return old;
      });

      return { previousNotifications, previousGrouped, previousCount };
    },
    onError: (_err, _notificationId, context) => {
      if (context?.previousNotifications) {
        queryClient.setQueryData(['notifications'], context.previousNotifications);
      }
      if (context?.previousGrouped) {
        queryClient.setQueryData(['notifications', 'grouped'], context.previousGrouped);
      }
      if (context?.previousCount) {
        queryClient.setQueryData(['notifications', 'unread', 'count'], context.previousCount);
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: ['notifications'] });
    },
  });
}

/**
 * Dismiss all notifications in a group (by tvdb_id)
 */
export function useDismissGroup() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (notificationIds: number[]) => {
      await Promise.all(notificationIds.map((id) => dismissNotification(id)));
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['notifications'] });
    },
  });
}
