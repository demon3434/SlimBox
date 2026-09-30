import {
  formatBytes, formatDateTime,
  formatDuration, calcSpeedRatio, escapeHtml,
} from '../utils.js';

function getCodecAlias(codec) {
  if (!codec) return '--';
  const c = codec.toLowerCase();
  if (c === 'hevc_videotoolbox') return 'Apple H.265';
  if (c === 'h264_videotoolbox') return 'Apple H.264';
  if (c === 'libx265') return 'H.265';
  if (c === 'libx264') return 'H.264';
  return codec;
}

function formatCompactDate(dateStr) {
  if (!dateStr) return '--';
  const d = new Date(dateStr);
  if (isNaN(d.getTime())) return '--';
  const pad = (n) => String(n).padStart(2, '0');
  return `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export function buildQuadrantDrawerHtml(t, isCompact = false) {
  const m = t.media_info || {}, v = m.video || {}, p = t.params || {};

  const srcRes = v.width && v.height ? `${v.width}×${v.height}` : '--';
  const srcFps = v.fps ? `${v.fps.toFixed(2)} fps` : '--';
  const srcCodec = (v.codec_name || '--').toUpperCase();
  const srcDuration = m.duration_seconds ? formatDuration(m.duration_seconds) : '--';
  const srcBitrate = m.bitrate ? `${(m.bitrate / 1000).toFixed(0)} kbps` : v.bitrate ? `${(v.bitrate / 1000).toFixed(0)} kbps` : '--';
  const audioTracksCount = m.audio_tracks ? m.audio_tracks.length : 0;
  const audioTrackText = audioTracksCount > 0 ? `${audioTracksCount} 轨 (${m.audio_tracks.map((a) => a.codec_name || 'aac').slice(0, 2).join('/')})` : '自动侦测';
  const subTracksCount = m.subtitle_tracks ? m.subtitle_tracks.length : 0;
  const subTrackText = subTracksCount > 0 ? `${subTracksCount} 条软字幕` : '无字幕';

  const profileName = (p.profile_name || (t.status === 'pending' ? '未配置(待启动)' : '默认')).toUpperCase();
  const targetCodec = p.video_codec || '--';
  const codecAlias = getCodecAlias(targetCodec);
  const crf = p.crf ? `CRF ${p.crf}` : '--';
  const audioBitrate = p.audio_bitrate ? `AAC ${p.audio_bitrate}` : '--';
  const subPolicy = p.subtitle_policy === 'drop' ? '丢弃字幕' : p.subtitle_policy ? '软字幕直通 copy' : '--';

  // 解析并合并目标分辨率
  let resLabel = '';
  if (p.target_width && p.target_height) {
    resLabel = `${p.target_width}×${p.target_height}`;
  } else if (p.target_resolution) {
    if (p.target_resolution === 'original') {
      resLabel = (v.width && v.height) ? `原画 (${v.width}×${v.height})` : '原画';
    } else {
      resLabel = p.target_resolution.toUpperCase();
    }
  } else if (p.profile_name && p.profile_name !== 'custom' && p.profile_name !== 'pending') {
    resLabel = p.profile_name.toUpperCase();
  } else if (v.width && v.height) {
    resLabel = `原画 (${v.width}×${v.height})`;
  } else {
    resLabel = '';
  }

  const isStdTier = ['360P', '480P', '720P', '1080P', '2K', '4K'].includes(profileName);
  let profileCombined = '';
  let fullProfileTitle = '';
  if (profileName === 'CUSTOM') {
    profileCombined = resLabel ? `CUSTOM · ${resLabel} · ${codecAlias}` : `CUSTOM · ${codecAlias}`;
    fullProfileTitle = resLabel ? `CUSTOM (${resLabel}) · ${targetCodec}` : `CUSTOM · ${targetCodec}`;
  } else if (!resLabel || profileName === resLabel || isStdTier) {
    profileCombined = `${profileName} · ${codecAlias}`;
    fullProfileTitle = resLabel && profileName !== resLabel ? `${profileName} (${resLabel}) · ${targetCodec}` : `${profileName} · ${targetCodec}`;
  } else {
    profileCombined = `${profileName} (${resLabel}) · ${codecAlias}`;
    fullProfileTitle = `${profileName} (${resLabel}) · ${targetCodec}`;
  }

  const isAppleHW = targetCodec.includes('videotoolbox');
  let qualitySpeedText = '';
  if (isAppleHW) {
    const qVal = p.crf ? Math.max(20, Math.min(95, 50 - (p.crf - 24) * 2)) : 50;
    qualitySpeedText = `Q${qVal} (VBR)`;
  } else {
    qualitySpeedText = p.preset ? `${crf} / ${p.preset}` : crf;
  }

  const specText = `${srcCodec} · ${srcRes} @ ${srcFps}`;
  const durBitrateText = `${srcDuration} · ${srcBitrate}`;

  const q1AndQ2 = `
    <div class="quadrant-box">
      <div class="quadrant-title">📊 原始文件档案</div>
      <div class="quadrant-item"><span class="qk">视频规格</span><span class="qv" title="${escapeHtml(specText)}">${escapeHtml(specText)}</span></div>
      <div class="quadrant-item"><span class="qk">时长 / 码率</span><span class="qv" title="${escapeHtml(durBitrateText)}">${escapeHtml(durBitrateText)}</span></div>
      <div class="quadrant-item"><span class="qk">音频轨道</span><span class="qv" title="${escapeHtml(audioTrackText)}">${escapeHtml(audioTrackText)}</span></div>
      <div class="quadrant-item"><span class="qk">内嵌软字幕</span><span class="qv" title="${escapeHtml(subTrackText)}">${escapeHtml(subTrackText)}</span></div>
    </div>
    <div class="quadrant-box">
      <div class="quadrant-title">⚙️ 目标压缩档案</div>
      <div class="quadrant-item"><span class="qk">预设档位</span><span class="qv" title="${escapeHtml(fullProfileTitle)}">${escapeHtml(profileCombined)}</span></div>
      <div class="quadrant-item"><span class="qk">质量速度</span><span class="qv" title="${escapeHtml(qualitySpeedText)}">${escapeHtml(qualitySpeedText)}</span></div>
      <div class="quadrant-item"><span class="qk">音频输出</span><span class="qv" title="${escapeHtml(audioBitrate)}">${escapeHtml(audioBitrate)}</span></div>
      <div class="quadrant-item"><span class="qk">字幕策略</span><span class="qv" title="${escapeHtml(subPolicy)}">${escapeHtml(subPolicy)}</span></div>
    </div>`;

  if (isCompact) return `<div class="film-sheet-drawer"><div class="film-quadrant-grid">${q1AndQ2}</div></div>`;

  const srcSize = t.source_file_size || 0, outSize = t.output_file_size || 0;
  let savingsPct = 0, savingsBytes = 0;
  if (srcSize > 0 && outSize > 0 && t.status === 'completed') {
    savingsBytes = srcSize - outSize;
    savingsPct = Math.max(0, Math.round((savingsBytes / srcSize) * 100));
  }
  const savingsBarWidth = Math.min(100, Math.max(0, savingsPct));
  const storageStatus = t.status === 'completed' ? '成品文件保留中' : '半成品残片已清理 (0 B)';

  const addedTimeFull = formatDateTime(t.created_at);
  const addedTimeCompact = formatCompactDate(t.created_at);
  let transcodeDuration = '--', speedRatio = '--';
  if (t.started_at && t.completed_at) {
    const durSec = (new Date(t.completed_at).getTime() - new Date(t.started_at).getTime()) / 1000;
    if (durSec > 0) {
      transcodeDuration = formatDuration(durSec);
      if (m.duration_seconds && m.duration_seconds > 0) speedRatio = calcSpeedRatio(m.duration_seconds, durSec);
    }
  }

  const sizeReductionText = `${formatBytes(srcSize)} ➔ ${outSize > 0 ? formatBytes(outSize) : '--'}`;
  const savedBytesText = savingsBytes > 0 ? formatBytes(savingsBytes) : '--';

  return `<div class="film-sheet-drawer"><div class="film-quadrant-grid">
    ${q1AndQ2}
    <div class="quadrant-box">
      <div class="quadrant-title">📉 瘦身与存储成效</div>
      <div class="quadrant-item"><span class="qk">体积缩减</span><span class="qv" title="${escapeHtml(sizeReductionText)}">${escapeHtml(sizeReductionText)}</span></div>
      <div class="savings-bar-container"><div class="savings-bar-track"><div class="savings-bar-fill" style="width: ${savingsBarWidth}%;"></div></div><span class="savings-bar-label">${savingsPct > 0 ? `${savingsPct}%⬇` : (savingsPct < 0 ? `+${Math.abs(savingsPct)}%⬆` : '--')}</span></div>
      <div class="quadrant-item"><span class="qk">节省空间</span><span class="qv" title="${escapeHtml(savedBytesText)}" style="color:var(--accent-emerald); font-weight:600;">${escapeHtml(savedBytesText)}</span></div>
      <div class="quadrant-item"><span class="qk">存储状态</span><span class="qv" title="${escapeHtml(storageStatus)}">${escapeHtml(storageStatus)}</span></div>
    </div>
    <div class="quadrant-box">
      <div class="quadrant-title">⏱️ 时间与压制性能</div>
      <div class="quadrant-item"><span class="qk">添加时间</span><span class="qv" title="${escapeHtml(addedTimeFull)}">${escapeHtml(addedTimeCompact)}</span></div>
      <div class="quadrant-item"><span class="qk">转码耗时</span><span class="qv" title="${escapeHtml(transcodeDuration)}">${escapeHtml(transcodeDuration)}</span></div>
      <div class="quadrant-item"><span class="qk">转码倍速</span><span class="qv" title="${escapeHtml(speedRatio)}" style="color:var(--accent-cyan); font-weight:600;">${escapeHtml(speedRatio)}</span></div>
      <div class="quadrant-item"><span class="qk">下载统计</span><span class="qv" title="${t.download_count || 0} 次">${t.download_count || 0} 次</span></div>
    </div>
  </div></div>`;
}
