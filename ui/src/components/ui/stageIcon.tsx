import {
  CheckCircle as CompletedIcon,
  Download as DownloadingIcon,
  Search as SearchingIcon,
  Schedule as ScheduledIcon,
  Error as FailedIcon,
} from "@mui/icons-material";

/**
 * Icon for a download stage, sized for inline use next to text. Kept apart from
 * StageChip so both the chip and plain lists can reach for it.
 */
export function getStageIcon(stage: string | undefined, fontSize = 18) {
  switch (stage) {
    case "completed":
      return <CompletedIcon sx={{ fontSize }} />;
    case "downloading":
      return <DownloadingIcon sx={{ fontSize }} />;
    case "scheduled":
      return <ScheduledIcon sx={{ fontSize }} />;
    case "failed":
      return <FailedIcon sx={{ fontSize }} />;
    case "searching":
    default:
      return <SearchingIcon sx={{ fontSize }} />;
  }
}
