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
} from "../hooks/useSettings";
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
  const update = useUpdateSettings();
  const [quality, setQuality] = useState(view.values.min_quality);
  useEffect(() => setQuality(view.values.min_quality), [view.values.min_quality]);
  const dirty = quality !== view.values.min_quality;

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
      <Box sx={{ mt: 2 }}>
        <Button variant="contained" size="small" disabled={!dirty || update.isPending} onClick={() => update.mutate({ min_quality: quality })}>
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
            <NotificationsSection />
          </>
        )}
      </Container>
    </Box>
  );
};

export default Settings;
