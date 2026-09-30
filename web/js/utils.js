// Utility helper functions

/**
 * Format bytes to human readable string (KB, MB, GB, TB).
 * @param {number} bytes
 * @returns {string}
 */
export function formatBytes(bytes) {
  if (!bytes || bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
}

/**
 * Format seconds to Chinese human readable string (e.g. 1时20分10秒).
 * @param {number} sec
 * @returns {string}
 */
export function formatSeconds(sec) {
  const s = Math.round(sec);
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const seconds = s % 60;
  if (h > 0) {
    return `${h}时${m}分${seconds}秒`;
  }
  return `${m}分${seconds}秒`;
}

/**
 * Sanitize and escape HTML special characters to prevent XSS.
 * @param {string} str
 * @returns {string}
 */
export function escapeHtml(str) {
  if (!str) return '';
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}

/**
 * Format ISO datetime string or Date object to YYYY-MM-DD HH:mm:ss.
 * @param {string|Date} dateStr
 * @returns {string}
 */
export function formatDateTime(dateStr) {
  if (!dateStr) return '--';
  const d = new Date(dateStr);
  if (isNaN(d.getTime())) return '--';
  const pad = (n) => String(n).padStart(2, '0');
  const year = d.getFullYear();
  const month = pad(d.getMonth() + 1);
  const day = pad(d.getDate());
  const hours = pad(d.getHours());
  const mins = pad(d.getMinutes());
  const secs = pad(d.getSeconds());
  return `${year}-${month}-${day} ${hours}:${mins}:${secs}`;
}

/**
 * Format duration in seconds into human-friendly string (e.g. 14分32秒, 45秒).
 * @param {number} seconds
 * @returns {string}
 */
export function formatDuration(seconds) {
  if (seconds == null || isNaN(seconds) || seconds < 0) return '--';
  const total = Math.round(seconds);
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  if (h > 0) {
    return `${h}时${m}分${s}秒`;
  }
  if (m > 0) {
    return `${m}分${s}秒`;
  }
  return `${s}秒`;
}

/**
 * Calculate transcoding speed ratio (video duration / transcode duration).
 * @param {number} videoDurationSec
 * @param {number} transcodeDurationSec
 * @returns {string}
 */
export function calcSpeedRatio(videoDurationSec, transcodeDurationSec) {
  if (!videoDurationSec || !transcodeDurationSec || transcodeDurationSec <= 0) {
    return '--';
  }
  const ratio = (videoDurationSec / transcodeDurationSec).toFixed(1);
  return `${ratio}x 实时`;
}

