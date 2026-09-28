/**
 * Netflix-style UI Components
 *
 * Core components for Scout's Netflix-inspired interface.
 * All components follow Material-UI v7 patterns and warm dark theme.
 */

export { default as ScoutLogo, ScoutMark } from './ScoutLogo';
export type { ScoutLogoProps } from './ScoutLogo';

export { default as PageHeader } from './PageHeader';
export type { PageHeaderProps } from './PageHeader';

export { default as MediaCard } from './MediaCard';
export type { MediaCardProps } from './MediaCard';

export { default as HorizontalCarousel } from './HorizontalCarousel';
export { CAROUSEL_ITEM_SX } from './carouselItem';

export { default as GenreRow } from './GenreRow';

export { default as ScheduleWidget } from './ScheduleWidget';

export { default as Navbar } from './Navbar';

export { default as SearchDropdown } from './SearchDropdown';

export { default as NotificationDropdown } from './NotificationDropdown';

export { default as MediaStatusDialog } from './MediaStatusDialog';
export type { MediaStatusDialogProps } from './MediaStatusDialog';

export { default as StageChip } from './StageChip';
export type { StageChipProps } from './StageChip';

export { default as ToastProvider } from './ToastProvider';

export { default as BottomNav, BOTTOM_NAV_HEIGHT } from './BottomNav';
export { NAV_LINKS } from './navLinks';
export type { NavLink } from './navLinks';
