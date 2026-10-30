import React, { useEffect, useMemo, useState } from "react";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Chip from "@mui/material/Chip";
import Container from "@mui/material/Container";
import FormControl from "@mui/material/FormControl";
import FormControlLabel from "@mui/material/FormControlLabel";
import InputLabel from "@mui/material/InputLabel";
import MenuItem from "@mui/material/MenuItem";
import Select from "@mui/material/Select";
import Skeleton from "@mui/material/Skeleton";
import Switch from "@mui/material/Switch";
import TextField from "@mui/material/TextField";
import Typography from "@mui/material/Typography";
import Alert from "@mui/material/Alert";
import { alpha, useTheme } from "@mui/material/styles";
import PageHeader from "../components/ui/PageHeader";
import {
  useAddUploader,
  useRemoveUploader,
  useSettings,
  useUpdateSettings,
  useUploaderPreferences,
  type SettingsView,
  type UploaderPreference,
  type RateLimit,
} from "../hooks/useSettings";
import { useIndexerHealth } from "../hooks/useIndexerHealth";
import { durationError, invalidLanguage, parseLanguages, tidyDuration } from "../utils/settingsForm";
import IconButton from "@mui/material/IconButton";
import Tooltip from "@mui/material/Tooltip";
import CircularProgress from "@mui/material/CircularProgress";
import DeleteOutlineIcon from "@mui/icons-material/DeleteOutline";
import SendIcon from "@mui/icons-material/Send";
import {
  TARGET_EVENTS,
  TARGET_KINDS,
  useCreateTarget,
  useDeleteTarget,
  useNotificationTargets,
  useTestTarget,
  useUpdateTarget,
  type NotificationTarget,
  type TargetEvent,
  type TargetKind,
} from "../hooks/useNotificationTargets";
import { formatRelativeTime } from "../utils/downloadStage";
import { formatBytes } from "../utils/formatBytes";
import { describeAttempt, useRunUpgrades, useUpgradeAttempts, useUpgradeCandidates } from "../hooks/useUpgrades";

/** A titled card holding one group of settings. */
const SettingCard: React.FC<{ title: string; description: string; children: React.ReactNode }> = ({
  title,
  description,
  children,
}) => {
  const theme = useTheme();
  return (
    <Box
      component="section"
      aria-label={title}
      sx={{
        mb: 3,
        p: { xs: 2, sm: 3 },
        backgroundColor: theme.palette.background.paper,
        border: `1px solid ${theme.palette.divider}`,
        borderRadius: 3,
      }}
    >
      <Typography variant="h6" sx={{ color: theme.palette.text.primary }}>
        {title}
      </Typography>
      <Typography variant="body2" sx={{ color: theme.palette.text.secondary, mb: 2.5 }}>
        {description}
      </Typography>
      {children}
    </Box>
  );
};

/** "Default: 1080p · Overridden" with a reset link when a value is overridden. */
const DefaultHint: React.FC<{ defaultLabel: string; overridden: boolean; onReset: () => void; busy: boolean }> = ({
  defaultLabel,
  overridden,
  onReset,
  busy,
}) => {
  const theme = useTheme();
  return (
    <Box sx={{ display: "flex", alignItems: "center", gap: 1, mt: 0.75, flexWrap: "wrap" }}>
      <Typography variant="caption" sx={{ color: theme.palette.text.secondary }}>
        Default from the environment: {defaultLabel || "none"}
      </Typography>
      {overridden && (
        <>
          <Chip size="small" label="Overridden" sx={{ height: 20, backgroundColor: alpha(theme.palette.primary.main, 0.18) }} />
          <Button size="small" onClick={onReset} disabled={busy} sx={{ minWidth: 0, px: 1 }}>
            Reset
          </Button>
        </>
      )}
    </Box>
  );
};

const DownloadsSection: React.FC<{ view: SettingsView }> = ({ view }) => {
  const theme = useTheme();
  const update = useUpdateSettings();
  const [quality, setQuality] = useState(view.values.min_quality);
  const [reserve, setReserve] = useState(String(view.values.min_free_space_gb));
  useEffect(() => setQuality(view.values.min_quality), [view.values.min_quality]);
  useEffect(() => setReserve(String(view.values.min_free_space_gb)), [view.values.min_free_space_gb]);
  const reserveNum = Number(reserve);
  const reserveError = reserve.trim() === "" || !Number.isInteger(reserveNum) || reserveNum < 0 || reserveNum > 100000 ? "Whole number of gigabytes, 0 or more" : null;
  const dirty = quality !== view.values.min_quality || (!reserveError && reserveNum !== view.values.min_free_space_gb);
  const freeNow = view.options.free_space_bytes >= 0 ? formatBytes(view.options.free_space_bytes) : null;

  const save = () => {
    const body: Parameters<typeof update.mutate>[0] = {};
    if (quality !== view.values.min_quality) body.min_quality = quality;
    if (!reserveError && reserveNum !== view.values.min_free_space_gb) body.min_free_space_gb = reserveNum;
    update.mutate(body);
  };

  return (
    <SettingCard
      title="Downloads"
      description="The lowest resolution Scout will accept. Releases below it are passed over even when nothing else is available, and the search moves on to the fallback indexer."
    >
      <FormControl size="small" sx={{ minWidth: 200 }}>
        <InputLabel id="min-quality-label">Minimum quality</InputLabel>
        <Select
          labelId="min-quality-label"
          label="Minimum quality"
          value={quality}
          onChange={(e) => setQuality(e.target.value)}
        >
          {view.options.min_quality.map((q) => (
            <MenuItem key={q} value={q}>
              {q}
            </MenuItem>
          ))}
        </Select>
      </FormControl>
      <DefaultHint
        defaultLabel={view.defaults.min_quality}
        overridden={!!view.overridden.min_quality}
        onReset={() => update.mutate({ reset: ["min_quality"] })}
        busy={update.isPending}
      />

      <Box sx={{ mt: 2.5 }}>
        <TextField
          size="small"
          label="Keep free on the downloads volume (GB)"
          value={reserve}
          onChange={(e) => setReserve(e.target.value)}
          error={!!reserveError}
          helperText={reserveError ?? (freeNow ? `Free right now: ${freeNow}. A release that would eat into the reserve is not started.` : "A release that would eat into the reserve is not started.")}
          slotProps={{ htmlInput: { inputMode: "numeric", pattern: "[0-9]*" } }}
          sx={{ minWidth: 300 }}
        />
        <DefaultHint
          defaultLabel={`${view.defaults.min_free_space_gb} GB`}
          overridden={!!view.overridden.min_free_space_gb}
          onReset={() => update.mutate({ reset: ["min_free_space_gb"] })}
          busy={update.isPending}
        />
        {!freeNow && (
          <Typography variant="caption" sx={{ color: theme.palette.text.disabled }}>
            Free space could not be read.
          </Typography>
        )}
      </Box>

      <Box sx={{ mt: 2 }}>
        <Button variant="contained" size="small" disabled={!dirty || !!reserveError || update.isPending} onClick={save}>
          Save
        </Button>
      </Box>
    </SettingCard>
  );
};

const SubtitlesSection: React.FC<{ view: SettingsView }> = ({ view }) => {
  const update = useUpdateSettings();
  const [languages, setLanguages] = useState(view.values.subtitle_languages.join(", "));
  const [sdh, setSdh] = useState(view.values.subtitle_prefer_sdh);
  useEffect(() => setLanguages(view.values.subtitle_languages.join(", ")), [view.values.subtitle_languages]);
  useEffect(() => setSdh(view.values.subtitle_prefer_sdh), [view.values.subtitle_prefer_sdh]);

  const parsed = useMemo(() => parseLanguages(languages), [languages]);
  const badLang = invalidLanguage(parsed);
  const languageError = badLang ? `"${badLang}" is not a two or three letter code` : parsed.length === 0 ? "At least one language" : null;
  const dirty = parsed.join(",") !== view.values.subtitle_languages.join(",") || sdh !== view.values.subtitle_prefer_sdh;

  const save = () => {
    const body: Parameters<typeof update.mutate>[0] = {};
    if (parsed.join(",") !== view.values.subtitle_languages.join(",")) body.subtitle_languages = parsed;
    if (sdh !== view.values.subtitle_prefer_sdh) body.subtitle_prefer_sdh = sdh;
    update.mutate(body);
  };

  return (
    <SettingCard
      title="Subtitles"
      description="Languages to fetch from OpenSubtitles once a download lands, as ISO 639 codes. One file is written per language that has a match."
    >
      {!view.options.subtitles_enabled && (
        <Alert severity="info" sx={{ mb: 2 }}>
          Subtitle fetching is off: no OpenSubtitles API key is configured. These settings are kept but have no effect until one is.
        </Alert>
      )}
      <TextField
        size="small"
        label="Languages"
        value={languages}
        onChange={(e) => setLanguages(e.target.value)}
        error={!!languageError}
        helperText={languageError ?? "Comma separated, e.g. en, es"}
        sx={{ minWidth: 260 }}
      />
      <DefaultHint
        defaultLabel={view.defaults.subtitle_languages.join(", ")}
        overridden={!!view.overridden.subtitle_languages}
        onReset={() => update.mutate({ reset: ["subtitle_languages"] })}
        busy={update.isPending}
      />
      <Box sx={{ mt: 2 }}>
        <FormControlLabel
          control={<Switch checked={sdh} onChange={(e) => setSdh(e.target.checked)} />}
          label="Prefer hearing-impaired (SDH) tracks when both exist"
        />
        <DefaultHint
          defaultLabel={view.defaults.subtitle_prefer_sdh ? "on" : "off"}
          overridden={!!view.overridden.subtitle_prefer_sdh}
          onReset={() => update.mutate({ reset: ["subtitle_prefer_sdh"] })}
          busy={update.isPending}
        />
      </Box>
      <Box sx={{ mt: 2 }}>
        <Button variant="contained" size="small" disabled={!dirty || !!languageError || update.isPending} onClick={save}>
          Save
        </Button>
      </Box>
    </SettingCard>
  );
};

const LibrarySyncSection: React.FC<{ view: SettingsView }> = ({ view }) => {
  const update = useUpdateSettings();
  const [delay, setDelay] = useState(tidyDuration(view.values.plex_sync_delay));
  const [interval, setInterval_] = useState(tidyDuration(view.values.plex_sync_interval));
  useEffect(() => setDelay(tidyDuration(view.values.plex_sync_delay)), [view.values.plex_sync_delay]);
  useEffect(() => setInterval_(tidyDuration(view.values.plex_sync_interval)), [view.values.plex_sync_interval]);

  const delayError = durationError(delay);
  const intervalError = durationError(interval, { allowZero: true, min: "1m" });
  const dirty = delay !== tidyDuration(view.values.plex_sync_delay) || interval !== tidyDuration(view.values.plex_sync_interval);

  const save = () => {
    const body: Parameters<typeof update.mutate>[0] = {};
    if (delay !== tidyDuration(view.values.plex_sync_delay)) body.plex_sync_delay = delay.trim();
    if (interval !== tidyDuration(view.values.plex_sync_interval)) body.plex_sync_interval = interval.trim();
    update.mutate(body);
  };

  return (
    <SettingCard
      title="Library sync"
      description="How Scout keeps its copy of the Plex library current: a quiet period after the last download lands before re-reading it, and a periodic re-read as a backstop."
    >
      <Box sx={{ display: "flex", gap: 2, flexWrap: "wrap" }}>
        <Box>
          <TextField
            size="small"
            label="Sync after a download"
            value={delay}
            onChange={(e) => setDelay(e.target.value)}
            error={!!delayError}
            helperText={delayError ?? "e.g. 2m"}
          />
          <DefaultHint
            defaultLabel={tidyDuration(view.defaults.plex_sync_delay)}
            overridden={!!view.overridden.plex_sync_delay}
            onReset={() => update.mutate({ reset: ["plex_sync_delay"] })}
            busy={update.isPending}
          />
        </Box>
        <Box>
          <TextField
            size="small"
            label="Periodic sync every"
            value={interval}
            onChange={(e) => setInterval_(e.target.value)}
            error={!!intervalError}
            helperText={intervalError ?? "e.g. 30m, or 0s to turn off"}
          />
          <DefaultHint
            defaultLabel={tidyDuration(view.defaults.plex_sync_interval, "off")}
            overridden={!!view.overridden.plex_sync_interval}
            onReset={() => update.mutate({ reset: ["plex_sync_interval"] })}
            busy={update.isPending}
          />
        </Box>
      </Box>
      <Box sx={{ mt: 2 }}>
        <Button variant="contained" size="small" disabled={!dirty || !!delayError || !!intervalError || update.isPending} onClick={save}>
          Save
        </Button>
      </Box>
    </SettingCard>
  );
};

const SeasonPacksSection: React.FC<{ view: SettingsView }> = ({ view }) => {
  const update = useUpdateSettings();
  const [min, setMin] = useState(String(view.values.season_pack_min_episodes));
  useEffect(() => setMin(String(view.values.season_pack_min_episodes)), [view.values.season_pack_min_episodes]);
  const minNum = Number(min);
  const minError = !Number.isInteger(minNum) || minNum < 2 || minNum > 100 ? "Whole number between 2 and 100" : null;
  const dirty = !minError && minNum !== view.values.season_pack_min_episodes;

  return (
    <SettingCard
      title="Season packs"
      description="When a request covers enough episodes of one season, look for a single season pack first and fall back to episode-by-episode searches if none fits. One well-seeded pack beats ten searches against a rate-limited indexer; episodes the pack turns out not to contain are marked failed individually."
    >
      <FormControlLabel
        control={<Switch checked={view.values.season_packs} disabled={update.isPending} onChange={(e) => update.mutate({ season_packs: e.target.checked })} />}
        label="Try a season pack before searching per episode"
      />
      <DefaultHint
        defaultLabel={view.defaults.season_packs ? "on" : "off"}
        overridden={!!view.overridden.season_packs}
        onReset={() => update.mutate({ reset: ["season_packs"] })}
        busy={update.isPending}
      />
      <Box sx={{ mt: 2 }}>
        <TextField
          size="small"
          label="Minimum episodes of a season"
          value={min}
          onChange={(e) => setMin(e.target.value)}
          error={!!minError}
          helperText={minError ?? "Fewer than this and the season is searched per episode"}
          slotProps={{ htmlInput: { inputMode: "numeric", pattern: "[0-9]*" } }}
          sx={{ width: 260 }}
        />
        <DefaultHint
          defaultLabel={String(view.defaults.season_pack_min_episodes)}
          overridden={!!view.overridden.season_pack_min_episodes}
          onReset={() => update.mutate({ reset: ["season_pack_min_episodes"] })}
          busy={update.isPending}
        />
      </Box>
      <Box sx={{ mt: 2 }}>
        <Button variant="contained" size="small" disabled={!dirty || update.isPending} onClick={() => update.mutate({ season_pack_min_episodes: minNum })}>
          Save
        </Button>
      </Box>
    </SettingCard>
  );
};

const UpgradesSection: React.FC<{ view: SettingsView }> = ({ view }) => {
  const theme = useTheme();
  const update = useUpdateSettings();
  const candidates = useUpgradeCandidates();
  const attempts = useUpgradeAttempts();
  const run = useRunUpgrades();
  const [cutoff, setCutoff] = useState(view.values.upgrade_cutoff);
  const [batch, setBatch] = useState(String(view.values.upgrade_batch));
  useEffect(() => setCutoff(view.values.upgrade_cutoff), [view.values.upgrade_cutoff]);
  useEffect(() => setBatch(String(view.values.upgrade_batch)), [view.values.upgrade_batch]);
  const batchNum = Number(batch);
  const batchError = !Number.isInteger(batchNum) || batchNum < 1 || batchNum > 50 ? "Whole number between 1 and 50" : null;
  const dirty = cutoff !== view.values.upgrade_cutoff || (!batchError && batchNum !== view.values.upgrade_batch);

  const save = () => {
    const body: Parameters<typeof update.mutate>[0] = {};
    if (cutoff !== view.values.upgrade_cutoff) body.upgrade_cutoff = cutoff;
    if (!batchError && batchNum !== view.values.upgrade_batch) body.upgrade_batch = batchNum;
    update.mutate(body);
  };

  const statusColor = (status: string) =>
    status === "completed" ? theme.palette.success.main : status === "downloading" || status === "searching" ? theme.palette.text.secondary : theme.palette.error.main;

  return (
    <SettingCard
      title="Quality upgrades"
      description="Replace library copies below a cutoff with better releases. Each pass searches a small batch so the indexers are not hammered; a miss is retried after a week and given up on after three tries. The old file is removed from the library folder when the better copy lands."
    >
      <FormControlLabel
        control={<Switch checked={view.values.upgrades_enabled} disabled={update.isPending} onChange={(e) => update.mutate({ upgrades_enabled: e.target.checked })} />}
        label="Upgrade copies below the cutoff automatically"
      />
      <DefaultHint
        defaultLabel={view.defaults.upgrades_enabled ? "on" : "off"}
        overridden={!!view.overridden.upgrades_enabled}
        onReset={() => update.mutate({ reset: ["upgrades_enabled"] })}
        busy={update.isPending}
      />

      <Box sx={{ display: "flex", gap: 2, flexWrap: "wrap", mt: 2 }}>
        <Box>
          <FormControl size="small" sx={{ minWidth: 180 }}>
            <InputLabel id="upgrade-cutoff-label">Upgrade up to</InputLabel>
            <Select labelId="upgrade-cutoff-label" label="Upgrade up to" value={cutoff} onChange={(e) => setCutoff(e.target.value)}>
              {view.options.min_quality.map((q) => (
                <MenuItem key={q} value={q}>
                  {q}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
          <DefaultHint
            defaultLabel={view.defaults.upgrade_cutoff}
            overridden={!!view.overridden.upgrade_cutoff}
            onReset={() => update.mutate({ reset: ["upgrade_cutoff"] })}
            busy={update.isPending}
          />
        </Box>
        <Box>
          <TextField
            size="small"
            label="Items per pass"
            value={batch}
            onChange={(e) => setBatch(e.target.value)}
            error={!!batchError}
            helperText={batchError ?? "A pass runs every 6 hours"}
            slotProps={{ htmlInput: { inputMode: "numeric", pattern: "[0-9]*" } }}
            sx={{ width: 160 }}
          />
          <DefaultHint
            defaultLabel={String(view.defaults.upgrade_batch)}
            overridden={!!view.overridden.upgrade_batch}
            onReset={() => update.mutate({ reset: ["upgrade_batch"] })}
            busy={update.isPending}
          />
        </Box>
      </Box>
      <Box sx={{ mt: 2, display: "flex", gap: 1.5, alignItems: "center", flexWrap: "wrap" }}>
        <Button variant="contained" size="small" disabled={!dirty || !!batchError || update.isPending} onClick={save}>
          Save
        </Button>
        <Button variant="outlined" size="small" disabled={!view.values.upgrades_enabled || run.isPending} onClick={() => run.mutate()}>
          {run.isPending ? "Starting…" : "Run a pass now"}
        </Button>
        {candidates.data && (
          <Typography variant="caption" sx={{ color: theme.palette.text.secondary }}>
            {candidates.data.total} item{candidates.data.total === 1 ? "" : "s"} below {candidates.data.cutoff}
            {candidates.data.deferred > 0 ? ` · ${candidates.data.deferred} waiting on a retry or given up` : ""}
          </Typography>
        )}
      </Box>

      {attempts.data && attempts.data.length > 0 && (
        <Box sx={{ mt: 2.5 }}>
          <Typography variant="subtitle2" sx={{ color: theme.palette.text.secondary, mb: 0.75 }}>
            Recent attempts
          </Typography>
          {attempts.data.slice(0, 8).map((a) => (
            <Typography key={a.item_key} variant="caption" sx={{ display: "block", color: theme.palette.text.secondary }}>
              {describeAttempt(a)} · {a.from_quality || "?"} → {a.to_quality} ·{" "}
              <Box component="span" sx={{ color: statusColor(a.status) }}>
                {a.status.replace("_", " ")}
              </Box>
              {a.reason ? ` (${a.reason})` : ""} · {formatRelativeTime(a.updated_at)}
              {a.attempts > 1 ? ` · try ${a.attempts}` : ""}
            </Typography>
          ))}
        </Box>
      )}
    </SettingCard>
  );
};

const SearchPacingSection: React.FC<{ view: SettingsView }> = ({ view }) => {
  const update = useUpdateSettings();
  const { data: health } = useIndexerHealth(true);
  const [rate, setRate] = useState(String(view.values.search_rate_per_minute));
  const [burst, setBurst] = useState(String(view.values.search_burst));
  const [batch, setBatch] = useState(String(view.values.bulk_request_batch));
  const [overrides, setOverrides] = useState<Record<string, { per_minute: string; burst: string }>>({});
  useEffect(() => setRate(String(view.values.search_rate_per_minute)), [view.values.search_rate_per_minute]);
  useEffect(() => setBurst(String(view.values.search_burst)), [view.values.search_burst]);
  useEffect(() => setBatch(String(view.values.bulk_request_batch)), [view.values.bulk_request_batch]);
  useEffect(() => {
    const next: Record<string, { per_minute: string; burst: string }> = {};
    for (const [id, l] of Object.entries(view.values.indexer_rate_limits ?? {})) next[id] = { per_minute: String(l.per_minute), burst: String(l.burst) };
    setOverrides(next);
  }, [view.values.indexer_rate_limits]);

  const intIn = (raw: string, lo: number, hi: number) => {
    const n = Number(raw);
    return Number.isInteger(n) && n >= lo && n <= hi ? n : null;
  };
  const rateNum = intIn(rate, 1, 120);
  const burstNum = intIn(burst, 1, 30);
  const batchNum = intIn(batch, 1, 500);
  const globalDirty = (rateNum !== null && rateNum !== view.values.search_rate_per_minute) || (burstNum !== null && burstNum !== view.values.search_burst);
  const batchDirty = batchNum !== null && batchNum !== view.values.bulk_request_batch;

  const indexers = (health?.indexers ?? []).filter((i) => i.enabled);
  const overridesValid = Object.values(overrides).every((o) => (o.per_minute === "" && o.burst === "") || (intIn(o.per_minute, 1, 120) !== null && intIn(o.burst, 1, 30) !== null));
  const saveOverrides = () => {
    const limits: Record<string, RateLimit> = {};
    for (const [id, o] of Object.entries(overrides)) {
      const pm = intIn(o.per_minute, 1, 120);
      const b = intIn(o.burst, 1, 30);
      if (pm !== null && b !== null) limits[id] = { per_minute: pm, burst: b };
    }
    update.mutate({ indexer_rate_limits: limits });
  };

  return (
    <SettingCard
      title="Search pacing"
      description="Every indexer search waits its turn: one at a time per indexer, within a sustained rate with a small burst, and a person's request ahead of follow checks, missing-episode fills and upgrades. Public indexers ban the exit IP after a burst of searches; the defaults are safe for 1337x. An indexer Prowlarr has disabled is skipped until it is healthy again."
    >
      <Box sx={{ display: "flex", gap: 2, flexWrap: "wrap", alignItems: "flex-start" }}>
        <TextField
          size="small"
          label="Searches per minute per indexer"
          value={rate}
          onChange={(e) => setRate(e.target.value)}
          error={rateNum === null}
          helperText={rateNum === null ? "Whole number between 1 and 120" : `Default ${view.defaults.search_rate_per_minute}`}
          slotProps={{ htmlInput: { inputMode: "numeric", pattern: "[0-9]*" } }}
          sx={{ width: 260 }}
        />
        <TextField
          size="small"
          label="Burst"
          value={burst}
          onChange={(e) => setBurst(e.target.value)}
          error={burstNum === null}
          helperText={burstNum === null ? "Whole number between 1 and 30" : `Default ${view.defaults.search_burst}`}
          slotProps={{ htmlInput: { inputMode: "numeric", pattern: "[0-9]*" } }}
          sx={{ width: 160 }}
        />
        <Button
          variant="contained"
          size="small"
          disabled={!globalDirty || update.isPending}
          onClick={() => update.mutate({ ...(rateNum !== null ? { search_rate_per_minute: rateNum } : {}), ...(burstNum !== null ? { search_burst: burstNum } : {}) })}
          sx={{ mt: 0.5 }}
        >
          Save
        </Button>
      </Box>
      <DefaultHint
        defaultLabel={`${view.defaults.search_rate_per_minute}/min, burst ${view.defaults.search_burst}`}
        overridden={!!view.overridden.search_rate_per_minute || !!view.overridden.search_burst}
        onReset={() => update.mutate({ reset: ["search_rate_per_minute", "search_burst"] })}
        busy={update.isPending}
      />

      {indexers.length > 0 && (
        <Box sx={{ mt: 2 }}>
          <Typography variant="body2" sx={{ mb: 1 }}>
            Per-indexer overrides (blank uses the values above)
          </Typography>
          {indexers.map((ix) => {
            const o = overrides[String(ix.id)] ?? { per_minute: "", burst: "" };
            const set = (field: "per_minute" | "burst", value: string) => setOverrides((prev) => ({ ...prev, [String(ix.id)]: { ...(prev[String(ix.id)] ?? { per_minute: "", burst: "" }), [field]: value } }));
            return (
              <Box key={ix.id} sx={{ display: "flex", gap: 2, alignItems: "center", mb: 1, flexWrap: "wrap" }}>
                <Typography variant="body2" sx={{ width: 120 }}>
                  {ix.name}
                </Typography>
                <TextField size="small" label="Per minute" value={o.per_minute} onChange={(e) => set("per_minute", e.target.value)} slotProps={{ htmlInput: { inputMode: "numeric" } }} sx={{ width: 130 }} />
                <TextField size="small" label="Burst" value={o.burst} onChange={(e) => set("burst", e.target.value)} slotProps={{ htmlInput: { inputMode: "numeric" } }} sx={{ width: 110 }} />
              </Box>
            );
          })}
          <Button variant="outlined" size="small" disabled={!overridesValid || update.isPending} onClick={saveOverrides}>
            Save overrides
          </Button>
          <DefaultHint defaultLabel="none" overridden={!!view.overridden.indexer_rate_limits} onReset={() => update.mutate({ reset: ["indexer_rate_limits"] })} busy={update.isPending} />
        </Box>
      )}

      <Box sx={{ mt: 2, display: "flex", gap: 2, alignItems: "flex-start", flexWrap: "wrap" }}>
        <TextField
          size="small"
          label="Episodes a follow check may request per run"
          value={batch}
          onChange={(e) => setBatch(e.target.value)}
          error={batchNum === null}
          helperText={batchNum === null ? "Whole number between 1 and 500" : "A big back catalogue trickles in over several checks; Check now is never capped"}
          slotProps={{ htmlInput: { inputMode: "numeric", pattern: "[0-9]*" } }}
          sx={{ width: 420, maxWidth: "100%" }}
        />
        <Button variant="contained" size="small" disabled={!batchDirty || update.isPending} onClick={() => update.mutate({ bulk_request_batch: batchNum ?? undefined })} sx={{ mt: 0.5 }}>
          Save
        </Button>
      </Box>
      <DefaultHint
        defaultLabel={String(view.defaults.bulk_request_batch)}
        overridden={!!view.overridden.bulk_request_batch}
        onReset={() => update.mutate({ reset: ["bulk_request_batch"] })}
        busy={update.isPending}
      />
    </SettingCard>
  );
};

const IndexersSection: React.FC<{ view: SettingsView }> = ({ view }) => {
  const update = useUpdateSettings();
  const configured = view.options.relay_agent_configured;
  return (
    <SettingCard
      title="Indexers and relay"
      description="Public indexers ban the exit IP after a burst of searches. With the host relay agent installed, Scout can move to another Mullvad relay on its own and ask Prowlarr to re-test the indexer."
    >
      {!configured && (
        <Alert severity="info" sx={{ mb: 2 }}>
          No relay agent is configured (RELAY_AGENT_URL). Rotation is off until tools/relay-agent is installed on the host.
        </Alert>
      )}
      <FormControlLabel
        control={
          <Switch
            checked={view.values.auto_rotate_relay}
            disabled={!configured || update.isPending}
            onChange={(e) => update.mutate({ auto_rotate_relay: e.target.checked })}
          />
        }
        label="Rotate the relay automatically when an indexer blocks the exit IP"
      />
      <DefaultHint
        defaultLabel={view.defaults.auto_rotate_relay ? "on" : "off"}
        overridden={!!view.overridden.auto_rotate_relay}
        onReset={() => update.mutate({ reset: ["auto_rotate_relay"] })}
        busy={update.isPending}
      />
    </SettingCard>
  );
};

const GROUPS: { key: string; label: string; media_type: "series" | "movie"; is_anime: boolean }[] = [
  { key: "series-false", label: "Shows", media_type: "series", is_anime: false },
  { key: "series-true", label: "Anime shows", media_type: "series", is_anime: true },
  { key: "movie-false", label: "Movies", media_type: "movie", is_anime: false },
  { key: "movie-true", label: "Anime movies", media_type: "movie", is_anime: true },
];

const UploadersSection: React.FC = () => {
  const theme = useTheme();
  const { data, isLoading, isError } = useUploaderPreferences();
  const add = useAddUploader();
  const remove = useRemoveUploader();
  const [group, setGroup] = useState(GROUPS[0].key);
  const [uploader, setUploader] = useState("");

  const byGroup = useMemo(() => {
    const map = new Map<string, UploaderPreference[]>();
    for (const g of GROUPS) map.set(g.key, []);
    for (const p of data ?? []) map.get(`${p.media_type}-${p.is_anime}`)?.push(p);
    return map;
  }, [data]);

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    const g = GROUPS.find((x) => x.key === group);
    if (!g || !uploader.trim()) return;
    add.mutate({ media_type: g.media_type, is_anime: g.is_anime, uploader: uploader.trim() }, { onSuccess: () => setUploader("") });
  };

  return (
    <SettingCard
      title="Preferred uploaders"
      description="Release groups the picker favours when several copies match. Matched against the torrent title; earlier entries weigh more. SubsPlease and Erai-raws are always preferred for anime, and QxR, YIFY and GalaxyRG for movies."
    >
      {isLoading && <Skeleton variant="rectangular" height={80} sx={{ borderRadius: 2 }} />}
      {isError && <Alert severity="error">Couldn't load the uploader list.</Alert>}
      {data && (
        <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" }, gap: 2, mb: 2.5 }}>
          {GROUPS.map((g) => (
            <Box key={g.key}>
              <Typography variant="subtitle2" sx={{ color: theme.palette.text.secondary, mb: 0.75 }}>
                {g.label}
              </Typography>
              <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.75 }}>
                {(byGroup.get(g.key) ?? []).length === 0 && (
                  <Typography variant="caption" sx={{ color: theme.palette.text.disabled }}>
                    None
                  </Typography>
                )}
                {(byGroup.get(g.key) ?? []).map((p) => (
                  <Chip key={p.id} label={p.uploader} onDelete={() => remove.mutate(p)} disabled={remove.isPending} />
                ))}
              </Box>
            </Box>
          ))}
        </Box>
      )}

      <Box component="form" onSubmit={submit} sx={{ display: "flex", gap: 1.5, flexWrap: "wrap", alignItems: "center" }}>
        <FormControl size="small" sx={{ minWidth: 160 }}>
          <InputLabel id="uploader-group-label">For</InputLabel>
          <Select labelId="uploader-group-label" label="For" value={group} onChange={(e) => setGroup(e.target.value)}>
            {GROUPS.map((g) => (
              <MenuItem key={g.key} value={g.key}>
                {g.label}
              </MenuItem>
            ))}
          </Select>
        </FormControl>
        <TextField
          size="small"
          label="Uploader"
          value={uploader}
          onChange={(e) => setUploader(e.target.value)}
          slotProps={{ htmlInput: { maxLength: 64 } }}
          sx={{ minWidth: 200 }}
        />
        <Button type="submit" variant="outlined" size="small" disabled={!uploader.trim() || add.isPending}>
          Add
        </Button>
      </Box>
    </SettingCard>
  );
};

const TargetRow: React.FC<{ target: NotificationTarget }> = ({ target }) => {
  const theme = useTheme();
  const update = useUpdateTarget();
  const remove = useDeleteTarget();
  const test = useTestTarget();
  const kind = TARGET_KINDS.find((k) => k.value === target.kind)?.label ?? target.kind;

  const toggle = (enabled: boolean) =>
    update.mutate({ id: target.id, name: target.name, kind: target.kind, url: target.url, events: target.events, enabled });

  return (
    <Box
      sx={{
        display: "flex",
        gap: 1.5,
        alignItems: "center",
        py: 1.25,
        borderBottom: `1px solid ${theme.palette.divider}`,
        "&:last-of-type": { borderBottom: 0 },
        flexWrap: "wrap",
      }}
    >
      <Switch size="small" checked={target.enabled} onChange={(e) => toggle(e.target.checked)} disabled={update.isPending} inputProps={{ "aria-label": `Enable ${target.name}` }} />
      <Box sx={{ flex: 1, minWidth: 200 }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap" }}>
          <Typography variant="body2" sx={{ fontWeight: 650 }}>
            {target.name}
          </Typography>
          <Chip size="small" label={kind} sx={{ height: 20 }} />
          {target.events.map((e) => (
            <Chip key={e} size="small" variant="outlined" label={TARGET_EVENTS.find((x) => x.value === e)?.label ?? e} sx={{ height: 20 }} />
          ))}
        </Box>
        <Typography variant="caption" sx={{ display: "block", color: theme.palette.text.secondary, wordBreak: "break-all" }}>
          {target.url}
          {target.has_token ? " · token set" : ""}
        </Typography>
        <Typography variant="caption" sx={{ display: "block", color: target.last_error ? theme.palette.error.main : theme.palette.text.disabled }}>
          {target.last_error
            ? `Last delivery failed: ${target.last_error}`
            : target.last_sent_at
              ? `Last sent ${formatRelativeTime(target.last_sent_at)}`
              : "Nothing sent yet"}
        </Typography>
      </Box>
      <Tooltip title="Send a test">
        <span>
          <IconButton size="small" aria-label={`Test ${target.name}`} disabled={test.isPending} onClick={() => test.mutate(target)}>
            {test.isPending ? <CircularProgress size={18} /> : <SendIcon fontSize="small" />}
          </IconButton>
        </span>
      </Tooltip>
      <Tooltip title="Remove">
        <span>
          <IconButton size="small" aria-label={`Remove ${target.name}`} disabled={remove.isPending} onClick={() => remove.mutate(target)}>
            <DeleteOutlineIcon fontSize="small" />
          </IconButton>
        </span>
      </Tooltip>
    </Box>
  );
};

const NotificationsSection: React.FC = () => {
  const theme = useTheme();
  const { data, isLoading, isError } = useNotificationTargets();
  const create = useCreateTarget();
  const [name, setName] = useState("");
  const [kind, setKind] = useState<TargetKind>("webhook");
  const [url, setUrl] = useState("");
  const [token, setToken] = useState("");
  const [events, setEvents] = useState<TargetEvent[]>(["completed", "failed"]);

  const toggleEvent = (e: TargetEvent) => setEvents((cur) => (cur.includes(e) ? cur.filter((x) => x !== e) : [...cur, e]));
  const canAdd = name.trim() !== "" && /^https?:\/\/\S+/.test(url.trim()) && events.length > 0;

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!canAdd) return;
    create.mutate(
      { name: name.trim(), kind, url: url.trim(), token: token.trim() || undefined, events },
      {
        onSuccess: () => {
          setName("");
          setUrl("");
          setToken("");
        },
      },
    );
  };

  return (
    <SettingCard
      title="Notifications"
      description="Where finished, failed and cancelled downloads are sent, outside the app. A Home Assistant webhook is a Webhook target; the JSON arrives under trigger.json."
    >
      {isLoading && <Skeleton variant="rectangular" height={60} sx={{ borderRadius: 2 }} />}
      {isError && <Alert severity="error">Couldn't load the notification targets.</Alert>}
      {data && data.length === 0 && (
        <Typography variant="body2" sx={{ color: theme.palette.text.disabled, mb: 2 }}>
          No targets yet.
        </Typography>
      )}
      {data && data.length > 0 && (
        <Box sx={{ mb: 2.5 }}>
          {data.map((t) => (
            <TargetRow key={t.id} target={t} />
          ))}
        </Box>
      )}

      <Box component="form" onSubmit={submit} sx={{ display: "grid", gap: 1.5, gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" } }}>
        <TextField size="small" label="Name" value={name} onChange={(e) => setName(e.target.value)} slotProps={{ htmlInput: { maxLength: 64 } }} />
        <FormControl size="small">
          <InputLabel id="target-kind-label">Kind</InputLabel>
          <Select labelId="target-kind-label" label="Kind" value={kind} onChange={(e) => setKind(e.target.value as TargetKind)}>
            {TARGET_KINDS.map((k) => (
              <MenuItem key={k.value} value={k.value}>
                {k.label}
              </MenuItem>
            ))}
          </Select>
        </FormControl>
        <TextField
          size="small"
          label="URL"
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          helperText={TARGET_KINDS.find((k) => k.value === kind)?.hint}
          sx={{ gridColumn: { sm: "1 / -1" } }}
        />
        <TextField size="small" label="Token (optional)" type="password" value={token} onChange={(e) => setToken(e.target.value)} helperText="Sent as a bearer token" />
        <Box sx={{ display: "flex", gap: 1, alignItems: "center", flexWrap: "wrap" }}>
          {TARGET_EVENTS.map((e) => (
            <FormControlLabel
              key={e.value}
              control={<Switch size="small" checked={events.includes(e.value)} onChange={() => toggleEvent(e.value)} />}
              label={e.label}
            />
          ))}
        </Box>
        <Box>
          <Button type="submit" variant="outlined" size="small" disabled={!canAdd || create.isPending}>
            Add target
          </Button>
        </Box>
      </Box>
    </SettingCard>
  );
};

/**
 * Settings Page
 *
 * The knobs that used to live only in environment variables and raw SQL:
 * quality floor, preferred uploaders, subtitles and library sync. Each shows
 * its environment default and whether it is overridden; a change applies
 * within seconds without a restart.
 */
const Settings: React.FC = () => {
  const theme = useTheme();
  const { data, isLoading, isError, refetch } = useSettings();

  return (
    <Box sx={{ minHeight: "100vh", backgroundColor: theme.palette.background.default, pt: { xs: 11, md: 13 }, pb: 8 }}>
      <Container maxWidth="md">
        <PageHeader
          eyebrow="Scout"
          title="Settings"
          description="Changes apply within a few seconds, without a restart. Environment variables remain the defaults."
        />

        {isLoading && (
          <>
            <Skeleton variant="rectangular" height={160} sx={{ borderRadius: 3, mb: 3 }} />
            <Skeleton variant="rectangular" height={160} sx={{ borderRadius: 3, mb: 3 }} />
          </>
        )}

        {isError && !isLoading && (
          <Box sx={{ textAlign: "center", py: 8 }}>
            <Typography variant="subtitle1" sx={{ color: theme.palette.text.primary }}>
              Couldn't load settings
            </Typography>
            <Typography variant="body2" sx={{ color: theme.palette.text.secondary, mb: 2 }}>
              Scout couldn't reach the download service.
            </Typography>
            <Button variant="outlined" size="small" onClick={() => refetch()}>
              Retry
            </Button>
          </Box>
        )}

        {data && (
          <>
            <DownloadsSection view={data} />
            <UploadersSection />
            <SubtitlesSection view={data} />
            <LibrarySyncSection view={data} />
            <SeasonPacksSection view={data} />
            <UpgradesSection view={data} />
            <IndexersSection view={data} />
            <SearchPacingSection view={data} />
            <NotificationsSection />
          </>
        )}
      </Container>
    </Box>
  );
};

export default Settings;
