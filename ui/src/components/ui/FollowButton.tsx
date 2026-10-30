import React, { useState } from "react";
import Button from "@mui/material/Button";
import CircularProgress from "@mui/material/CircularProgress";
import ListItemText from "@mui/material/ListItemText";
import Menu from "@mui/material/Menu";
import MenuItem from "@mui/material/MenuItem";
import BookmarkAddOutlinedIcon from "@mui/icons-material/BookmarkAddOutlined";
import BookmarkAddedRoundedIcon from "@mui/icons-material/BookmarkAddedRounded";
import { useFollows, useFollowShow, useUnfollowShow, type FollowMedia } from "../../hooks/useFollows";

export interface FollowButtonProps {
  media: FollowMedia;
  size?: "small" | "medium";
}

/**
 * FollowButton
 *
 * Follow a show so new episodes are requested on their own, or stop. The
 * first follow asks whether the back catalogue should come along, since
 * that can be a lot of downloads.
 */
export const FollowButton: React.FC<FollowButtonProps> = ({ media, size = "medium" }) => {
  const { data: follows } = useFollows();
  const follow = useFollowShow();
  const unfollow = useUnfollowShow();
  const [anchor, setAnchor] = useState<HTMLElement | null>(null);

  const followed = follows?.some((f) => f.tvdb_id === media.id) ?? false;
  const busy = follow.isPending || unfollow.isPending;

  const choose = (includeMissing: boolean) => {
    setAnchor(null);
    follow.mutate({ media, includeMissing });
  };

  if (followed) {
    return (
      <Button
        size={size}
        variant="outlined"
        color="inherit"
        startIcon={busy ? <CircularProgress size={16} color="inherit" /> : <BookmarkAddedRoundedIcon />}
        disabled={busy}
        onClick={() => unfollow.mutate({ tvdbId: media.id, title: media.mediaName })}
        aria-label={`Unfollow ${media.mediaName}`}
      >
        Following
      </Button>
    );
  }

  return (
    <>
      <Button
        size={size}
        variant="outlined"
        startIcon={busy ? <CircularProgress size={16} color="inherit" /> : <BookmarkAddOutlinedIcon />}
        disabled={busy}
        onClick={(e) => setAnchor(e.currentTarget)}
        aria-haspopup="menu"
        aria-label={`Follow ${media.mediaName}`}
      >
        Follow
      </Button>
      <Menu anchorEl={anchor} open={anchor !== null} onClose={() => setAnchor(null)}>
        <MenuItem onClick={() => choose(false)}>
          <ListItemText primary="New episodes only" secondary="Anything airing from today on" />
        </MenuItem>
        <MenuItem onClick={() => choose(true)}>
          <ListItemText primary="Also grab missing episodes" secondary="Everything not in your library, then new episodes" />
        </MenuItem>
      </Menu>
    </>
  );
};

export default FollowButton;
