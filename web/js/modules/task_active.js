import { showConfirm, showAlert, showToast } from './dialog.js';
import { apiFetch } from '../api.js';
import { formatSeconds } from '../utils.js';

export function renderActiveTask(task) {
  const noActiveMsg = document.getElementById('noActiveTaskMsg');
  const activeContent = document.getElementById('activeTaskContent');
  if (!noActiveMsg || !activeContent) return;

  const cardTitle = document.getElementById('activeCardTitle');
  const liveDot = document.getElementById('activeLiveDot');
  const activeCard = document.getElementById('activeTaskCard');

  const abortActiveBtn = document.getElementById('abortActiveBtn');
  const delActiveBtn = document.getElementById('delActiveBtn');

  if (!task) {
    noActiveMsg.style.display = 'block';
    activeContent.style.display = 'none';
    if (cardTitle) cardTitle.textContent = '转码执行器 (空闲待命)';
    if (liveDot) liveDot.classList.remove('active');
    if (activeCard) activeCard.classList.remove('is-busy');
    if (abortActiveBtn) abortActiveBtn.dataset.taskId = '';
    if (delActiveBtn) delActiveBtn.dataset.taskId = '';
    return;
  }

  noActiveMsg.style.display = 'none';
  activeContent.style.display = 'block';
  if (cardTitle) cardTitle.textContent = '正在转码 (1/1 串行独占)';
  if (liveDot) liveDot.classList.add('active');
  if (activeCard) activeCard.classList.add('is-busy');

  const fileNameEl = document.getElementById('activeTaskFileName');
  if (fileNameEl) fileNameEl.textContent = task.source_file_name;

  const profileEl = document.getElementById('activeTaskProfile');
  if (profileEl) {
    const pName = (task.params && task.params.profile_name) ? task.params.profile_name.toUpperCase() : 'BALANCED';
    const vCodec = (task.params && task.params.video_codec) ? task.params.video_codec.toUpperCase() : 'HEVC';
    profileEl.textContent = `${pName} (${vCodec})`;
  }

  const p = task.progress || {};
  const pct = Math.min(100, Math.max(0, p.percent || 0));

  const fillEl = document.getElementById('activeProgressBar') || document.getElementById('activeProgressFill');
  if (fillEl) fillEl.style.width = `${pct}%`;

  const pctTextEl = document.getElementById('activeProgressPct') || document.getElementById('activeProgressText');
  if (pctTextEl) pctTextEl.textContent = `${pct.toFixed(1)}%`;

  const speedFpsEl = document.getElementById('activeSpeedFps');
  if (speedFpsEl) {
    const fpsText = p.current_fps ? `${p.current_fps.toFixed(1)} fps` : '-- fps';
    const speedText = p.speed ? `(${p.speed})` : '(--x)';
    speedFpsEl.textContent = `${fpsText} ${speedText}`;
  }

  const fpsEl = document.getElementById('activeFPS');
  if (fpsEl) fpsEl.textContent = p.current_fps ? `${p.current_fps.toFixed(1)} fps` : '--';

  const speedEl = document.getElementById('activeSpeed');
  if (speedEl) speedEl.textContent = p.speed || '--';

  const timeEl = document.getElementById('activeTime');
  if (timeEl) {
    timeEl.textContent = p.current_time_str || p.time_str || (p.current_seconds ? formatSeconds(p.current_seconds) : '00:00:00');
  }

  const bitrateEl = document.getElementById('activeBitrate');
  if (bitrateEl) bitrateEl.textContent = p.bitrate || '--';

  const etaEl = document.getElementById('activeETA');
  if (etaEl) {
    if (p.eta_seconds && p.eta_seconds > 0) {
      etaEl.textContent = formatSeconds(p.eta_seconds);
    } else {
      etaEl.textContent = '计算中...';
    }
  }

  if (abortActiveBtn) abortActiveBtn.dataset.taskId = task.id;
  if (delActiveBtn) delActiveBtn.dataset.taskId = task.id;
}

export async function abortActiveTask(onAborted) {
  const abortActiveBtn = document.getElementById('abortActiveBtn');
  const taskId = abortActiveBtn ? abortActiveBtn.dataset.taskId : '';

  const confirmed = await showConfirm({
    title: '强行终止转码',
    message: '确定要强行终止正在运行的任务吗？未完成的转码残片将被自动清除。',
    confirmText: '确认终止',
    cancelText: '取消',
    type: 'danger',
    icon: '⏹',
  });
  if (!confirmed) return;

  try {
    const url = taskId ? `/api/v1/tasks/${taskId}/abort` : '/api/v1/tasks/active/abort';
    const res = await apiFetch(url, { method: 'POST' });
    if (res.ok) {
      showToast('任务已终止', 'info');
      if (onAborted) onAborted();
    } else {
      const err = await res.json().catch(() => ({}));
      const msg = err.message || '无法终止任务';
      if (msg.includes('无需终止') || msg.includes('已转码完成') || msg.includes('当前没有正在执行的任务')) {
        showToast(msg, 'info');
        if (onAborted) onAborted();
      } else {
        showAlert({ title: '终止失败', message: msg, type: 'error' });
      }
    }
  } catch (e) {
    showAlert({ title: '网络错误', message: String(e), type: 'error' });
  }
}

export async function deleteActiveTask(onDeleted) {
  const delActiveBtn = document.getElementById('delActiveBtn');
  const taskId = delActiveBtn ? delActiveBtn.dataset.taskId : null;
  if (!taskId) return;

  const confirmed = await showConfirm({
    title: '彻底删除任务与文件',
    message: '确定要强行终止当前任务并彻底删除已上传视频源文件吗？此操作无法撤销。',
    confirmText: '确认删除',
    cancelText: '取消',
    type: 'danger',
    icon: '🗑️',
  });
  if (!confirmed) return;

  try {
    const res = await apiFetch(`/api/v1/tasks/${taskId}`, { method: 'DELETE' });
    if (res.ok) {
      showToast('当前任务与文件已彻底删除', 'info');
      if (onDeleted) onDeleted();
    } else {
      const err = await res.json().catch(() => ({}));
      showAlert({ title: '删除失败', message: err.message || '无法删除任务', type: 'error' });
    }
  } catch (e) {
    showAlert({ title: '网络错误', message: String(e), type: 'error' });
  }
}
