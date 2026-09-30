import { bus } from '../bus.js';
import { apiFetch } from '../api.js';
import { formatBytes, formatSeconds, escapeHtml } from '../utils.js';
import { getSelectedProfile } from './profiles.js';
import { showConfirm, showToast } from './dialog.js';

let stagedTasks = []; // Array of Task objects
let selectedTaskIds = new Set(); // Set of checked task IDs for batch enqueue
let activeInspectedTaskId = null; // ID of task currently active in probe card

export function displayProbeInfo(task) {
  const probeCard = document.getElementById('probeCard');
  if (!probeCard || !task) return;
  const media = task.media_info;
  document.getElementById('probeFileName').textContent = task.source_file_name;

  if (media && media.video) {
    document.getElementById('probeVideoCodec').textContent = media.video.codec_name.toUpperCase();
    document.getElementById('probeResolution').textContent = `${media.video.width} x ${media.video.height}`;
    // Fix: Duration first, then Framerate to match header "时长 / 帧率"
    document.getElementById('probeFpsDuration').textContent = `${formatSeconds(media.duration_seconds)} / ${media.video.fps.toFixed(1)} fps`;
  } else {
    document.getElementById('probeVideoCodec').textContent = '通用媒体';
    document.getElementById('probeResolution').textContent = '自适应';
    document.getElementById('probeFpsDuration').textContent = '--';
  }

  document.getElementById('probeSize').textContent = formatBytes(task.source_file_size);

  const audioCount = (media && media.audio_tracks) ? media.audio_tracks.length : 0;
  let audioDetail = '无音轨';
  if (media && media.audio_tracks && media.audio_tracks.length > 0) {
    audioDetail = media.audio_tracks.map((a) => {
      const parts = [];
      if (a.codec_name) parts.push(a.codec_name.toUpperCase());
      const ch = a.channel_layout || (a.channels === 2 ? '双声道' : (a.channels === 1 ? '单声道' : (a.channels ? `${a.channels}声道` : '')));
      if (ch) parts.push(ch);
      if (a.sample_rate) parts.push(`${(a.sample_rate / 1000).toFixed(1)} kHz`);
      return parts.join(' · ');
    }).join('；');
    if (audioCount > 1) {
      audioDetail = `${audioCount}轨: ${audioDetail}`;
    }
  }
  const audioEl = document.getElementById('probeAudioSummary');
  if (audioEl) {
    audioEl.textContent = audioDetail;
    audioEl.title = audioDetail;
  }

  const subCount = (media && media.subtitle_tracks) ? media.subtitle_tracks.length : 0;
  const subEl = document.getElementById('probeSubtitleSummary');
  if (subEl) {
    const subText = subCount > 0 ? `${subCount} 条 (直通保留)` : '无字幕 (直通)';
    subEl.textContent = subText;
    subEl.title = subText;
  }

  probeCard.style.display = 'block';
}

export function getStagedTasks() {
  return stagedTasks;
}

export function getSelectedStagedTaskIds() {
  return Array.from(selectedTaskIds);
}

export function removeStagedTaskIds(idsToRemove) {
  const removeSet = new Set(idsToRemove);
  stagedTasks = stagedTasks.filter(t => !removeSet.has(t.id));
  idsToRemove.forEach(id => selectedTaskIds.delete(id));

  if (activeInspectedTaskId && removeSet.has(activeInspectedTaskId)) {
    activeInspectedTaskId = stagedTasks.length > 0 ? stagedTasks[0].id : null;
    if (activeInspectedTaskId) {
      const nextTask = stagedTasks.find(t => t.id === activeInspectedTaskId);
      if (nextTask) displayProbeInfo(nextTask);
    }
  }

  // Default re-select remaining items if none currently selected
  if (stagedTasks.length > 0 && selectedTaskIds.size === 0) {
    stagedTasks.forEach(t => selectedTaskIds.add(t.id));
  }

  renderStagedBatchUI();
  updateEnqueueButtonState();
  if (stagedTasks.length === 0) {
    const probeCard = document.getElementById('probeCard');
    if (probeCard) probeCard.style.display = 'none';
  }
}

export function setStagedTasks(tasks) {
  stagedTasks = tasks || [];
  selectedTaskIds = new Set(stagedTasks.map(t => t.id));
  activeInspectedTaskId = stagedTasks.length > 0 ? stagedTasks[0].id : null;
  if (activeInspectedTaskId) {
    displayProbeInfo(stagedTasks[0]);
  }
  renderStagedBatchUI();
  updateEnqueueButtonState();
}

export function addStagedTask(task) {
  if (!task || !task.id) return;
  if (!stagedTasks.some(t => t.id === task.id)) {
    stagedTasks.push(task);
    selectedTaskIds.add(task.id);
  }
  if (!activeInspectedTaskId) {
    activeInspectedTaskId = task.id;
    displayProbeInfo(task);
  }
  renderStagedBatchUI();
  updateEnqueueButtonState();
}

export async function loadServerPendingDrafts() {
  try {
    const res = await apiFetch('/api/v1/tasks?status=pending');
    if (!res.ok) return;
    const data = await res.json();
    const drafts = data.tasks || [];
    if (drafts.length > 0) {
      stagedTasks = drafts;
      selectedTaskIds = new Set(drafts.map(t => t.id));
      activeInspectedTaskId = drafts[0].id;
      displayProbeInfo(drafts[0]);
      renderStagedBatchUI();
      updateEnqueueButtonState();
    }
  } catch (e) {
    console.warn('Failed to load pending drafts', e);
  }
}

export function clearStagedTasksInMemory() {
  stagedTasks = [];
  selectedTaskIds.clear();
  activeInspectedTaskId = null;
  renderStagedBatchUI();
  updateEnqueueButtonState();
}

export async function clearStagedBatchWithServerPurge() {
  if (stagedTasks.length === 0) return;
  const ids = stagedTasks.map(t => t.id);
  try {
    const res = await apiFetch('/api/v1/tasks/batch-delete', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ task_ids: ids }),
    });
    if (res.ok) {
      showToast(`已清空并物理删除 ${ids.length} 个暂存视频`, 'info');
    }
  } catch (_) {}
  clearStagedTasksInMemory();
  const probeCard = document.getElementById('probeCard');
  if (probeCard) probeCard.style.display = 'none';
  const fileInput = document.getElementById('fileInput');
  if (fileInput) fileInput.value = '';
}

export function syncSelectAllState() {
  const chk = document.getElementById('chkSelectAllStaged');
  const txt = document.getElementById('txtSelectAllStaged');
  if (!chk || !txt) return;

  const total = stagedTasks.length;
  const checked = selectedTaskIds.size;

  if (total === 0) {
    chk.checked = false;
    chk.indeterminate = false;
    txt.textContent = '全选';
    return;
  }

  if (checked === total) {
    chk.checked = true;
    chk.indeterminate = false;
    txt.textContent = '取消全选';
  } else if (checked === 0) {
    chk.checked = false;
    chk.indeterminate = false;
    txt.textContent = '全选';
  } else {
    chk.checked = false;
    chk.indeterminate = true;
    txt.textContent = '全选';
  }
}

export function updateEnqueueButtonState() {
  const btn = document.getElementById('submitQueueBtn');
  if (!btn) return;
  const isTabAdvanced = document.getElementById('paneAdvanced')?.style.display !== 'none';
  const selectedProfile = getSelectedProfile() || document.querySelector('.profile-card.selected')?.dataset?.name;
  const total = stagedTasks.length;
  const checked = selectedTaskIds.size;

  if (total > 0) {
    btn.textContent = `立即转码压缩 (已选 ${checked}/${total} 个)`;
  } else {
    btn.textContent = '立即转码压缩';
  }

  if (checked > 0 && (selectedProfile || isTabAdvanced)) {
    btn.disabled = false;
  } else {
    btn.disabled = true;
  }

  syncSelectAllState();
}

export function renderStagedBatchUI() {
  const card = document.getElementById('stagedBatchCard');
  const countEl = document.getElementById('stagedBatchCount');
  const listEl = document.getElementById('stagedBatchList');
  if (!card || !countEl || !listEl) return;

  const count = stagedTasks.length;
  countEl.textContent = String(count);

  if (count === 0) {
    card.style.display = 'none';
    listEl.innerHTML = '';
    return;
  }

  card.style.display = 'block';
  listEl.innerHTML = '';

  stagedTasks.forEach((task) => {
    const isChecked = selectedTaskIds.has(task.id);
    const isActive = activeInspectedTaskId === task.id;
    const chip = document.createElement('div');
    chip.className = `staged-file-chip${isActive ? ' is-active' : ''}${isChecked ? ' is-checked' : ''}`;
    chip.dataset.taskId = task.id;
    chip.innerHTML = `
      <label class="staged-file-chk-wrap" title="勾选加入本次转码批次">
        <input type="checkbox" class="staged-file-chk" ${isChecked ? 'checked' : ''}>
      </label>
      <span class="staged-file-name" title="${escapeHtml(task.source_file_name)}">🎬 ${escapeHtml(task.source_file_name)}</span>
      <span class="staged-file-size">${formatBytes(task.source_file_size)}</span>
      <button class="staged-file-del-btn" title="废弃并彻底物理删除此文件" data-task-id="${task.id}">✕</button>
    `;

    const chk = chip.querySelector('.staged-file-chk');
    if (chk) {
      chk.addEventListener('click', (e) => e.stopPropagation());
      chk.addEventListener('change', (e) => {
        e.stopPropagation();
        if (e.target.checked) {
          selectedTaskIds.add(task.id);
          chip.classList.add('is-checked');
        } else {
          selectedTaskIds.delete(task.id);
          chip.classList.remove('is-checked');
        }
        updateEnqueueButtonState();
      });
    }

    chip.addEventListener('click', (e) => {
      if (e.target.closest('.staged-file-del-btn, .staged-file-chk-wrap, input')) return;
      activeInspectedTaskId = task.id;
      displayProbeInfo(task);
      listEl.querySelectorAll('.staged-file-chip').forEach(c => c.classList.remove('is-active'));
      chip.classList.add('is-active');
    });

    const delBtn = chip.querySelector('.staged-file-del-btn');
    if (delBtn) {
      delBtn.addEventListener('click', async (e) => {
        e.stopPropagation();
        await removeSingleStagedTask(task.id, task.source_file_name);
      });
    }

    listEl.appendChild(chip);
  });

  syncSelectAllState();
}

export function updateChipsSelectionUI() {
  const listEl = document.getElementById('stagedBatchList');
  if (!listEl) return;
  const chips = listEl.querySelectorAll('.staged-file-chip');
  chips.forEach((chip) => {
    const taskId = chip.dataset.taskId;
    const isChecked = selectedTaskIds.has(taskId);
    chip.classList.toggle('is-checked', isChecked);
    const chk = chip.querySelector('.staged-file-chk');
    if (chk && chk.checked !== isChecked) {
      chk.checked = isChecked;
    }
  });
  syncSelectAllState();
}

async function removeSingleStagedTask(taskId, fileName) {
  try {
    await apiFetch(`/api/v1/tasks/${taskId}`, { method: 'DELETE' });
    stagedTasks = stagedTasks.filter(t => t.id !== taskId);
    selectedTaskIds.delete(taskId);
    showToast(`已删除暂存视频: ${fileName}`, 'info');
    if (activeInspectedTaskId === taskId) {
      activeInspectedTaskId = stagedTasks.length > 0 ? stagedTasks[0].id : null;
      if (activeInspectedTaskId) {
        const nextTask = stagedTasks.find(t => t.id === activeInspectedTaskId);
        if (nextTask) displayProbeInfo(nextTask);
      }
    }
    renderStagedBatchUI();
    updateEnqueueButtonState();
    if (stagedTasks.length === 0) {
      const probeCard = document.getElementById('probeCard');
      if (probeCard) probeCard.style.display = 'none';
    }
  } catch (err) {
    showToast(`删除失败: ${err.message}`, 'danger');
  }
}

export function initStagedBatch() {
  const clearBtn = document.getElementById('btnClearStagedBatch');
  if (clearBtn) {
    clearBtn.addEventListener('click', async () => {
      if (stagedTasks.length === 0) return;
      const confirmed = await showConfirm({
        title: '清空当前待配置批次',
        message: `确定清空当前已上传的 ${stagedTasks.length} 个待配置视频吗？源文件与临时分片将被彻底物理删除。`,
        confirmText: '确认清空',
        cancelText: '取消',
        type: 'danger',
        icon: '🗑️',
      });
      if (confirmed) {
        await clearStagedBatchWithServerPurge();
      }
    });
  }

  const chkSelectAll = document.getElementById('chkSelectAllStaged');
  if (chkSelectAll) {
    chkSelectAll.addEventListener('change', () => {
      if (stagedTasks.length === 0) return;
      const allSelected = selectedTaskIds.size === stagedTasks.length;
      if (allSelected) {
        selectedTaskIds.clear();
      } else {
        stagedTasks.forEach(t => selectedTaskIds.add(t.id));
      }
      updateChipsSelectionUI();
      updateEnqueueButtonState();
    });
  }

  bus.on('profile:selected', () => {
    updateEnqueueButtonState();
  });

  loadServerPendingDrafts();
  bus.on('auth:unlocked', () => {
    loadServerPendingDrafts();
  });
}
