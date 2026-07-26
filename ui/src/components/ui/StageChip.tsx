import React from "react";
import Chip from "@mui/material/Chip";
import { alpha } from "@mui/material/styles";
import { getStageColor, getStageLabel } from "../../utils/downloadStage";
import { getStageIcon } from "./stageIcon";

export interface StageChipProps {
  stage: string | undefined;
  /** Overrides the stage name as the chip label (e.g. "Retrying"). */
  label?: string;
  size?: "small" | "medium";
}

/**
 * StageChip
 *
 * The one place a download stage is rendered as a badge, so the bell dropdown
 * and the Activity page always agree on colour and wording.
 */
export const StageChip: React.FC<StageChipProps> = ({ stage, label, size = "small" }) => {
  const color = getStageColor(stage);

  return (
    <Chip
      icon={getStageIcon(stage)}
      label={label ?? getStageLabel(stage)}
      size={size}
      sx={{
        backgroundColor: alpha(color, 0.16),
        color,
        fontWeight: 600,
        fontSize: "0.75rem",
        height: 24,
        "& .MuiChip-icon": { color },
      }}
    />
  );
};

export default StageChip;
