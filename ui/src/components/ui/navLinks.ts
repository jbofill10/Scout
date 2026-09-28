import type { SvgIconComponent } from '@mui/icons-material';
import HomeRoundedIcon from '@mui/icons-material/HomeRounded';
import LiveTvRoundedIcon from '@mui/icons-material/LiveTvRounded';
import MovieRoundedIcon from '@mui/icons-material/MovieRounded';
import VideoLibraryRoundedIcon from '@mui/icons-material/VideoLibraryRounded';
import DownloadRoundedIcon from '@mui/icons-material/DownloadRounded';

export interface NavLink {
  label: string;
  path: string;
  /** Only the phone tab bar renders the icon; the desktop pills are text. */
  icon: SvgIconComponent;
}

/** The app's top-level routes, in display order. Shared by both navigations. */
export const NAV_LINKS: NavLink[] = [
  { label: 'Home', path: '/', icon: HomeRoundedIcon },
  { label: 'Shows', path: '/shows', icon: LiveTvRoundedIcon },
  { label: 'Movies', path: '/movies', icon: MovieRoundedIcon },
  { label: 'Library', path: '/library', icon: VideoLibraryRoundedIcon },
  { label: 'Activity', path: '/activity', icon: DownloadRoundedIcon },
];
